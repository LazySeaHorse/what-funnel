package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// IngestInbound processes an incoming message event from a channel.
// Steps: resolve account, idempotency check, upsert contact, upsert
// conversation, auto-create lead (if enabled), insert message, commit,
// then publish to conversation.updated.
func (s *Service) IngestInbound(ctx context.Context, event types.InboundEvent) error {
	channelID, err := uuid.Parse(event.ChannelID)
	if err != nil {
		return fmt.Errorf("invalid channel ID: %w", err)
	}

	// 1. Resolve account ID from the channel.
	var accountID uuid.UUID
	err = s.pool.QueryRow(ctx, `SELECT account_id FROM channels WHERE id = $1`, channelID).Scan(&accountID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("channel not found: %s", event.ChannelID)
		}
		return fmt.Errorf("lookup channel account: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin ingestion tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 2. Idempotency – skip if external message ID already persisted.
	if event.Message.ExternalMessageID != "" {
		var existingMsgID uuid.UUID
		err = tx.QueryRow(ctx, `
			SELECT m.id
			FROM messages m
			JOIN conversations c ON m.conversation_id = c.id
			WHERE c.channel_id = $1 AND m.external_message_id = $2 AND m.account_id = $3
		`, channelID, event.Message.ExternalMessageID, accountID).Scan(&existingMsgID)
		if err == nil {
			return nil // already persisted
		}
		if err != pgx.ErrNoRows {
			return fmt.Errorf("check message duplicate: %w", err)
		}
	}

	// 3. Upsert contact.
	var contactID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO contacts (account_id, channel_id, external_identity, display_name, avatar_url)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (channel_id, external_identity)
		DO UPDATE SET
			display_name = COALESCE(NULLIF(EXCLUDED.display_name, ''), contacts.display_name),
			avatar_url = COALESCE(NULLIF(EXCLUDED.avatar_url, ''), contacts.avatar_url)
		RETURNING id
	`, accountID, channelID, event.Contact.ExternalIdentity, event.Contact.DisplayName, event.Contact.AvatarURL).Scan(&contactID)
	if err != nil {
		return fmt.Errorf("upsert contact: %w", err)
	}

	// 4. Upsert conversation.
	timestamp := event.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	var conversationID uuid.UUID
	var isNew bool
	err = tx.QueryRow(ctx, `SELECT id FROM conversations WHERE contact_id = $1 AND channel_id = $2`, contactID, channelID).Scan(&conversationID)
	if err != nil {
		if err != pgx.ErrNoRows {
			return fmt.Errorf("check existing conversation: %w", err)
		}
		isNew = true
	}

	if isNew {
		err = tx.QueryRow(ctx, `
			INSERT INTO conversations (account_id, contact_id, channel_id, last_message_at)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, accountID, contactID, channelID, timestamp).Scan(&conversationID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE conversations SET last_message_at = $1 WHERE id = $2`, timestamp, conversationID)
	}
	if err != nil {
		return fmt.Errorf("upsert conversation: %w", err)
	}

	// 4.1 Auto-create Lead on new conversations when lead tracking is enabled.
	if isNew {
		var settingsRaw []byte
		if err = tx.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1`, accountID).Scan(&settingsRaw); err != nil {
			return fmt.Errorf("get account settings for auto-lead: %w", err)
		}
		if types.IsLeadTrackingEnabled(settingsRaw) {
			var pipelineID uuid.UUID
			var statesJSON []byte
			err = tx.QueryRow(ctx, `SELECT id, states FROM lead_pipelines WHERE account_id = $1 ORDER BY created_at ASC LIMIT 1`, accountID).Scan(&pipelineID, &statesJSON)
			if err != nil && err != pgx.ErrNoRows {
				return fmt.Errorf("get lead pipeline for auto-lead: %w", err)
			}
			if err == nil {
				var states []types.PipelineState
				if err := json.Unmarshal(statesJSON, &states); err != nil {
					return fmt.Errorf("unmarshal pipeline states: %w", err)
				}
				if len(states) > 0 {
					firstStateKey := states[0].Key
					var leadID uuid.UUID
					err = tx.QueryRow(ctx, `
						INSERT INTO leads (account_id, conversation_id, pipeline_id, current_state_key)
						VALUES ($1, $2, $3, $4)
						RETURNING id
					`, accountID, conversationID, pipelineID, firstStateKey).Scan(&leadID)
					if err != nil {
						return fmt.Errorf("auto-create lead: %w", err)
					}
					_, err = tx.Exec(ctx, `
						INSERT INTO lead_state_history (account_id, lead_id, from_state, to_state)
						VALUES ($1, $2, NULL, $3)
					`, accountID, leadID, firstStateKey)
					if err != nil {
						return fmt.Errorf("insert lead state history for auto-lead: %w", err)
					}
					aw := audit.NewWriterFromTx(tx)
					if err = aw.Write(ctx, audit.Entry{
						AccountID:   accountID,
						ActorUserID: nil,
						Action:      "lead.created",
						TargetType:  "lead",
						TargetID:    &leadID,
						Metadata: map[string]any{
							"conversation_id":   conversationID,
							"current_state_key": firstStateKey,
							"auto":              true,
						},
					}); err != nil {
						return fmt.Errorf("write auto-lead audit log: %w", err)
					}
				}
			}
		}
	}

	// 5. Insert message.
	contentRaw, err := json.Marshal(map[string]any{
		"text":                 event.Message.Text,
		"media_url":            event.Message.MediaURL,
		"reply_to_external_id": event.Message.ReplyToExternalID,
	})
	if err != nil {
		return fmt.Errorf("marshal message content: %w", err)
	}

	var externalMsgID *string
	if event.Message.ExternalMessageID != "" {
		externalMsgID = &event.Message.ExternalMessageID
	}

	var messageID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content, external_message_id, created_at)
		VALUES ($1, $2, 'inbound', 'contact', $3, $4, $5, $6)
		RETURNING id
	`, accountID, conversationID, event.Message.ContentType, contentRaw, externalMsgID, timestamp).Scan(&messageID)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit ingestion tx: %w", err)
	}

	// 6. Publish (best-effort after commit).
	if _, err = s.pubsub.Publish(ctx, "conversation.updated", ConversationUpdatedEvent{
		AccountID:      accountID,
		ConversationID: conversationID,
		MessageID:      messageID,
	}); err != nil {
		fmt.Printf("failed to publish conversation.updated: %v\n", err)
	}
	return nil
}

// IngestExternalOutbound handles persisting an outbound message sent externally
// (e.g. from the phone directly via the bridge) without going through SendMessage.
func (s *Service) IngestExternalOutbound(ctx context.Context, event types.ExternalOutboundEvent) error {
	channelID, err := uuid.Parse(event.ChannelID)
	if err != nil {
		return fmt.Errorf("invalid channel ID: %w", err)
	}

	var accountID uuid.UUID
	err = s.pool.QueryRow(ctx, `SELECT account_id FROM channels WHERE id = $1`, channelID).Scan(&accountID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("channel not found: %s", event.ChannelID)
		}
		return fmt.Errorf("lookup channel account: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin ingestion tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Idempotency.
	if event.ExternalMessageID != "" {
		var existingMsgID uuid.UUID
		err = tx.QueryRow(ctx, `
			SELECT m.id
			FROM messages m
			JOIN conversations c ON m.conversation_id = c.id
			WHERE c.channel_id = $1 AND m.external_message_id = $2 AND m.account_id = $3
		`, channelID, event.ExternalMessageID, accountID).Scan(&existingMsgID)
		if err == nil {
			return nil
		}
		if err != pgx.ErrNoRows {
			return fmt.Errorf("check message duplicate: %w", err)
		}
	}

	// Resolve conversation by channel + contact's external identity.
	var conversationID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT c.id
		FROM conversations c
		JOIN contacts co ON c.contact_id = co.id
		WHERE c.channel_id = $1 AND co.external_identity = $2 AND c.account_id = $3
	`, channelID, event.ExternalThreadID, accountID).Scan(&conversationID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("conversation not found for channel %s and thread %s", event.ChannelID, event.ExternalThreadID)
		}
		return fmt.Errorf("lookup conversation: %w", err)
	}

	contentRaw, err := json.Marshal(map[string]any{
		"text":            event.Message.Text,
		"media_url":       event.Message.MediaURL,
		"external_origin": true,
	})
	if err != nil {
		return fmt.Errorf("marshal message content: %w", err)
	}

	timestamp := event.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	var externalMsgID *string
	if event.ExternalMessageID != "" {
		externalMsgID = &event.ExternalMessageID
	}

	var messageID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO messages (account_id, conversation_id, direction, sender_type, sender_user_id, content_type, content, external_message_id, created_at)
		VALUES ($1, $2, 'outbound', 'human', NULL, $3, $4, $5, $6)
		RETURNING id
	`, accountID, conversationID, event.Message.ContentType, contentRaw, externalMsgID, timestamp).Scan(&messageID)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE conversations
		SET last_message_at = $1, ai_mode_active = false
		WHERE id = $2 AND account_id = $3
	`, timestamp, conversationID, accountID)
	if err != nil {
		return fmt.Errorf("update conversation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit ingestion tx: %w", err)
	}

	if _, err = s.pubsub.Publish(ctx, "conversation.updated", ConversationUpdatedEvent{
		AccountID:      accountID,
		ConversationID: conversationID,
		MessageID:      messageID,
	}); err != nil {
		fmt.Printf("failed to publish conversation.updated: %v\n", err)
	}
	return nil
}

