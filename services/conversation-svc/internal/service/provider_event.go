package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

type providerEventResult struct {
	accountID      uuid.UUID
	conversationID uuid.UUID
	messageID      uuid.UUID
}

// IngestProviderEvent is the single provider-neutral ingress boundary. The
// event marker and every domain mutation commit atomically, making Redis
// redelivery safe.
func (s *Service) IngestProviderEvent(ctx context.Context, event messaging.Event) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate provider event: %w", err)
	}
	channelID, err := uuid.Parse(event.ChannelID)
	if err != nil {
		return fmt.Errorf("parse provider channel id: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin provider event: %w", err)
	}
	defer tx.Rollback(ctx)

	var accountID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT account_id
		FROM channels
		WHERE id = $1 AND provider = $2
	`, channelID, event.Provider).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("provider channel not found")
	}
	if err != nil {
		return fmt.Errorf("resolve provider channel: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO processed_adapter_events (provider, event_id, channel_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (provider, event_id) DO NOTHING
	`, event.Provider, event.ID, channelID)
	if err != nil {
		return fmt.Errorf("record provider event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}

	result := providerEventResult{accountID: accountID}
	switch event.Kind {
	case messaging.EventMessageCreated:
		result, err = ingestProviderMessage(ctx, tx, accountID, channelID, event)
	case messaging.EventMessageEdited:
		result, err = editProviderMessage(ctx, tx, accountID, channelID, event)
	case messaging.EventMessageDeleted:
		result, err = deleteProviderMessage(ctx, tx, accountID, channelID, event)
	case messaging.EventReactionChanged:
		result, err = changeProviderReaction(ctx, tx, accountID, channelID, event)
	case messaging.EventReceiptChanged:
		result, err = applyProviderReceipt(ctx, tx, accountID, channelID, event)
	case messaging.EventChannelStatus:
		err = applyProviderStatus(ctx, tx, accountID, channelID, event)
	default:
		err = messaging.ErrInvalidEnvelope
	}
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit provider event: %w", err)
	}

	if result.conversationID != uuid.Nil && result.messageID != uuid.Nil {
		if _, err := s.pubsub.Publish(ctx, "conversation.updated", ConversationUpdatedEvent{
			AccountID: result.accountID, ConversationID: result.conversationID, MessageID: result.messageID,
		}); err != nil {
			fmt.Printf("failed to publish conversation.updated for provider event: %v\n", err)
		}
	}
	return nil
}

