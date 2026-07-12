package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	matrixadapter "github.com/whatfunnel/whatfunnel/adapters/matrix-mautrix"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/crypto"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// ConversationUpdatedEvent is published to conversation.updated stream.
type ConversationUpdatedEvent struct {
	AccountID      uuid.UUID `json:"account_id"`
	ConversationID uuid.UUID `json:"conversation_id"`
	MessageID      uuid.UUID `json:"message_id"`
}

type Service struct {
	pool      *pgxpool.Pool
	cipher    *crypto.Cipher
	pubsub    *pubsub.Client
	adapters  map[string]types.ChannelAdapter
	adaptersMu sync.RWMutex
}

func New(pool *pgxpool.Pool, cipher *crypto.Cipher, pubsub *pubsub.Client) *Service {
	return &Service{
		pool:     pool,
		cipher:   cipher,
		pubsub:   pubsub,
		adapters: make(map[string]types.ChannelAdapter),
	}
}

// RegisterAdapter associates a channel type with an adapter instance.
func (s *Service) RegisterAdapter(channelType string, adapter types.ChannelAdapter) {
	s.adaptersMu.Lock()
	defer s.adaptersMu.Unlock()
	s.adapters[channelType] = adapter
}

// GetAdapter retrieves the adapter for a channel type.
func (s *Service) GetAdapter(channelType string) (types.ChannelAdapter, error) {
	s.adaptersMu.RLock()
	defer s.adaptersMu.RUnlock()
	adapter, ok := s.adapters[channelType]
	if !ok {
		return nil, fmt.Errorf("no adapter registered for channel type: %s", channelType)
	}
	return adapter, nil
}

// InitAdapters loads all channels from DB, decrypts credentials, and configures adapters.
func (s *Service) InitAdapters(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `SELECT id, type, bridge_credentials FROM channels`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		var channelType string
		var dbCreds []byte
		if err := rows.Scan(&id, &channelType, &dbCreds); err != nil {
			return err
		}

		if len(dbCreds) > 0 {
			decrypted, err := s.DecryptCredentials(dbCreds)
			if err != nil {
				continue
			}

			if adapter, err := s.GetAdapter(channelType); err == nil {
				if configurable, ok := adapter.(interface {
					Configure(channelID string, creds matrixadapter.Credentials)
				}); ok {
					var mc matrixadapter.Credentials
					if err := json.Unmarshal(decrypted, &mc); err == nil {
						configurable.Configure(id.String(), mc)
					}
				}
			}
		}
	}
	return nil
}

// EncryptCredentials encrypts raw credentials for storing in Postgres JSONB.
func (s *Service) EncryptCredentials(creds []byte) ([]byte, error) {
	ciphertext, err := s.cipher.Encrypt(creds)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt credentials: %w", err)
	}
	payload := map[string]string{"encrypted_data": ciphertext}
	return json.Marshal(payload)
}

// DecryptCredentials decrypts credentials retrieved from Postgres JSONB.
func (s *Service) DecryptCredentials(dbCreds []byte) ([]byte, error) {
	if len(dbCreds) == 0 {
		return nil, nil
	}
	var payload map[string]string
	if err := json.Unmarshal(dbCreds, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal encrypted credentials wrapper: %w", err)
	}
	ciphertext, ok := payload["encrypted_data"]
	if !ok {
		return nil, errors.New("invalid credentials structure: missing encrypted_data")
	}
	return s.cipher.Decrypt(ciphertext)
}

// IngestInbound processes an incoming message event from a channel.
func (s *Service) IngestInbound(ctx context.Context, event types.InboundEvent) error {
	channelID, err := uuid.Parse(event.ChannelID)
	if err != nil {
		return fmt.Errorf("invalid channel ID: %w", err)
	}

	// 1. Resolve account ID from the channel
	var accountID uuid.UUID
	err = s.pool.QueryRow(ctx, `SELECT account_id FROM channels WHERE id = $1`, channelID).Scan(&accountID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("channel not found: %s", event.ChannelID)
		}
		return fmt.Errorf("lookup channel account: %w", err)
	}

	// Start single transaction for atomicity
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin ingestion tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 2. Check for duplicate if external message ID is present (Idempotency)
	if event.Message.ExternalMessageID != "" {
		var existingMsgID uuid.UUID
		err = tx.QueryRow(ctx, `
			SELECT m.id
			FROM messages m
			JOIN conversations c ON m.conversation_id = c.id
			WHERE c.channel_id = $1 AND m.external_message_id = $2 AND m.account_id = $3
		`, channelID, event.Message.ExternalMessageID, accountID).Scan(&existingMsgID)
		if err == nil {
			// Message already persisted, return success (skip)
			return nil
		}
		if err != pgx.ErrNoRows {
			return fmt.Errorf("check message duplicate: %w", err)
		}
	}

	// 3. Upsert contact
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

	// 4. Upsert conversation
	var conversationID uuid.UUID
	var isNew bool
	timestamp := event.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	err = tx.QueryRow(ctx, `SELECT id FROM conversations WHERE contact_id = $1 AND channel_id = $2`, contactID, channelID).Scan(&conversationID)
	if err != nil {
		if err == pgx.ErrNoRows {
			isNew = true
		} else {
			return fmt.Errorf("check existing conversation: %w", err)
		}
	}

	if isNew {
		err = tx.QueryRow(ctx, `
			INSERT INTO conversations (account_id, contact_id, channel_id, last_message_at)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, accountID, contactID, channelID, timestamp).Scan(&conversationID)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE conversations SET last_message_at = $1 WHERE id = $2
		`, timestamp, conversationID)
	}
	if err != nil {
		return fmt.Errorf("upsert conversation: %w", err)
	}

	// 4.1 Auto-create Lead if enabled and is new conversation
	if isNew {
		var settingsRaw []byte
		err = tx.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1`, accountID).Scan(&settingsRaw)
		if err != nil {
			return fmt.Errorf("get account settings for auto-lead: %w", err)
		}
		if types.IsLeadTrackingEnabled(settingsRaw) {
			var pipelineID uuid.UUID
			var statesJSON []byte
			err = tx.QueryRow(ctx, `SELECT id, states FROM lead_pipelines WHERE account_id = $1 ORDER BY created_at ASC LIMIT 1`, accountID).Scan(&pipelineID, &statesJSON)
			if err != nil {
				if err != pgx.ErrNoRows {
					return fmt.Errorf("get lead pipeline for auto-lead: %w", err)
				}
			} else {
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
					err = aw.Write(ctx, audit.Entry{
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
					})
					if err != nil {
						return fmt.Errorf("write auto-lead audit log: %w", err)
					}
				}
			}
		}
	}

	// Serialize message content
	contentRaw, err := json.Marshal(map[string]any{
		"text":                 event.Message.Text,
		"media_url":            event.Message.MediaURL,
		"reply_to_external_id": event.Message.ReplyToExternalID,
	})
	if err != nil {
		return fmt.Errorf("marshal message content: %w", err)
	}

	// 5. Insert message
	var messageID uuid.UUID
	var externalMsgID *string
	if event.Message.ExternalMessageID != "" {
		externalMsgID = &event.Message.ExternalMessageID
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content, external_message_id, created_at)
		VALUES ($1, $2, 'inbound', 'contact', $3, $4, $5, $6)
		RETURNING id
	`, accountID, conversationID, event.Message.ContentType, contentRaw, externalMsgID, timestamp).Scan(&messageID)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit ingestion tx: %w", err)
	}

	// 6. Publish to conversation.updated stream
	_, err = s.pubsub.Publish(ctx, "conversation.updated", ConversationUpdatedEvent{
		AccountID:      accountID,
		ConversationID: conversationID,
		MessageID:      messageID,
	})
	if err != nil {
		// Log error but don't fail ingestion (it's committed)
		fmt.Printf("failed to publish conversation.updated: %v\n", err)
	}

	return nil
}

