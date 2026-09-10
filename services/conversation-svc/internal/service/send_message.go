package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	matrixadapter "github.com/whatfunnel/whatfunnel/adapters/matrix-mautrix"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

type sendMessageCommand struct {
	accountID       uuid.UUID
	conversationID  uuid.UUID
	sender          types.MessageSender
	senderUserID    *uuid.UUID
	contentType     string
	text            string
	mediaURL        string
	aiReplyDraftID  *uuid.UUID
	generationEpoch *int64
	purpose         types.MessagePurpose
	idempotencyKey  string
}

func (c sendMessageCommand) human() bool { return c.sender == types.MessageSenderHuman }
func (c sendMessageCommand) ai() bool    { return c.sender == types.MessageSenderAI }

func (c sendMessageCommand) validate() error {
	if !c.human() && !c.ai() {
		return fmt.Errorf("invalid sender_type: %q", c.sender)
	}
	if c.aiReplyDraftID != nil && !c.human() {
		return errors.New("AI reply drafts can only be used by a human sender")
	}
	if c.ai() && c.generationEpoch == nil {
		return errors.New("generation_epoch is required for AI messages")
	}
	return nil
}

type outboundDestination struct {
	channelID        uuid.UUID
	channelType      string
	externalIdentity string
}

// SendMessage sends an outbound message via the registered adapter and records
// it in the database within a single transaction.
func (s *Service) SendMessage(
	ctx context.Context,
	accountID, conversationID uuid.UUID,
	senderType string,
	senderUserID *uuid.UUID,
	contentType, text, mediaURL string,
	aiReplyDraftID *uuid.UUID,
	generationEpoch *int64,
	messagePurpose, idempotencyKey string,
) (*types.Message, error) {
	return s.sendMessage(ctx, sendMessageCommand{
		accountID: accountID, conversationID: conversationID,
		sender: types.MessageSender(senderType), senderUserID: senderUserID,
		contentType: contentType, text: text, mediaURL: mediaURL,
		aiReplyDraftID: aiReplyDraftID, generationEpoch: generationEpoch,
		purpose: types.MessagePurpose(messagePurpose), idempotencyKey: idempotencyKey,
	})
}

func (s *Service) sendMessage(ctx context.Context, cmd sendMessageCommand) (*types.Message, error) {
	if err := cmd.validate(); err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin send tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if existing, err := findIdempotentMessage(ctx, tx, cmd); err != nil || existing != nil {
		return existing, err
	}
	if err = authorizeAIMessage(ctx, tx, cmd); err != nil {
		return nil, err
	}
	destination, err := loadOutboundDestination(ctx, tx, cmd.accountID, cmd.conversationID)
	if err != nil {
		return nil, err
	}
	if err = lockReplyDraft(ctx, tx, cmd); err != nil {
		return nil, err
	}

	externalMessageID, err := s.sendViaAdapter(ctx, tx, destination, cmd)
	if err != nil {
		return nil, err
	}
	msg, err := insertOutboundMessage(ctx, tx, cmd, externalMessageID)
	if err != nil {
		return nil, err
	}
	invalidatedDraftID, err := applyOutboundMessageEffects(ctx, tx, cmd, msg)
	if err != nil {
		return nil, err
	}
	if err = writeMessageSentAudit(ctx, tx, cmd, msg); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit send tx: %w", err)
	}

	s.publishOutboundMessageEvents(ctx, cmd, msg, invalidatedDraftID)
	return msg, nil
}

func findIdempotentMessage(ctx context.Context, tx pgx.Tx, cmd sendMessageCommand) (*types.Message, error) {
	if cmd.idempotencyKey == "" {
		return nil, nil
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, cmd.accountID.String()+":"+cmd.idempotencyKey); err != nil {
		return nil, fmt.Errorf("lock message idempotency key: %w", err)
	}
	var existing types.Message
	err := tx.QueryRow(ctx, `
		SELECT id, account_id, conversation_id, direction, sender_type, sender_user_id,
		       content_type, content, external_message_id, idempotency_key, created_at
		FROM messages
		WHERE account_id = $1 AND idempotency_key = $2
	`, cmd.accountID, cmd.idempotencyKey).Scan(
		&existing.ID, &existing.AccountID, &existing.ConversationID, &existing.Direction,
		&existing.SenderType, &existing.SenderUserID, &existing.ContentType,
		&existing.Content, &existing.ExternalMessageID, &existing.IdempotencyKey, &existing.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("check message idempotency key: %w", err)
	}
	return &existing, nil
}

