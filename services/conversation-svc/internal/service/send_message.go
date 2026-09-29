package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// SendMessageParams contains all arguments for sending an outbound message.
type SendMessageParams struct {
	AccountID         uuid.UUID
	ConversationID    uuid.UUID
	Sender            types.MessageSender
	SenderUserID      *uuid.UUID
	ContentType       string
	Text              string
	MediaID           string
	ReplyToMessageID  *uuid.UUID
	ReplyToProviderID string
	AIReplyDraftID    *uuid.UUID
	GenerationEpoch   *int64
	Purpose           types.MessagePurpose
	IdempotencyKey    string
}

func (c SendMessageParams) human() bool { return c.Sender == types.MessageSenderHuman }
func (c SendMessageParams) ai() bool    { return c.Sender == types.MessageSenderAI }

func (c SendMessageParams) validate() error {
	if !c.human() && !c.ai() {
		return invalidf("invalid sender_type: %q", c.Sender)
	}
	if c.AIReplyDraftID != nil && !c.human() {
		return invalidf("AI reply drafts can only be used by a human sender")
	}
	if c.ai() && c.GenerationEpoch == nil {
		return invalidf("generation_epoch is required for AI messages")
	}
	return nil
}

type outboundDestination struct {
	channelID uuid.UUID
	// simulated channels are local development stand-ins with no provider
	// session; their messages must never enter the adapter command streams.
	simulated        bool
	provider         messaging.Provider
	externalIdentity string
	capabilities     messaging.Capabilities
}

// SendMessage sends an outbound message via the registered adapter and records
// it in the database within a single transaction.
func (s *ConversationService) SendMessage(ctx context.Context, params SendMessageParams) (*types.Message, error) {
	return s.sendMessage(ctx, params)
}