// SendMessage sends an outbound message via the adapter and records it in the database.
func (s *Service) SendMessage(ctx context.Context, accountID, conversationID uuid.UUID, senderType string, senderUserID *uuid.UUID, contentType, text, mediaURL string) (*types.Message, error) {
	if senderType != "human" && senderType != "ai" {
		return nil, fmt.Errorf("invalid sender_type: %q", senderType)
	}

	// Start transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin send tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Look up conversation, channel details, and contact external identity
	var channelID uuid.UUID
	var channelType string
	var externalIdentity string
	err = tx.QueryRow(ctx, `
		SELECT c.channel_id, ch.type, co.external_identity
		FROM conversations c
		JOIN channels ch ON c.channel_id = ch.id
		JOIN contacts co ON c.contact_id = co.id
		WHERE c.id = $1 AND c.account_id = $2
	`, conversationID, accountID).Scan(&channelID, &channelType, &externalIdentity)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("conversation not found")
		}
		return nil, fmt.Errorf("lookup conversation details: %w", err)
	}

	// Get registered adapter for the channel type
	adapter, err := s.GetAdapter(channelType)
	if err != nil {
		return nil, err
	}

	// Try to configure the adapter dynamically before sending to ensure credentials are fresh/loaded
	var dbCreds []byte
	err = tx.QueryRow(ctx, `SELECT bridge_credentials FROM channels WHERE id = $1`, channelID).Scan(&dbCreds)
	if err == nil && len(dbCreds) > 0 {
		decrypted, err := s.DecryptCredentials(dbCreds)
		if err == nil {
			if configurable, ok := adapter.(interface {
				Configure(channelID string, creds matrixadapter.Credentials)
			}); ok {
				var mc matrixadapter.Credentials
				if err := json.Unmarshal(decrypted, &mc); err == nil {
					configurable.Configure(channelID.String(), mc)
				}
			}
		}
	}

	// Call adapter SendMessage
	msgPayload := types.NormalizedMessage{
		ContentType: contentType,
		Text:        text,
		MediaURL:    mediaURL,
	}

	// Call adapter
	externalMsgID, err := adapter.SendMessage(ctx, channelID.String(), externalIdentity, msgPayload)
	if err != nil {
		return nil, fmt.Errorf("adapter send failed: %w", err)
	}

	// Serialize content
	contentRaw, err := json.Marshal(map[string]any{
		"text":      text,
		"media_url": mediaURL,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal outbound message: %w", err)
	}

	var extMsgPtr *string
	if externalMsgID != "" {
		extMsgPtr = &externalMsgID
	}

	// 2. Persist outbound message in DB
	msg := &types.Message{
		AccountID:         accountID,
		ConversationID:    conversationID,
		Direction:         "outbound",
		SenderType:        senderType,
		SenderUserID:      senderUserID,
		ContentType:       contentType,
		Content:           contentRaw,
		ExternalMessageID: extMsgPtr,
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO messages (account_id, conversation_id, direction, sender_type, sender_user_id, content_type, content, external_message_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		RETURNING id, created_at
	`, msg.AccountID, msg.ConversationID, msg.Direction, msg.SenderType, msg.SenderUserID, msg.ContentType, msg.Content, msg.ExternalMessageID).
		Scan(&msg.ID, &msg.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert outbound message: %w", err)
	}

	// 3. Update conversation last_message_at
	if senderType == "human" {
		_, err = tx.Exec(ctx, `
			UPDATE conversations
			SET last_message_at = $1, ai_mode_active = false
			WHERE id = $2 AND account_id = $3
		`, msg.CreatedAt, conversationID, accountID)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE conversations
			SET last_message_at = $1
			WHERE id = $2 AND account_id = $3
		`, msg.CreatedAt, conversationID, accountID)
	}
	if err != nil {
		return nil, fmt.Errorf("update conversation details: %w", err)
	}

	// 4. Audit logging (reusing standard writer)
	aw := audit.NewWriterFromTx(tx)
	err = aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: senderUserID,
		Action:      "message.sent",
		TargetType:  "message",
		TargetID:    &msg.ID,
		Metadata: map[string]any{
			"conversation_id": conversationID,
			"content_type":    contentType,
			"sender_type":     senderType,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("write audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit send tx: %w", err)
	}

	// 5. Publish to conversation.updated stream
	_, err = s.pubsub.Publish(ctx, "conversation.updated", ConversationUpdatedEvent{
		AccountID:      accountID,
		ConversationID: conversationID,
		MessageID:      msg.ID,
	})
	if err != nil {
		fmt.Printf("failed to publish conversation.updated for outbound send: %v\n", err)
	}

	return msg, nil
}