func authorizeAIMessage(ctx context.Context, tx pgx.Tx, cmd sendMessageCommand) error {
	if !cmd.ai() {
		return nil
	}
	var state types.AIState
	var currentEpoch int64
	err := tx.QueryRow(ctx, `
		SELECT state, generation_epoch
		FROM conversation_ai_state
		WHERE conversation_id = $1 AND account_id = $2
		FOR UPDATE
	`, cmd.conversationID, cmd.accountID).Scan(&state, &currentEpoch)
	if err != nil {
		return fmt.Errorf("lock conversation AI state: %w", err)
	}
	allowed := cmd.purpose == types.MessagePurposeReply && state == types.AIStateActive
	if cmd.purpose == types.MessagePurposeHumanReviewAck {
		allowed = state == types.AIStateCooldown || state == types.AIStateReviewRequired
	}
	if !allowed || currentEpoch != *cmd.generationEpoch {
		return errors.New("stale or unauthorized AI message")
	}
	return nil
}

func loadOutboundDestination(ctx context.Context, tx pgx.Tx, accountID, conversationID uuid.UUID) (outboundDestination, error) {
	var destination outboundDestination
	err := tx.QueryRow(ctx, `
		SELECT c.channel_id, ch.type, co.external_identity
		FROM conversations c
		JOIN channels ch ON c.channel_id = ch.id
		JOIN contacts co ON c.contact_id = co.id
		WHERE c.id = $1 AND c.account_id = $2
	`, conversationID, accountID).Scan(&destination.channelID, &destination.channelType, &destination.externalIdentity)
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundDestination{}, errors.New("conversation not found")
	}
	if err != nil {
		return outboundDestination{}, fmt.Errorf("lookup conversation details: %w", err)
	}
	return destination, nil
}

func lockReplyDraft(ctx context.Context, tx pgx.Tx, cmd sendMessageCommand) error {
	if cmd.aiReplyDraftID == nil {
		return nil
	}
	var draftID uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM ai_reply_drafts
		WHERE id = $1 AND account_id = $2 AND conversation_id = $3 AND status = 'pending'
		FOR UPDATE
	`, *cmd.aiReplyDraftID, cmd.accountID, cmd.conversationID).Scan(&draftID)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("reply draft not found")
	}
	if err != nil {
		return fmt.Errorf("lock AI reply draft: %w", err)
	}
	return nil
}

func (s *Service) sendViaAdapter(ctx context.Context, tx pgx.Tx, destination outboundDestination, cmd sendMessageCommand) (string, error) {
	adapter, err := s.GetAdapter(destination.channelType)
	if err != nil {
		return "", err
	}

	var dbCredentials []byte
	var bridgeIdentity string
	if err = tx.QueryRow(ctx, `SELECT bridge_credentials, COALESCE(bridge_identity, '') FROM channels WHERE id = $1`, destination.channelID).Scan(&dbCredentials, &bridgeIdentity); err == nil && len(dbCredentials) > 0 {
		if decrypted, decryptErr := s.DecryptCredentials(dbCredentials); decryptErr == nil {
			if configurable, ok := adapter.(interface {
				Configure(channelID string, config matrixadapter.ChannelConfig)
			}); ok {
				var credentials matrixadapter.Credentials
				if json.Unmarshal(decrypted, &credentials) == nil {
					configurable.Configure(destination.channelID.String(), matrixadapter.ChannelConfig{
						Credentials: credentials, BridgeIdentity: bridgeIdentity,
					})
				}
			}
		}
	}

	externalMessageID, err := adapter.SendMessage(ctx, destination.channelID.String(), destination.externalIdentity, types.NormalizedMessage{
		ContentType: cmd.contentType, Text: cmd.text, MediaURL: cmd.mediaURL,
	})
	if err != nil {
		return "", fmt.Errorf("adapter send failed: %w", err)
	}
	return externalMessageID, nil
}

func insertOutboundMessage(ctx context.Context, tx pgx.Tx, cmd sendMessageCommand, externalMessageID string) (*types.Message, error) {
	content, err := json.Marshal(map[string]any{"text": cmd.text, "media_url": cmd.mediaURL})
	if err != nil {
		return nil, fmt.Errorf("marshal outbound message: %w", err)
	}
	msg := &types.Message{
		AccountID: cmd.accountID, ConversationID: cmd.conversationID, Direction: "outbound",
		SenderType: cmd.sender, SenderUserID: cmd.senderUserID, ContentType: cmd.contentType, Content: content,
	}
	if externalMessageID != "" {
		msg.ExternalMessageID = &externalMessageID
	}
	if cmd.idempotencyKey != "" {
		msg.IdempotencyKey = &cmd.idempotencyKey
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO messages (account_id, conversation_id, direction, sender_type, sender_user_id, content_type, content, external_message_id, idempotency_key, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		RETURNING id, created_at
	`, msg.AccountID, msg.ConversationID, msg.Direction, msg.SenderType, msg.SenderUserID, msg.ContentType, msg.Content, msg.ExternalMessageID, msg.IdempotencyKey).Scan(&msg.ID, &msg.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert outbound message: %w", err)
	}
	return msg, nil
}

