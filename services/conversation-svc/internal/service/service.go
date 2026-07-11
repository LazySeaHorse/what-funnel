package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