// CreateChannel registers a new channel.
func (s *Service) CreateChannel(ctx context.Context, accountID uuid.UUID, channelType string, bridgeIdentity *string, rawCredentials []byte) (*types.Channel, error) {
	var encryptedCreds []byte
	if len(rawCredentials) > 0 {
		var err error
		encryptedCreds, err = s.EncryptCredentials(rawCredentials)
		if err != nil {
			return nil, err
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	ch := &types.Channel{
		AccountID:         accountID,
		Type:              channelType,
		BridgeIdentity:    bridgeIdentity,
		BridgeCredentials: encryptedCreds,
		Status:            "disconnected",
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, bridge_identity, bridge_credentials, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`, ch.AccountID, ch.Type, ch.BridgeIdentity, ch.BridgeCredentials, ch.Status).
		Scan(&ch.ID, &ch.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert channel: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	err = aw.Write(ctx, audit.Entry{
		AccountID:  accountID,
		Action:     "channel.created",
		TargetType: "channel",
		TargetID:   &ch.ID,
		Metadata:   map[string]any{"type": channelType},
	})
	if err != nil {
		return nil, fmt.Errorf("write audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Configure the adapter if applicable
	if len(rawCredentials) > 0 {
		if adapter, err := s.GetAdapter(channelType); err == nil {
			if configurable, ok := adapter.(interface {
				Configure(channelID string, creds matrixadapter.Credentials)
			}); ok {
				var mc matrixadapter.Credentials
				if err := json.Unmarshal(rawCredentials, &mc); err == nil {
					configurable.Configure(ch.ID.String(), mc)
				}
			}
		}
	}

	return ch, nil
}

// GetChannel retrieves a channel by ID.
func (s *Service) GetChannel(ctx context.Context, accountID, channelID uuid.UUID) (*types.Channel, error) {
	ch := &types.Channel{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, account_id, type, bridge_identity, bridge_credentials, status, status_detail, created_at
		FROM channels
		WHERE id = $1 AND account_id = $2
	`, channelID, accountID).Scan(
		&ch.ID, &ch.AccountID, &ch.Type, &ch.BridgeIdentity, &ch.BridgeCredentials, &ch.Status, &ch.StatusDetail, &ch.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("channel not found")
		}
		return nil, err
	}
	return ch, nil
}

// ListChannels returns all channels for an account.
func (s *Service) ListChannels(ctx context.Context, accountID uuid.UUID) ([]*types.Channel, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, account_id, type, bridge_identity, bridge_credentials, status, status_detail, created_at
		FROM channels
		WHERE account_id = $1
		ORDER BY created_at ASC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*types.Channel
	for rows.Next() {
		ch := &types.Channel{}
		err := rows.Scan(
			&ch.ID, &ch.AccountID, &ch.Type, &ch.BridgeIdentity, &ch.BridgeCredentials, &ch.Status, &ch.StatusDetail, &ch.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		channels = append(channels, ch)
	}
	return channels, rows.Err()
}

// DisconnectChannel updates channel status to disconnected and clears credentials if requested.
func (s *Service) DisconnectChannel(ctx context.Context, accountID, channelID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Verify channel exists
	var typeName string
	err = tx.QueryRow(ctx, `SELECT type FROM channels WHERE id = $1 AND account_id = $2`, channelID, accountID).Scan(&typeName)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("channel not found")
		}
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE channels
		SET status = 'disconnected', status_detail = 'Disconnected by admin'
		WHERE id = $1 AND account_id = $2
	`, channelID, accountID)
	if err != nil {
		return fmt.Errorf("disconnect channel: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	err = aw.Write(ctx, audit.Entry{
		AccountID:  accountID,
		Action:     "channel.disconnected",
		TargetType: "channel",
		TargetID:   &channelID,
		Metadata:   map[string]any{},
	})
	if err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}

	return tx.Commit(ctx)
}

// GetChannelStatus reflects live Status from the adapter.
func (s *Service) GetChannelStatus(ctx context.Context, accountID, channelID uuid.UUID) (types.ChannelStatus, error) {
	var chType string
	err := s.pool.QueryRow(ctx, `SELECT type FROM channels WHERE id = $1 AND account_id = $2`, channelID, accountID).Scan(&chType)
	if err != nil {
		if err == pgx.ErrNoRows {
			return types.ChannelStatus{}, fmt.Errorf("channel not found")
		}
		return types.ChannelStatus{}, err
	}

	adapter, err := s.GetAdapter(chType)
	if err != nil {
		// Fallback to DB status if no live adapter is running
		var dbStatus, dbDetail string
		var rawDetail *string
		err = s.pool.QueryRow(ctx, `SELECT status, status_detail FROM channels WHERE id = $1`, channelID).Scan(&dbStatus, &rawDetail)
		if err == nil {
			if rawDetail != nil {
				dbDetail = *rawDetail
			}
			return types.ChannelStatus{Status: dbStatus, Detail: dbDetail}, nil
		}
		return types.ChannelStatus{}, err
	}

	status := adapter.Status(channelID.String())

	// Optionally sync status back to DB
	_, _ = s.pool.Exec(ctx, `UPDATE channels SET status = $1, status_detail = $2 WHERE id = $3`, status.Status, status.Detail, channelID)

	return status, nil
}

type ConversationAssignedEvent struct {
	AccountID       uuid.UUID   `json:"account_id"`
	ConversationID  uuid.UUID   `json:"conversation_id"`
	AssignedUserIDs []uuid.UUID `json:"assigned_user_ids"`
}

func (s *Service) ListConversations(ctx context.Context, accountID, userID uuid.UUID, userRole string, filter string, leadState string) ([]*types.ConversationListItem, error) {
	var settingsBytes []byte
	err := s.pool.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1`, accountID).Scan(&settingsBytes)
	if err != nil {
		return nil, fmt.Errorf("get account settings: %w", err)
	}
	unassignedVisible := types.IsUnassignedVisible(settingsBytes)

	// Base SQL query selecting conversations with contact info, unread flag, lateral last message, and lead details.
	sqlQuery := `
		SELECT c.id, c.account_id, c.contact_id, c.channel_id, c.status, c.assigned_user_ids, c.last_message_at, c.ai_mode_active, c.created_at,
		       co.display_name, co.avatar_url,
		       cr.last_read_at,
		       m.id as msg_id, m.direction as msg_direction, m.sender_type as msg_sender_type, m.sender_user_id as msg_sender_user_id,
		       m.content_type as msg_content_type, m.content as msg_content, m.external_message_id as msg_external_id, m.created_at as msg_created_at,
		       l.id as lead_id, l.pipeline_id as lead_pipeline_id, l.current_state_key as lead_current_state_key, l.tags as lead_tags, l.created_by as lead_created_by, l.created_at as lead_created_at, l.updated_at as lead_updated_at
		FROM conversations c
		JOIN contacts co ON c.contact_id = co.id
		LEFT JOIN conversation_reads cr ON c.id = cr.conversation_id AND cr.user_id = $1
		LEFT JOIN leads l ON c.id = l.conversation_id
		LEFT JOIN LATERAL (
		    SELECT id, direction, sender_type, sender_user_id, content_type, content, external_message_id, created_at
		    FROM messages
		    WHERE conversation_id = c.id
		    ORDER BY created_at DESC, id DESC
		    LIMIT 1
		) m ON TRUE
		WHERE c.account_id = $2 AND (
		    $3 = 'admin' OR
		    $1 = ANY(c.assigned_user_ids) OR
		    (cardinality(c.assigned_user_ids) = 0 AND $4 = true)
		)
	`
	args := []any{userID, accountID, userRole, unassignedVisible}

	if filter == "mine" {
		sqlQuery += ` AND $1 = ANY(c.assigned_user_ids)`
	} else if filter == "unassigned" {
		sqlQuery += ` AND (c.assigned_user_ids IS NULL OR cardinality(c.assigned_user_ids) = 0)`
	}

	if leadState != "" {
		args = append(args, leadState)
		sqlQuery += fmt.Sprintf(` AND l.current_state_key = $%d`, len(args))
	}

	sqlQuery += ` ORDER BY c.last_message_at DESC NULLS LAST, c.created_at DESC`

	rows, err := s.pool.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("query conversations: %w", err)
	}
	defer rows.Close()

	var list []*types.ConversationListItem
	for rows.Next() {
		item := &types.ConversationListItem{}
		var lastReadAt *time.Time
		
		// Message fields (nullable fields parsed manually)
		var msgID *uuid.UUID
		var msgDirection *string
		var msgSenderType *string
		var msgSenderUserID *uuid.UUID
		var msgContentType *string
		var msgContent []byte
		var msgExternalID *string
		var msgCreatedAt *time.Time

		// Lead fields (nullable)
		var leadID *uuid.UUID
		var leadPipelineID *uuid.UUID
		var leadStateKey *string
		var leadTags []string
		var leadCreatedBy *uuid.UUID
		var leadCreatedAt *time.Time
		var leadUpdatedAt *time.Time

		err := rows.Scan(
			&item.Conversation.ID,
			&item.Conversation.AccountID,
			&item.Conversation.ContactID,
			&item.Conversation.ChannelID,
			&item.Conversation.Status,
			&item.Conversation.AssignedUserIDs,
			&item.Conversation.LastMessageAt,
			&item.Conversation.AIModeActive,
			&item.Conversation.CreatedAt,
			&item.ContactName,
			&item.ContactAvatarURL,
			&lastReadAt,
			&msgID,
			&msgDirection,
			&msgSenderType,
			&msgSenderUserID,
			&msgContentType,
			&msgContent,
			&msgExternalID,
			&msgCreatedAt,
			&leadID,
			&leadPipelineID,
			&leadStateKey,
			&leadTags,
			&leadCreatedBy,
			&leadCreatedAt,
			&leadUpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan conversation row: %w", err)
		}

		// Compute unread flag
		if item.Conversation.LastMessageAt != nil {
			if lastReadAt == nil {
				item.Unread = true
			} else {
				item.Unread = item.Conversation.LastMessageAt.After(*lastReadAt)
			}
		} else {
			item.Unread = false
		}

		// Build last message preview if present
		if msgID != nil {
			item.LastMessagePreview = &types.Message{
				ID:                *msgID,
				AccountID:         item.Conversation.AccountID,
				ConversationID:    item.Conversation.ID,
				Direction:         *msgDirection,
				SenderType:        *msgSenderType,
				SenderUserID:      msgSenderUserID,
				ContentType:       *msgContentType,
				Content:           msgContent,
				ExternalMessageID: msgExternalID,
				CreatedAt:         *msgCreatedAt,
			}
		}

		// Build lead summary if present
		if leadID != nil {
			item.Lead = &types.Lead{
				ID:              *leadID,
				AccountID:       item.Conversation.AccountID,
				ConversationID:  item.Conversation.ID,
				PipelineID:      *leadPipelineID,
				CurrentStateKey: *leadStateKey,
				Tags:            leadTags,
				CreatedBy:       leadCreatedBy,
				CreatedAt:       *leadCreatedAt,
				UpdatedAt:       *leadUpdatedAt,
			}
		}

		list = append(list, item)
	}

	return list, rows.Err()
}