// PublishInbound publishes a normalized InboundEvent to the messages.inbound stream.
func (s *Service) PublishInbound(ctx context.Context, event types.InboundEvent) error {
	if _, err := s.pubsub.Publish(ctx, "messages.inbound", event); err != nil {
		return fmt.Errorf("publish inbound event: %w", err)
	}
	return nil
}

// SimulateInbound builds a mock InboundEvent and publishes it to the
// messages.inbound stream, used for testing channel integrations in the UI.
func (s *Service) SimulateInbound(
	ctx context.Context,
	accountID uuid.UUID,
	channelID string,
	senderExternalID string,
	senderDisplayName string,
	senderAvatarURL string,
	contentType string,
	text string,
	mediaURL string,
) error {
	// Verify the channel belongs to this account.
	var exists bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM channels WHERE id = $1 AND account_id = $2)`,
		channelID, accountID,
	).Scan(&exists); err != nil {
		return fmt.Errorf("channel lookup failed: %w", err)
	}
	if !exists {
		return fmt.Errorf("channel not found or not owned by account")
	}

	extMsgID := fmt.Sprintf("$sim-%s", uuid.New().String())
	event := types.InboundEvent{
		ChannelID:        channelID,
		ExternalThreadID: senderExternalID,
		Contact: types.ContactRef{
			ExternalIdentity: senderExternalID,
			DisplayName:      senderDisplayName,
			AvatarURL:        senderAvatarURL,
		},
		Message: types.NormalizedMessage{
			ContentType:       contentType,
			Text:              text,
			MediaURL:          mediaURL,
			ExternalMessageID: extMsgID,
		},
		Timestamp: time.Now(),
	}

	if _, err := s.pubsub.Publish(ctx, "messages.inbound", event); err != nil {
		return fmt.Errorf("publish simulated inbound event: %w", err)
	}
	return nil
}
