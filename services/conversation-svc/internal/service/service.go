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
	timestamp := event.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO conversations (account_id, contact_id, channel_id, last_message_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (contact_id, channel_id)
		DO UPDATE SET
			last_message_at = EXCLUDED.last_message_at
		RETURNING id
	`, accountID, contactID, channelID, timestamp).Scan(&conversationID)
	if err != nil {
		return fmt.Errorf("upsert conversation: %w", err)
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

	// Call adapter SendMessage
	msgPayload := types.NormalizedMessage{
		ContentType: contentType,
		Text:        text,
		MediaURL:    mediaURL,
	}

	// Call adapter
	err = adapter.SendMessage(ctx, channelID.String(), externalIdentity, msgPayload)
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

	// 2. Persist outbound message in DB
	msg := &types.Message{
		AccountID:      accountID,
		ConversationID: conversationID,
		Direction:      "outbound",
		SenderType:     senderType,
		SenderUserID:   senderUserID,
		ContentType:    contentType,
		Content:        contentRaw,
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO messages (account_id, conversation_id, direction, sender_type, sender_user_id, content_type, content, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		RETURNING id, created_at
	`, msg.AccountID, msg.ConversationID, msg.Direction, msg.SenderType, msg.SenderUserID, msg.ContentType, msg.Content).
		Scan(&msg.ID, &msg.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert outbound message: %w", err)
	}

	// 3. Update conversation last_message_at
	_, err = tx.Exec(ctx, `
		UPDATE conversations
		SET last_message_at = $1
		WHERE id = $2 AND account_id = $3
	`, msg.CreatedAt, conversationID, accountID)
	if err != nil {
		return nil, fmt.Errorf("update conversation last_message_at: %w", err)
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

func (s *Service) ListConversations(ctx context.Context, accountID, userID uuid.UUID, userRole string, filter string) ([]*types.ConversationListItem, error) {
	var settingsBytes []byte
	err := s.pool.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1`, accountID).Scan(&settingsBytes)
	if err != nil {
		return nil, fmt.Errorf("get account settings: %w", err)
	}
	unassignedVisible := types.IsUnassignedVisible(settingsBytes)

	// Base SQL query selecting conversations with contact info, unread flag and lateral last message.
	sqlQuery := `
		SELECT c.id, c.account_id, c.contact_id, c.channel_id, c.status, c.assigned_user_ids, c.last_message_at, c.ai_mode_active, c.created_at,
		       co.display_name, co.avatar_url,
		       cr.last_read_at,
		       m.id as msg_id, m.direction as msg_direction, m.sender_type as msg_sender_type, m.sender_user_id as msg_sender_user_id,
		       m.content_type as msg_content_type, m.content as msg_content, m.external_message_id as msg_external_id, m.created_at as msg_created_at
		FROM conversations c
		JOIN contacts co ON c.contact_id = co.id
		LEFT JOIN conversation_reads cr ON c.id = cr.conversation_id AND cr.user_id = $1
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
	if filter == "mine" {
		sqlQuery += ` AND $1 = ANY(c.assigned_user_ids)`
	} else if filter == "unassigned" {
		sqlQuery += ` AND (c.assigned_user_ids IS NULL OR cardinality(c.assigned_user_ids) = 0)`
	}

	sqlQuery += ` ORDER BY c.last_message_at DESC NULLS LAST, c.created_at DESC`

	rows, err := s.pool.Query(ctx, sqlQuery, userID, accountID, userRole, unassignedVisible)
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

	// Fetch single conversation.
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

	err = s.pool.QueryRow(ctx, `
		SELECT c.id, c.account_id, c.contact_id, c.channel_id, c.status, c.assigned_user_ids, c.last_message_at, c.ai_mode_active, c.created_at,
		       co.display_name, co.avatar_url,
		       cr.last_read_at,
		       m.id as msg_id, m.direction as msg_direction, m.sender_type as msg_sender_type, m.sender_user_id as msg_sender_user_id,
		       m.content_type as msg_content_type, m.content as msg_content, m.external_message_id as msg_external_id, m.created_at as msg_created_at
		FROM conversations c
		JOIN contacts co ON c.contact_id = co.id
		LEFT JOIN conversation_reads cr ON c.id = cr.conversation_id AND cr.user_id = $1
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