func (s *Service) GetConversation(ctx context.Context, accountID, userID, conversationID uuid.UUID, userRole string) (*types.ConversationListItem, error) {
	var settingsBytes []byte
	err := s.pool.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1`, accountID).Scan(&settingsBytes)
	if err != nil {
		return nil, fmt.Errorf("get account settings: %w", err)
	}
	unassignedVisible := types.IsUnassignedVisible(settingsBytes)

	// Fetch single conversation with lead details.
	item := &types.ConversationListItem{}
	var lastReadAt *time.Time

	var msgID *uuid.UUID
	var msgDirection *string
	var msgSenderType *string
	var msgSenderUserID *uuid.UUID
	var msgContentType *string
	var msgContent []byte
	var msgExternalID *string
	var msgCreatedAt *time.Time

	var leadID *uuid.UUID
	var leadPipelineID *uuid.UUID
	var leadStateKey *string
	var leadTags []string
	var leadCreatedBy *uuid.UUID
	var leadCreatedAt *time.Time
	var leadUpdatedAt *time.Time

	err = s.pool.QueryRow(ctx, `
		SELECT c.id, c.account_id, c.contact_id, c.channel_id, c.status, c.assigned_user_ids, c.last_message_at, c.ai_mode_active, c.created_at,
		       co.display_name, co.avatar_url,
		       cr.last_read_at,
		       m.id as msg_id, m.direction as msg_direction, m.sender_type as msg_sender_type, m.sender_user_id as msg_sender_user_id,
		       m.content_type as msg_content_type, m.content as msg_content, m.external_message_id as msg_external_id, m.created_at as msg_created_at,
		       l.id as lead_id, l.pipeline_id as lead_pipeline_id, l.current_state_key as lead_current_state_key, l.tags as lead_tags, l.created_by as lead_created_by, l.created_at as lead_created_at, l.updated_at as lead_updated_at
		FROM conversations c
		JOIN contacts co ON c.contact_id = co.id
		LEFT JOIN conversation_reads cr ON c.id = cr.conversation_id AND cr.user_id = $1
		LEFT JOIN leads l ON c.id = l.conversation_id
		LEFT JOIN LATERAL (
		    SELECT id, direction, sender_type, sender_user_id, content_type, content, external_message_id, created_at
		    FROM messages
		    WHERE conversation_id = c.id
		    ORDER BY created_at DESC, id DESC
		    LIMIT 1
		) m ON TRUE
		WHERE c.id = $2 AND c.account_id = $3
	`, userID, conversationID, accountID).Scan(
		&item.Conversation.ID,
		&item.Conversation.AccountID,
		&item.Conversation.ContactID,
		&item.Conversation.ChannelID,
		&item.Conversation.Status,
		&item.Conversation.AssignedUserIDs,
		&item.Conversation.LastMessageAt,
		&item.Conversation.AIModeActive,
		&item.Conversation.CreatedAt,
		&item.ContactName,
		&item.ContactAvatarURL,
		&lastReadAt,
		&msgID,
		&msgDirection,
		&msgSenderType,
		&msgSenderUserID,
		&msgContentType,
		&msgContent,
		&msgExternalID,
		&msgCreatedAt,
		&leadID,
		&leadPipelineID,
		&leadStateKey,
		&leadTags,
		&leadCreatedBy,
		&leadCreatedAt,
		&leadUpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.New("conversation not found")
		}
		return nil, err
	}

	// Apply visibility check
	if !types.CanSeeConversation(userRole, userID, item.Conversation.AssignedUserIDs, unassignedVisible) {
		return nil, errors.New("conversation not found")
	}

	// Compute unread flag
	if item.Conversation.LastMessageAt != nil {
		if lastReadAt == nil {
			item.Unread = true
		} else {
			item.Unread = item.Conversation.LastMessageAt.After(*lastReadAt)
		}
	} else {
		item.Unread = false
	}

	// Build last message preview if present
	if msgID != nil {
		item.LastMessagePreview = &types.Message{
			ID:                *msgID,
			AccountID:         item.Conversation.AccountID,
			ConversationID:    item.Conversation.ID,
			Direction:         *msgDirection,
			SenderType:        *msgSenderType,
			SenderUserID:      msgSenderUserID,
			ContentType:       *msgContentType,
			Content:           msgContent,
			ExternalMessageID: msgExternalID,
			CreatedAt:         *msgCreatedAt,
		}
	}

	// Build lead summary if present
	if leadID != nil {
		item.Lead = &types.Lead{
			ID:              *leadID,
			AccountID:       item.Conversation.AccountID,
			ConversationID:  item.Conversation.ID,
			PipelineID:      *leadPipelineID,
			CurrentStateKey: *leadStateKey,
			Tags:            leadTags,
			CreatedBy:       leadCreatedBy,
			CreatedAt:       *leadCreatedAt,
			UpdatedAt:       *leadUpdatedAt,
		}
	}

	return item, nil
}

func (s *Service) GetConversationMessages(ctx context.Context, accountID, userID, conversationID uuid.UUID, userRole string, beforeCursor string, limit int) ([]*types.Message, string, error) {
	// First check visibility using GetConversation
	_, err := s.GetConversation(ctx, accountID, userID, conversationID, userRole)
	if err != nil {
		return nil, "", err
	}

	// Build query
	var sqlQuery string
	var args []any
	args = append(args, conversationID, accountID)

	sqlQuery = `
		SELECT id, account_id, conversation_id, direction, sender_type, sender_user_id, content_type, content, external_message_id, created_at
		FROM messages
		WHERE conversation_id = $1 AND account_id = $2
	`

	if beforeCursor != "" {
		cursorTime, cursorID, err := decodeCursor(beforeCursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		args = append(args, cursorTime, cursorID)
		sqlQuery += fmt.Sprintf(" AND (created_at < $%d OR (created_at = $%d AND id < $%d))", len(args)-1, len(args)-1, len(args))
	}

	args = append(args, limit)
	sqlQuery += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", len(args))

	rows, err := s.pool.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, "", fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var messages []*types.Message
	for rows.Next() {
		msg := &types.Message{}
		err := rows.Scan(
			&msg.ID,
			&msg.AccountID,
			&msg.ConversationID,
			&msg.Direction,
			&msg.SenderType,
			&msg.SenderUserID,
			&msg.ContentType,
			&msg.Content,
			&msg.ExternalMessageID,
			&msg.CreatedAt,
		)
		if err != nil {
			return nil, "", fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if len(messages) > 0 && len(messages) == limit {
		lastMsg := messages[len(messages)-1]
		nextCursor = encodeCursor(lastMsg.CreatedAt, lastMsg.ID)
	}

	return messages, nextCursor, nil
}

func (s *Service) AssignConversation(ctx context.Context, accountID uuid.UUID, conversationID uuid.UUID, assignedUserIDs []uuid.UUID, actorUserID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Update conversation assigned_user_ids
	_, err = tx.Exec(ctx, `
		UPDATE conversations
		SET assigned_user_ids = $1
		WHERE id = $2 AND account_id = $3
	`, assignedUserIDs, conversationID, accountID)
	if err != nil {
		return fmt.Errorf("update assignment: %w", err)
	}

	// Write audit log entry
	aw := audit.NewWriterFromTx(tx)
	err = aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &actorUserID,
		Action:      "conversation.assigned",
		TargetType:  "conversation",
		TargetID:    &conversationID,
		Metadata: map[string]any{
			"assigned_user_ids": assignedUserIDs,
		},
	})
	if err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Publish to conversation.assigned stream
	_, err = s.pubsub.Publish(ctx, "conversation.assigned", ConversationAssignedEvent{
		AccountID:       accountID,
		ConversationID:  conversationID,
		AssignedUserIDs: assignedUserIDs,
	})
	if err != nil {
		fmt.Printf("failed to publish conversation.assigned: %v\n", err)
	}

	return nil
}

func (s *Service) checkConversationVisibility(ctx context.Context, accountID, userID uuid.UUID, convoID uuid.UUID, userRole string) error {
	_, err := s.GetConversation(ctx, accountID, userID, convoID, userRole)
	return err
}

func (s *Service) CreateLead(ctx context.Context, accountID, userID, convoID uuid.UUID, userRole string) (*types.Lead, error) {
	// 1. Verify conversation visibility
	_, err := s.GetConversation(ctx, accountID, userID, convoID, userRole)
	if err != nil {
		return nil, err
	}

	// Start transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Check if lead already exists for this conversation
	var existing types.Lead
	var leadCreatedBy *uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id, account_id, conversation_id, pipeline_id, current_state_key, tags, created_by, created_at, updated_at
		FROM leads WHERE conversation_id = $1 AND account_id = $2
	`, convoID, accountID).Scan(
		&existing.ID, &existing.AccountID, &existing.ConversationID, &existing.PipelineID,
		&existing.CurrentStateKey, &existing.Tags, &leadCreatedBy, &existing.CreatedAt, &existing.UpdatedAt,
	)
	if err == nil {
		existing.CreatedBy = leadCreatedBy
		// Already exists! Return the existing lead
		return &existing, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("check existing lead: %w", err)
	}

	// Fetch first state key of the lead pipeline
	var pipelineID uuid.UUID
	var statesJSON []byte
	err = tx.QueryRow(ctx, `SELECT id, states FROM lead_pipelines WHERE account_id = $1 ORDER BY created_at ASC LIMIT 1`, accountID).Scan(&pipelineID, &statesJSON)
	if err != nil {
		return nil, fmt.Errorf("get lead pipeline: %w", err)
	}
	var states []types.PipelineState
	if err := json.Unmarshal(statesJSON, &states); err != nil {
		return nil, fmt.Errorf("unmarshal pipeline states: %w", err)
	}
	if len(states) == 0 {
		return nil, fmt.Errorf("pipeline has no states configured")
	}
	firstStateKey := states[0].Key

	// Insert lead
	var leadID uuid.UUID
	var createdAt, updatedAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO leads (account_id, conversation_id, pipeline_id, current_state_key, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`, accountID, convoID, pipelineID, firstStateKey, userID).Scan(&leadID, &createdAt, &updatedAt)
	if err != nil {
		// Handle database unique constraint conflict gracefully just in case of concurrent insert
		var existing types.Lead
		err2 := tx.QueryRow(ctx, `
			SELECT id, account_id, conversation_id, pipeline_id, current_state_key, tags, created_by, created_at, updated_at
			FROM leads WHERE conversation_id = $1 AND account_id = $2
		`, convoID, accountID).Scan(
			&existing.ID, &existing.AccountID, &existing.ConversationID, &existing.PipelineID,
			&existing.CurrentStateKey, &existing.Tags, &leadCreatedBy, &existing.CreatedAt, &existing.UpdatedAt,
		)
		if err2 == nil {
			existing.CreatedBy = leadCreatedBy
			return &existing, nil
		}
		return nil, fmt.Errorf("create lead: %w", err)
	}

	// Insert lead_state_history
	_, err = tx.Exec(ctx, `
		INSERT INTO lead_state_history (account_id, lead_id, from_state, to_state, changed_by)
		VALUES ($1, $2, NULL, $3, $4)
	`, accountID, leadID, firstStateKey, userID)
	if err != nil {
		return nil, fmt.Errorf("insert lead state history: %w", err)
	}

	// Write audit log entry
	aw := audit.NewWriterFromTx(tx)
	err = aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &userID,
		Action:      "lead.created",
		TargetType:  "lead",
		TargetID:    &leadID,
		Metadata: map[string]any{
			"conversation_id":   convoID,
			"current_state_key": firstStateKey,
			"auto":              false,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("write audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create lead tx: %w", err)
	}

	return &types.Lead{
		ID:              leadID,
		AccountID:       accountID,
		ConversationID:  convoID,
		PipelineID:      pipelineID,
		CurrentStateKey: firstStateKey,
		Tags:            []string{},
		CreatedBy:       &userID,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}, nil
}

func (s *Service) getLeadAndCheckVisibility(ctx context.Context, accountID, userID uuid.UUID, leadID uuid.UUID, userRole string) (*types.Lead, error) {
	var lead types.Lead
	var leadCreatedBy *uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT id, account_id, conversation_id, pipeline_id, current_state_key, tags, created_by, created_at, updated_at
		FROM leads WHERE id = $1 AND account_id = $2
	`, leadID, accountID).Scan(
		&lead.ID, &lead.AccountID, &lead.ConversationID, &lead.PipelineID,
		&lead.CurrentStateKey, &lead.Tags, &leadCreatedBy, &lead.CreatedAt, &lead.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("lead not found")
		}
		return nil, err
	}
	lead.CreatedBy = leadCreatedBy

	// Check conversation visibility
	if err := s.checkConversationVisibility(ctx, accountID, userID, lead.ConversationID, userRole); err != nil {
		// Return "lead not found" to avoid leaking existence
		return nil, fmt.Errorf("lead not found")
	}

	return &lead, nil
}