func ingestProviderMessage(
	ctx context.Context,
	tx pgx.Tx,
	accountID, channelID uuid.UUID,
	event messaging.Event,
) (providerEventResult, error) {
	message := event.Message
	if message.Direction == messaging.DirectionOutbound && event.CorrelationID != "" {
		localMessageID, err := uuid.Parse(event.CorrelationID)
		if err != nil {
			return providerEventResult{}, fmt.Errorf("parse correlated message id: %w", err)
		}
		var conversationID uuid.UUID
		err = tx.QueryRow(ctx, `
			UPDATE messages AS message
			SET provider_message_id = $1,
			    external_message_id = $1,
			    provider_timestamp = $2,
			    delivery_status = 'sent',
			    delivery_detail = NULL
			FROM conversations AS conversation
			WHERE message.id = $3
			  AND message.account_id = $4
			  AND message.conversation_id = conversation.id
			  AND conversation.channel_id = $5
			RETURNING message.conversation_id
		`, message.ProviderMessageID, message.ProviderTimestamp, localMessageID, accountID, channelID).Scan(&conversationID)
		if errors.Is(err, pgx.ErrNoRows) {
			return providerEventResult{}, errors.New("correlated outbound message not found")
		}
		if err != nil {
			return providerEventResult{}, fmt.Errorf("confirm outbound provider message: %w", err)
		}
		return providerEventResult{accountID: accountID, conversationID: conversationID, messageID: localMessageID}, nil
	}

	_, conversationID, isNew, err := upsertProviderConversation(ctx, tx, accountID, channelID, *message)
	if err != nil {
		return providerEventResult{}, err
	}
	if isNew {
		if _, err := createInitialLeadIfEnabled(ctx, tx, accountID, conversationID); err != nil {
			return providerEventResult{}, err
		}
	}

	content, err := providerMessageContent(*message, uuid.Nil)
	if err != nil {
		return providerEventResult{}, err
	}
	var replyToMessageID *uuid.UUID
	if message.ReplyToProviderID != "" {
		var replyID uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT id FROM messages
			WHERE conversation_id = $1 AND provider_message_id = $2
		`, conversationID, message.ReplyToProviderID).Scan(&replyID)
		if err == nil {
			replyToMessageID = &replyID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return providerEventResult{}, fmt.Errorf("resolve provider reply: %w", err)
		}
	}

	senderType := types.MessageSenderContact
	if message.Direction == messaging.DirectionOutbound {
		senderType = types.MessageSenderHuman
	}
	var messageID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO messages (
			account_id, conversation_id, direction, sender_type, content_type, content,
			external_message_id, provider_message_id, reply_to_message_id,
			delivery_status, provider_timestamp, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $8, 'sent', $9, $9)
		RETURNING id
	`, accountID, conversationID, message.Direction, senderType, message.ContentType, content,
		message.ProviderMessageID, replyToMessageID, message.ProviderTimestamp).Scan(&messageID)
	if err != nil {
		return providerEventResult{}, fmt.Errorf("insert provider message: %w", err)
	}
	if err := insertProviderMedia(ctx, tx, accountID, channelID, messageID, message); err != nil {
		return providerEventResult{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE conversations SET last_message_at = GREATEST(COALESCE(last_message_at, $1), $1)
		WHERE id = $2
	`, message.ProviderTimestamp, conversationID); err != nil {
		return providerEventResult{}, fmt.Errorf("update provider conversation: %w", err)
	}
	if message.Direction == messaging.DirectionOutbound {
		if err := pauseAIAfterHumanMessage(ctx, tx, accountID, conversationID, nil, messageID, types.AIStateReasonExternalHumanMessage); err != nil {
			return providerEventResult{}, err
		}
	}
	return providerEventResult{accountID: accountID, conversationID: conversationID, messageID: messageID}, nil
}

func upsertProviderConversation(
	ctx context.Context,
	tx pgx.Tx,
	accountID, channelID uuid.UUID,
	message messaging.Message,
) (uuid.UUID, uuid.UUID, bool, error) {
	displayName := message.Sender.DisplayName
	if message.Direction == messaging.DirectionOutbound {
		displayName = ""
	}
	var contactID uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO contacts (account_id, channel_id, external_identity, display_name, avatar_url)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''))
		ON CONFLICT (channel_id, external_identity) DO UPDATE
		SET display_name = COALESCE(EXCLUDED.display_name, contacts.display_name),
		    avatar_url = COALESCE(EXCLUDED.avatar_url, contacts.avatar_url)
		RETURNING id
	`, accountID, channelID, message.ExternalThreadID, displayName, message.Sender.AvatarURL).Scan(&contactID)
	if err != nil {
		return uuid.Nil, uuid.Nil, false, fmt.Errorf("upsert provider contact: %w", err)
	}

	var conversationID uuid.UUID
	isNew := false
	err = tx.QueryRow(ctx, `
		SELECT id FROM conversations
		WHERE channel_id = $1 AND external_thread_id = $2
	`, channelID, message.ExternalThreadID).Scan(&conversationID)
	if errors.Is(err, pgx.ErrNoRows) {
		isNew = true
		err = tx.QueryRow(ctx, `
			INSERT INTO conversations (account_id, contact_id, channel_id, external_thread_id, last_message_at)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id
		`, accountID, contactID, channelID, message.ExternalThreadID, message.ProviderTimestamp).Scan(&conversationID)
	}
	if err != nil {
		return uuid.Nil, uuid.Nil, false, fmt.Errorf("upsert provider conversation: %w", err)
	}
	return contactID, conversationID, isNew, nil
}

func providerMessageContent(message messaging.Message, mediaID uuid.UUID) ([]byte, error) {
	content := map[string]any{
		"text":        message.Text,
		"notice_code": message.NoticeCode,
	}
	if mediaID != uuid.Nil {
		content["media_id"] = mediaID
	}
	data, err := json.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("marshal provider message: %w", err)
	}
	return data, nil
}

func insertProviderMedia(
	ctx context.Context,
	tx pgx.Tx,
	accountID, channelID, messageID uuid.UUID,
	message *messaging.Message,
) error {
	if message.Media == nil {
		return nil
	}
	retention, err := messaging.RetentionForSize(message.Media.SizeBytes)
	if err != nil {
		return err
	}
	mediaID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO media_objects (
			id, account_id, message_id, channel_id, provider_ref, filename,
			mime_type, size_bytes, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8, $9)
	`, mediaID, accountID, messageID, channelID, message.Media.ProviderRef,
		message.Media.Filename, message.Media.MIMEType, message.Media.SizeBytes,
		time.Now().UTC().Add(retention)); err != nil {
		return fmt.Errorf("insert provider media: %w", err)
	}
	content, err := providerMessageContent(*message, mediaID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE messages SET content = $1 WHERE id = $2`, content, messageID); err != nil {
		return fmt.Errorf("attach provider media: %w", err)
	}
	return nil
}

func editProviderMessage(ctx context.Context, tx pgx.Tx, accountID, channelID uuid.UUID, event messaging.Event) (providerEventResult, error) {
	content, err := providerMessageContent(*event.Message, uuid.Nil)
	if err != nil {
		return providerEventResult{}, err
	}
	var result providerEventResult
	result.accountID = accountID
	err = tx.QueryRow(ctx, `
		UPDATE messages AS message
		SET content_type = $1, content = $2, edited_at = $3
		FROM conversations AS conversation
		WHERE message.provider_message_id = $4
		  AND message.account_id = $5
		  AND message.conversation_id = conversation.id
		  AND conversation.channel_id = $6
		RETURNING message.id, message.conversation_id
	`, event.Message.ContentType, content, event.OccurredAt, event.Message.ProviderMessageID, accountID, channelID).
		Scan(&result.messageID, &result.conversationID)
	if err != nil {
		return providerEventResult{}, fmt.Errorf("edit provider message: %w", err)
	}
	return result, nil
}