func (s *ConversationService) sendMessage(ctx context.Context, cmd SendMessageParams) (*types.Message, error) {
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
	destination, err := loadOutboundDestination(ctx, tx, cmd.AccountID, cmd.ConversationID)
	if err != nil {
		return nil, err
	}
	if err = authorizeAIMessage(ctx, tx, cmd); err != nil {
		return nil, err
	}
	if err = lockReplyDraft(ctx, tx, cmd); err != nil {
		return nil, err
	}
	if cmd.ReplyToMessageID != nil && !destination.capabilities.Replies {
		return nil, invalidf("this messaging provider does not support replies")
	}
	if err = resolveReplyTarget(ctx, tx, &cmd); err != nil {
		return nil, err
	}

	msg, err := insertOutboundMessage(ctx, tx, cmd, "")
	if err != nil {
		return nil, err
	}
	if destination.simulated {
		if err := markSimulatedMessageSent(ctx, tx, msg); err != nil {
			return nil, err
		}
	} else if err := insertOutboxCommand(ctx, tx, destination, cmd, msg); err != nil {
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
	if !destination.simulated {
		s.outbox.nudgeOutbox(ctx, "send:"+msg.ID.String())
	}
	return msg, nil
}

func findIdempotentMessage(ctx context.Context, tx pgx.Tx, cmd SendMessageParams) (*types.Message, error) {
	if cmd.IdempotencyKey == "" {
		return nil, nil
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, cmd.AccountID.String()+":"+cmd.IdempotencyKey); err != nil {
		return nil, fmt.Errorf("lock message idempotency key: %w", err)
	}
	var existing types.Message
	err := tx.QueryRow(ctx, `
		SELECT id, account_id, conversation_id, direction, sender_type, sender_user_id,
		       content_type, content, external_message_id, idempotency_key, created_at
		FROM messages
		WHERE account_id = $1 AND idempotency_key = $2
	`, cmd.AccountID, cmd.IdempotencyKey).Scan(
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
	// The key is unique per account, not per conversation. Never hand back a
	// message that belongs to a conversation the caller did not address: the
	// visibility check only covered cmd.ConversationID.
	if existing.ConversationID != cmd.ConversationID {
		return nil, conflictf("idempotency key was already used for a different conversation")
	}
	return &existing, nil
}

func authorizeAIMessage(ctx context.Context, tx pgx.Tx, cmd SendMessageParams) error {
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
	`, cmd.ConversationID, cmd.AccountID).Scan(&state, &currentEpoch)
	if err != nil {
		return fmt.Errorf("lock conversation AI state: %w", err)
	}
	allowed := cmd.Purpose == types.MessagePurposeReply && state == types.AIStateActive
	if cmd.Purpose == types.MessagePurposeHumanReviewAck {
		allowed = state == types.AIStateCooldown || state == types.AIStateReviewRequired
	}
	if !allowed || currentEpoch != *cmd.GenerationEpoch {
		return conflictf("stale or unauthorized AI message")
	}
	return nil
}

func loadOutboundDestination(ctx context.Context, tx pgx.Tx, accountID, conversationID uuid.UUID) (outboundDestination, error) {
	var destination outboundDestination
	var capabilityJSON []byte
	var remoteAccountID string
	err := tx.QueryRow(ctx, `
		SELECT c.channel_id, COALESCE(ch.provider, ch.type),
		       COALESCE(c.external_thread_id, co.external_identity), ch.capabilities,
		       COALESCE(ch.remote_account_id, '')
		FROM conversations c
		JOIN channels ch ON c.channel_id = ch.id
		JOIN contacts co ON c.contact_id = co.id
		WHERE c.id = $1 AND c.account_id = $2
		FOR UPDATE OF c
	`, conversationID, accountID).Scan(&destination.channelID, &destination.provider, &destination.externalIdentity, &capabilityJSON, &remoteAccountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundDestination{}, notFoundf("conversation not found")
	}
	if err != nil {
		return outboundDestination{}, fmt.Errorf("lookup conversation details: %w", err)
	}
	if err := json.Unmarshal(capabilityJSON, &destination.capabilities); err != nil {
		return outboundDestination{}, fmt.Errorf("decode provider capabilities: %w", err)
	}
	destination.simulated = strings.HasPrefix(remoteAccountID, simulatorRemoteAccountPrefix)
	return destination, nil
}

// markSimulatedMessageSent completes a send on a simulator channel locally:
// there is no provider to deliver to, so the message is simply "sent".
func markSimulatedMessageSent(ctx context.Context, tx pgx.Tx, msg *types.Message) error {
	if _, err := tx.Exec(ctx, `
		UPDATE messages SET delivery_status = 'sent', provider_timestamp = created_at WHERE id = $1
	`, msg.ID); err != nil {
		return fmt.Errorf("mark simulated message sent: %w", err)
	}
	msg.DeliveryStatus = "sent"
	return nil
}

func lockReplyDraft(ctx context.Context, tx pgx.Tx, cmd SendMessageParams) error {
	if cmd.AIReplyDraftID == nil {
		return nil
	}
	var draftID uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM ai_reply_drafts
		WHERE id = $1 AND account_id = $2 AND conversation_id = $3 AND status = 'pending'
		FOR UPDATE
	`, *cmd.AIReplyDraftID, cmd.AccountID, cmd.ConversationID).Scan(&draftID)
	if errors.Is(err, pgx.ErrNoRows) {
		return notFoundf("reply draft not found")
	}
	if err != nil {
		return fmt.Errorf("lock AI reply draft: %w", err)
	}
	return nil
}

func resolveReplyTarget(ctx context.Context, tx pgx.Tx, cmd *SendMessageParams) error {
	if cmd.ReplyToMessageID == nil {
		return nil
	}
	err := tx.QueryRow(ctx, `
		SELECT provider_message_id FROM messages
		WHERE id = $1 AND account_id = $2 AND conversation_id = $3
		  AND provider_message_id IS NOT NULL AND deleted_at IS NULL
	`, *cmd.ReplyToMessageID, cmd.AccountID, cmd.ConversationID).Scan(&cmd.ReplyToProviderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return invalidf("reply target is unavailable")
	}
	if err != nil {
		return fmt.Errorf("resolve reply target: %w", err)
	}
	return nil
}

func insertOutboundMessage(ctx context.Context, tx pgx.Tx, cmd SendMessageParams, externalMessageID string) (*types.Message, error) {
	content, err := json.Marshal(map[string]any{"text": cmd.Text, "media_id": cmd.MediaID})
	if err != nil {
		return nil, fmt.Errorf("marshal outbound message: %w", err)
	}
	msg := &types.Message{
		AccountID: cmd.AccountID, ConversationID: cmd.ConversationID, Direction: "outbound",
		SenderType: cmd.Sender, SenderUserID: cmd.SenderUserID, ContentType: cmd.ContentType, Content: content,
		DeliveryStatus:   "queued",
		ReplyToMessageID: cmd.ReplyToMessageID,
	}
	if externalMessageID != "" {
		msg.ExternalMessageID = &externalMessageID
	}
	if cmd.IdempotencyKey != "" {
		msg.IdempotencyKey = &cmd.IdempotencyKey
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO messages (
			account_id, conversation_id, direction, sender_type, sender_user_id,
			content_type, content, external_message_id, reply_to_message_id, idempotency_key, delivery_status, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'queued', NOW())
		RETURNING id, created_at
	`, msg.AccountID, msg.ConversationID, msg.Direction, msg.SenderType, msg.SenderUserID, msg.ContentType, msg.Content, msg.ExternalMessageID, msg.ReplyToMessageID, msg.IdempotencyKey).Scan(&msg.ID, &msg.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert outbound message: %w", err)
	}
	return msg, nil
}

func insertOutboxCommand(
	ctx context.Context,
	tx pgx.Tx,
	destination outboundDestination,
	cmd SendMessageParams,
	message *types.Message,
) error {
	contentType := messaging.ContentType(cmd.ContentType)
	if !contentType.Valid() || contentType == messaging.ContentNotice {
		return invalidf("unsupported outbound content type %q", cmd.ContentType)
	}
	normalized := &messaging.Message{
		ExternalThreadID: destination.externalIdentity,
		Direction:        messaging.DirectionOutbound,
		Sender: messaging.Sender{
			ExternalID: "business",
		},
		ContentType:       contentType,
		Text:              cmd.Text,
		ProviderTimestamp: message.CreatedAt,
		ReplyToProviderID: cmd.ReplyToProviderID,
	}
	if cmd.MediaID != "" {
		mediaID, err := uuid.Parse(cmd.MediaID)
		if err != nil {
			return invalidf("invalid outbound media id")
		}
		var media messaging.Media
		err = tx.QueryRow(ctx, `
			SELECT id::TEXT, COALESCE(filename, ''), mime_type, size_bytes
			FROM media_objects
			WHERE id = $1 AND account_id = $2 AND channel_id = $3
			  AND storage_key IS NOT NULL AND expires_at > NOW()
		`, mediaID, cmd.AccountID, destination.channelID).Scan(
			&media.ID, &media.Filename, &media.MIMEType, &media.SizeBytes,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalidf("outbound media is unavailable")
		}
		if err != nil {
			return fmt.Errorf("load outbound media: %w", err)
		}
		normalized.Media = &media
	}
	command := messaging.Command{
		SchemaVersion: messaging.SchemaVersion,
		ID:            "send:" + message.ID.String(),
		Kind:          messaging.CommandSendMessage,
		Provider:      destination.provider,
		ChannelID:     destination.channelID.String(),
		CreatedAt:     message.CreatedAt,
		MessageID:     message.ID.String(),
		Message:       normalized,
	}
	if err := command.Validate(); err != nil {
		return fmt.Errorf("build outbound command: %w", err)
	}
	commandJSON, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("marshal outbound command: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO message_outbox (account_id, channel_id, message_id, provider, command)
		VALUES ($1, $2, $3, $4, $5)
	`, cmd.AccountID, destination.channelID, message.ID, destination.provider, commandJSON); err != nil {
		return fmt.Errorf("insert outbound command: %w", err)
	}
	return nil
}

func applyOutboundMessageEffects(ctx context.Context, tx pgx.Tx, cmd SendMessageParams, msg *types.Message) (*uuid.UUID, error) {
	if _, err := tx.Exec(ctx, `UPDATE conversations SET last_message_at = $1 WHERE id = $2 AND account_id = $3`, msg.CreatedAt, cmd.ConversationID, cmd.AccountID); err != nil {
		return nil, fmt.Errorf("update conversation details: %w", err)
	}
	if !cmd.human() {
		return nil, nil
	}
	if err := pauseAIAfterHumanMessage(ctx, tx, cmd.AccountID, cmd.ConversationID, cmd.SenderUserID, msg.ID, types.AIStateReasonHumanMessageSent); err != nil {
		return nil, err
	}
	return updateReplyDraftAfterHumanMessage(ctx, tx, cmd, msg.ID)
}

func updateReplyDraftAfterHumanMessage(ctx context.Context, tx pgx.Tx, cmd SendMessageParams, messageID uuid.UUID) (*uuid.UUID, error) {
	if cmd.AIReplyDraftID != nil {
		_, err := tx.Exec(ctx, `
			UPDATE ai_reply_drafts SET status = 'used', used_message_id = $1, updated_at = NOW()
			WHERE id = $2 AND account_id = $3 AND conversation_id = $4 AND status = 'pending'
		`, messageID, *cmd.AIReplyDraftID, cmd.AccountID, cmd.ConversationID)
		if err != nil {
			return nil, fmt.Errorf("update AI reply draft: %w", err)
		}
		return cmd.AIReplyDraftID, nil
	}

	var draftID uuid.UUID
	err := tx.QueryRow(ctx, `
		UPDATE ai_reply_drafts SET status = 'superseded', updated_at = NOW()
		WHERE account_id = $1 AND conversation_id = $2 AND status = 'pending'
		RETURNING id
	`, cmd.AccountID, cmd.ConversationID).Scan(&draftID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update AI reply draft: %w", err)
	}
	return &draftID, nil
}

func writeMessageSentAudit(ctx context.Context, tx pgx.Tx, cmd SendMessageParams, msg *types.Message) error {
	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID: cmd.AccountID, ActorUserID: cmd.SenderUserID,
		Action: "message.sent", TargetType: "message", TargetID: &msg.ID,
		Metadata: map[string]any{"conversation_id": cmd.ConversationID, "content_type": cmd.ContentType, "sender_type": cmd.Sender},
	}); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}
	return nil
}

func (s *ConversationService) publishOutboundMessageEvents(ctx context.Context, cmd SendMessageParams, msg *types.Message, draftID *uuid.UUID) {
	if _, err := s.pubsub.Publish(ctx, "conversation.updated", ConversationUpdatedEvent{AccountID: cmd.AccountID, ConversationID: cmd.ConversationID, MessageID: msg.ID}); err != nil {
		fmt.Printf("failed to publish conversation.updated for outbound send: %v\n", err)
	}
	if !cmd.human() || draftID == nil {
		return
	}
	action := "superseded"
	if cmd.AIReplyDraftID != nil {
		action = "used"
	}
	if _, err := s.pubsub.Publish(ctx, "ai.reply_draft.updated", AIReplyDraftUpdatedEvent{
		AccountID: cmd.AccountID, ConversationID: cmd.ConversationID, DraftID: draftID, Action: action,
	}); err != nil {
		fmt.Printf("failed to publish AI reply draft update: %v\n", err)
	}
}