func (s *Service) UpdateLeadState(ctx context.Context, accountID, userID uuid.UUID, leadID uuid.UUID, userRole string, targetStateKey string) (*types.Lead, error) {
	lead, err := s.getLeadAndCheckVisibility(ctx, accountID, userID, leadID, userRole)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Validate targetStateKey exists in the lead pipeline definition
	var statesJSON []byte
	err = tx.QueryRow(ctx, `SELECT states FROM lead_pipelines WHERE id = $1 AND account_id = $2`, lead.PipelineID, accountID).Scan(&statesJSON)
	if err != nil {
		return nil, fmt.Errorf("get pipeline: %w", err)
	}
	var states []types.PipelineState
	if err := json.Unmarshal(statesJSON, &states); err != nil {
		return nil, fmt.Errorf("unmarshal pipeline states: %w", err)
	}
	validState := false
	for _, st := range states {
		if st.Key == targetStateKey {
			validState = true
			break
		}
	}
	if !validState {
		return nil, fmt.Errorf("invalid state key: %q", targetStateKey)
	}

	// Update current_state_key and updated_at
	var updatedAt time.Time
	fromState := lead.CurrentStateKey
	err = tx.QueryRow(ctx, `
		UPDATE leads
		SET current_state_key = $1, updated_at = NOW()
		WHERE id = $2 AND account_id = $3
		RETURNING updated_at
	`, targetStateKey, leadID, accountID).Scan(&updatedAt)
	if err != nil {
		return nil, fmt.Errorf("update lead state: %w", err)
	}
	lead.CurrentStateKey = targetStateKey
	lead.UpdatedAt = updatedAt

	// Insert lead_state_history
	_, err = tx.Exec(ctx, `
		INSERT INTO lead_state_history (account_id, lead_id, from_state, to_state, changed_by)
		VALUES ($1, $2, $3, $4, $5)
	`, accountID, leadID, fromState, targetStateKey, userID)
	if err != nil {
		return nil, fmt.Errorf("insert lead state history: %w", err)
	}

	// Write audit log entry
	aw := audit.NewWriterFromTx(tx)
	err = aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &userID,
		Action:      "lead.state_changed",
		TargetType:  "lead",
		TargetID:    &leadID,
		Metadata: map[string]any{
			"from_state": fromState,
			"to_state":   targetStateKey,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("write audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update lead state tx: %w", err)
	}

	// Publish lead.state_changed event
	type LeadStateChangedPayload struct {
		Type           string  `json:"type"`
		ConversationID string  `json:"conversation_id"`
		LeadID         string  `json:"lead_id"`
		FromState      *string `json:"from_state"`
		ToState        string  `json:"to_state"`
	}
	_, err = s.pubsub.Publish(ctx, "lead.state_changed", LeadStateChangedPayload{
		Type:           "lead.state_changed",
		ConversationID: lead.ConversationID.String(),
		LeadID:         lead.ID.String(),
		FromState:      &fromState,
		ToState:        targetStateKey,
	})
	if err != nil {
		// Log error but don't fail (tx is committed)
		fmt.Printf("failed to publish lead.state_changed: %v\n", err)
	}

	return lead, nil
}