func applyOutboundMessageEffects(ctx context.Context, tx pgx.Tx, cmd sendMessageCommand, msg *types.Message) (*uuid.UUID, error) {
	if _, err := tx.Exec(ctx, `UPDATE conversations SET last_message_at = $1 WHERE id = $2 AND account_id = $3`, msg.CreatedAt, cmd.conversationID, cmd.accountID); err != nil {
		return nil, fmt.Errorf("update conversation details: %w", err)
	}
	if !cmd.human() {
		return nil, nil
	}
	if err := pauseAIAfterHumanMessage(ctx, tx, cmd.accountID, cmd.conversationID, cmd.senderUserID, msg.ID, types.AIStateReasonHumanMessageSent); err != nil {
		return nil, err
	}
	return updateReplyDraftAfterHumanMessage(ctx, tx, cmd, msg.ID)
}

func updateReplyDraftAfterHumanMessage(ctx context.Context, tx pgx.Tx, cmd sendMessageCommand, messageID uuid.UUID) (*uuid.UUID, error) {
	if cmd.aiReplyDraftID != nil {
		_, err := tx.Exec(ctx, `
			UPDATE ai_reply_drafts SET status = 'used', used_message_id = $1, updated_at = NOW()
			WHERE id = $2 AND account_id = $3 AND conversation_id = $4 AND status = 'pending'
		`, messageID, *cmd.aiReplyDraftID, cmd.accountID, cmd.conversationID)
		if err != nil {
			return nil, fmt.Errorf("update AI reply draft: %w", err)
		}
		return cmd.aiReplyDraftID, nil
	}

	var draftID uuid.UUID
	err := tx.QueryRow(ctx, `
		UPDATE ai_reply_drafts SET status = 'superseded', updated_at = NOW()
		WHERE account_id = $1 AND conversation_id = $2 AND status = 'pending'
		RETURNING id
	`, cmd.accountID, cmd.conversationID).Scan(&draftID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update AI reply draft: %w", err)
	}
	return &draftID, nil
}

func writeMessageSentAudit(ctx context.Context, tx pgx.Tx, cmd sendMessageCommand, msg *types.Message) error {
	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID: cmd.accountID, ActorUserID: cmd.senderUserID,
		Action: "message.sent", TargetType: "message", TargetID: &msg.ID,
		Metadata: map[string]any{"conversation_id": cmd.conversationID, "content_type": cmd.contentType, "sender_type": cmd.sender},
	}); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}
	return nil
}

func (s *Service) publishOutboundMessageEvents(ctx context.Context, cmd sendMessageCommand, msg *types.Message, draftID *uuid.UUID) {
	if _, err := s.pubsub.Publish(ctx, "conversation.updated", ConversationUpdatedEvent{AccountID: cmd.accountID, ConversationID: cmd.conversationID, MessageID: msg.ID}); err != nil {
		fmt.Printf("failed to publish conversation.updated for outbound send: %v\n", err)
	}
	if !cmd.human() || draftID == nil {
		return
	}
	action := "superseded"
	if cmd.aiReplyDraftID != nil {
		action = "used"
	}
	if _, err := s.pubsub.Publish(ctx, "ai.reply_draft.updated", AIReplyDraftUpdatedEvent{
		AccountID: cmd.accountID, ConversationID: cmd.conversationID, DraftID: draftID, Action: action,
	}); err != nil {
		fmt.Printf("failed to publish AI reply draft update: %v\n", err)
	}
}