func deleteProviderMessage(ctx context.Context, tx pgx.Tx, accountID, channelID uuid.UUID, event messaging.Event) (providerEventResult, error) {
	content, _ := json.Marshal(map[string]any{"text": "This message was deleted.", "notice_code": "deleted"})
	result := providerEventResult{accountID: accountID}
	err := tx.QueryRow(ctx, `
		UPDATE messages AS message
		SET content_type = 'notice', content = $1, deleted_at = $2
		FROM conversations AS conversation
		WHERE message.provider_message_id = $3
		  AND message.account_id = $4
		  AND message.conversation_id = conversation.id
		  AND conversation.channel_id = $5
		RETURNING message.id, message.conversation_id
	`, content, event.OccurredAt, event.Message.ProviderMessageID, accountID, channelID).
		Scan(&result.messageID, &result.conversationID)
	if err != nil {
		return providerEventResult{}, fmt.Errorf("delete provider message: %w", err)
	}
	return result, nil
}

func changeProviderReaction(ctx context.Context, tx pgx.Tx, accountID, channelID uuid.UUID, event messaging.Event) (providerEventResult, error) {
	result := providerEventResult{accountID: accountID}
	err := tx.QueryRow(ctx, `
		SELECT message.id, message.conversation_id
		FROM messages AS message
		JOIN conversations AS conversation ON conversation.id = message.conversation_id
		WHERE message.provider_message_id = $1
		  AND message.account_id = $2
		  AND conversation.channel_id = $3
	`, event.Reaction.ProviderMessageID, accountID, channelID).Scan(&result.messageID, &result.conversationID)
	if err != nil {
		return providerEventResult{}, fmt.Errorf("resolve reaction message: %w", err)
	}
	if event.Reaction.Removed {
		_, err = tx.Exec(ctx, `
			DELETE FROM message_reactions
			WHERE message_id = $1 AND sender_external_id = $2
		`, result.messageID, event.Reaction.SenderExternalID)
	} else {
		_, err = tx.Exec(ctx, `
			INSERT INTO message_reactions (
				account_id, message_id, sender_external_id, emoji, provider_timestamp
			)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (message_id, sender_external_id) DO UPDATE
			SET emoji = EXCLUDED.emoji,
			    provider_timestamp = EXCLUDED.provider_timestamp,
			    updated_at = NOW()
		`, accountID, result.messageID, event.Reaction.SenderExternalID,
			event.Reaction.Emoji, event.Reaction.ProviderTimestamp)
	}
	if err != nil {
		return providerEventResult{}, fmt.Errorf("change provider reaction: %w", err)
	}
	return result, nil
}

func applyProviderReceipt(ctx context.Context, tx pgx.Tx, accountID, channelID uuid.UUID, event messaging.Event) (providerEventResult, error) {
	result := providerEventResult{accountID: accountID}
	err := tx.QueryRow(ctx, `
		UPDATE messages AS message
		SET delivery_status = CASE
			WHEN message.delivery_status = 'failed' THEN message.delivery_status
			WHEN $1 = 'failed' THEN 'failed'
			WHEN ARRAY_POSITION(ARRAY['queued','sent','delivered','read'], $1::TEXT)
			   > ARRAY_POSITION(ARRAY['queued','sent','delivered','read'], message.delivery_status)
			THEN $1
			ELSE message.delivery_status
		END,
		delivery_detail = NULLIF($2, '')
		FROM conversations AS conversation
		WHERE message.provider_message_id = $3
		  AND message.account_id = $4
		  AND message.conversation_id = conversation.id
		  AND conversation.channel_id = $5
		RETURNING message.id, message.conversation_id
	`, event.Receipt.Status, event.Receipt.Detail, event.Receipt.ProviderMessageID, accountID, channelID).
		Scan(&result.messageID, &result.conversationID)
	if err != nil {
		return providerEventResult{}, fmt.Errorf("apply provider receipt: %w", err)
	}
	return result, nil
}

func applyProviderStatus(ctx context.Context, tx pgx.Tx, accountID, channelID uuid.UUID, event messaging.Event) error {
	_, err := tx.Exec(ctx, `
		UPDATE provider_connections
		SET state = $1, detail = NULLIF($2, ''), updated_at = $3
		WHERE channel_id = $4 AND account_id = $5
	`, event.Status.State, event.Status.Detail, event.OccurredAt, channelID, accountID)
	if err != nil {
		return fmt.Errorf("update provider connection status: %w", err)
	}
	_, err = tx.Exec(ctx, `
		UPDATE channels
		SET status = $1, status_detail = NULLIF($2, ''), updated_at = $3
		WHERE id = $4 AND account_id = $5
	`, event.Status.State, event.Status.Detail, event.OccurredAt, channelID, accountID)
	if err != nil {
		return fmt.Errorf("update provider channel status: %w", err)
	}
	return nil
}