func (s *Service) UpdateLeadTags(ctx context.Context, accountID, userID uuid.UUID, leadID uuid.UUID, userRole string, tags []string) (*types.Lead, error) {
	lead, err := s.getLeadAndCheckVisibility(ctx, accountID, userID, leadID, userRole)
	if err != nil {
		return nil, err
	}

	if tags == nil {
		tags = []string{}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var updatedAt time.Time
	err = tx.QueryRow(ctx, `
		UPDATE leads
		SET tags = $1, updated_at = NOW()
		WHERE id = $2 AND account_id = $3
		RETURNING updated_at
	`, tags, leadID, accountID).Scan(&updatedAt)
	if err != nil {
		return nil, fmt.Errorf("update lead tags: %w", err)
	}
	lead.Tags = tags
	lead.UpdatedAt = updatedAt

	// Write audit log entry
	aw := audit.NewWriterFromTx(tx)
	err = aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &userID,
		Action:      "lead.tags_updated",
		TargetType:  "lead",
		TargetID:    &leadID,
		Metadata: map[string]any{
			"tags": tags,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("write audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update lead tags tx: %w", err)
	}

	return lead, nil
}

func (s *Service) CreateLeadNote(ctx context.Context, accountID, userID uuid.UUID, leadID uuid.UUID, userRole string, body string) (*types.LeadNote, error) {
	_, err := s.getLeadAndCheckVisibility(ctx, accountID, userID, leadID, userRole)
	if err != nil {
		return nil, err
	}

	if body == "" {
		return nil, fmt.Errorf("note body cannot be empty")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var noteID uuid.UUID
	var createdAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO lead_notes (account_id, lead_id, author_user_id, body)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`, accountID, leadID, userID, body).Scan(&noteID, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert lead note: %w", err)
	}

	// Write audit log entry
	aw := audit.NewWriterFromTx(tx)
	err = aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &userID,
		Action:      "lead.note_added",
		TargetType:  "lead",
		TargetID:    &leadID,
		Metadata: map[string]any{
			"note_id": noteID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("write audit log: %w", err)
	}

	var authorEmail string
	err = tx.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&authorEmail)
	if err != nil {
		return nil, fmt.Errorf("resolve author email: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create lead note tx: %w", err)
	}

	return &types.LeadNote{
		ID:           noteID,
		AccountID:    accountID,
		LeadID:       leadID,
		AuthorUserID: &userID,
		AuthorEmail:  authorEmail,
		Body:         body,
		CreatedAt:    createdAt,
	}, nil
}

func (s *Service) ListLeadNotes(ctx context.Context, accountID, userID uuid.UUID, leadID uuid.UUID, userRole string) ([]*types.LeadNote, error) {
	_, err := s.getLeadAndCheckVisibility(ctx, accountID, userID, leadID, userRole)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT ln.id, ln.account_id, ln.lead_id, ln.author_user_id, ln.body, ln.created_at, COALESCE(u.email, '')
		FROM lead_notes ln
		LEFT JOIN users u ON ln.author_user_id = u.id
		WHERE ln.lead_id = $1 AND ln.account_id = $2
		ORDER BY ln.created_at ASC
	`, leadID, accountID)
	if err != nil {
		return nil, fmt.Errorf("query lead notes: %w", err)
	}
	defer rows.Close()

	var notes []*types.LeadNote
	for rows.Next() {
		var n types.LeadNote
		var authorUserID *uuid.UUID
		if err := rows.Scan(&n.ID, &n.AccountID, &n.LeadID, &authorUserID, &n.Body, &n.CreatedAt, &n.AuthorEmail); err != nil {
			return nil, err
		}
		n.AuthorUserID = authorUserID
		notes = append(notes, &n)
	}
	if notes == nil {
		notes = []*types.LeadNote{}
	}
	return notes, rows.Err()
}

func (s *Service) ListLeadHistory(ctx context.Context, accountID, userID uuid.UUID, leadID uuid.UUID, userRole string) ([]*types.LeadStateHistory, error) {
	_, err := s.getLeadAndCheckVisibility(ctx, accountID, userID, leadID, userRole)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT lsh.id, lsh.account_id, lsh.lead_id, lsh.from_state, lsh.to_state, lsh.changed_by, lsh.changed_at, COALESCE(u.email, '')
		FROM lead_state_history lsh
		LEFT JOIN users u ON lsh.changed_by = u.id
		WHERE lsh.lead_id = $1 AND lsh.account_id = $2
		ORDER BY lsh.changed_at ASC
	`, leadID, accountID)
	if err != nil {
		return nil, fmt.Errorf("query lead history: %w", err)
	}
	defer rows.Close()

	var history []*types.LeadStateHistory
	for rows.Next() {
		var h types.LeadStateHistory
		var fromState *string
		var changedBy *uuid.UUID
		if err := rows.Scan(&h.ID, &h.AccountID, &h.LeadID, &fromState, &h.ToState, &changedBy, &h.ChangedAt, &h.ActorEmail); err != nil {
			return nil, err
		}
		h.FromState = fromState
		h.ChangedBy = changedBy
		history = append(history, &h)
	}
	if history == nil {
		history = []*types.LeadStateHistory{}
	}
	return history, rows.Err()
}

func (s *Service) ReadConversation(ctx context.Context, accountID, userID, conversationID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO conversation_reads (account_id, conversation_id, user_id, last_read_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (conversation_id, user_id)
		DO UPDATE SET last_read_at = NOW()
	`, accountID, conversationID, userID)
	if err != nil {
		return fmt.Errorf("upsert conversation read: %w", err)
	}
	return nil
}

func encodeCursor(t time.Time, id uuid.UUID) string {
	str := fmt.Sprintf("%s,%s", t.Format(time.RFC3339Nano), id.String())
	return base64.URLEncoding.EncodeToString([]byte(str))
}

func decodeCursor(cursorStr string) (time.Time, uuid.UUID, error) {
	b, err := base64.URLEncoding.DecodeString(cursorStr)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	parts := strings.SplitN(string(b), ",", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor format")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	return t, id, nil
}

func (s *Service) PubSub() *pubsub.Client {
	return s.pubsub
}

func (s *Service) Pool() *pgxpool.Pool {
	return s.pool
}


