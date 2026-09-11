package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

type providerMessageTarget struct {
	channelID       uuid.UUID
	provider        messaging.Provider
	externalThread  string
	providerMessage string
	contentType     messaging.ContentType
	capabilities    messaging.Capabilities
}

func (s *Service) EditProviderMessage(ctx context.Context, accountID, userID, conversationID, messageID uuid.UUID, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("message text is required")
	}
	return s.enqueueProviderMessageCommand(ctx, accountID, userID, conversationID, messageID, messaging.CommandEditMessage, text, "", false)
}

func (s *Service) DeleteProviderMessage(ctx context.Context, accountID, userID, conversationID, messageID uuid.UUID) error {
	return s.enqueueProviderMessageCommand(ctx, accountID, userID, conversationID, messageID, messaging.CommandDeleteMessage, "", "", false)
}

func (s *Service) ChangeProviderReaction(ctx context.Context, accountID, userID, conversationID, messageID uuid.UUID, emoji string, removed bool) error {
	emoji = strings.TrimSpace(emoji)
	if !removed && emoji == "" {
		return errors.New("reaction emoji is required")
	}
	return s.enqueueProviderMessageCommand(ctx, accountID, userID, conversationID, messageID, messaging.CommandChangeReaction, "", emoji, removed)
}

func (s *Service) enqueueProviderMessageCommand(ctx context.Context, accountID, userID, conversationID, messageID uuid.UUID, kind messaging.CommandKind, text, emoji string, removed bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin provider message command: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	target, err := loadProviderMessageTarget(ctx, tx, accountID, conversationID, messageID)
	if err != nil {
		return err
	}
	if err := requireProviderCapability(target.capabilities, kind); err != nil {
		return err
	}
	now := time.Now().UTC()
	command := messaging.Command{
		SchemaVersion: messaging.SchemaVersion,
		ID:            fmt.Sprintf("%s:%s", kind, uuid.NewString()),
		Kind:          kind,
		Provider:      target.provider,
		ChannelID:     target.channelID.String(),
		CreatedAt:     now,
		MessageID:     messageID.String(),
	}
	switch kind {
	case messaging.CommandEditMessage:
		message := &messaging.Message{ProviderMessageID: target.providerMessage, ExternalThreadID: target.externalThread, Direction: messaging.DirectionOutbound, ContentType: target.contentType, Text: text, ProviderTimestamp: now}
		if target.contentType != messaging.ContentText {
			message.Media = &messaging.Media{ProviderRef: "existing"}
		}
		command.Message = message
	case messaging.CommandDeleteMessage:
		command.Message = &messaging.Message{ProviderMessageID: target.providerMessage, ExternalThreadID: target.externalThread, ProviderTimestamp: now}
	case messaging.CommandChangeReaction:
		command.Reaction = &messaging.Reaction{ProviderMessageID: target.providerMessage, SenderExternalID: target.externalThread, Emoji: emoji, Removed: removed, ProviderTimestamp: now}
	}
	if err := command.Validate(); err != nil {
		return fmt.Errorf("build provider message command: %w", err)
	}
	payload, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("marshal provider message command: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO message_outbox (account_id, channel_id, message_id, provider, command) VALUES ($1, $2, $3, $4, $5)`, accountID, target.channelID, messageID, target.provider, payload); err != nil {
		return fmt.Errorf("insert provider message command: %w", err)
	}
	if err := audit.NewWriterFromTx(tx).Write(ctx, audit.Entry{AccountID: accountID, ActorUserID: &userID, Action: "message." + strings.TrimPrefix(string(kind), "message."), TargetType: "message", TargetID: &messageID, Metadata: map[string]any{"conversation_id": conversationID, "provider": target.provider}}); err != nil {
		return fmt.Errorf("audit provider message command: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit provider message command: %w", err)
	}
	_ = s.DispatchOutboxOnce(ctx)
	return nil
}

func loadProviderMessageTarget(ctx context.Context, tx pgx.Tx, accountID, conversationID, messageID uuid.UUID) (providerMessageTarget, error) {
	var target providerMessageTarget
	var capabilityJSON []byte
	err := tx.QueryRow(ctx, `
		SELECT ch.id, COALESCE(ch.provider, ch.type), COALESCE(c.external_thread_id, co.external_identity),
		       m.provider_message_id, m.content_type, ch.capabilities
		FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		JOIN channels ch ON ch.id = c.channel_id
		JOIN contacts co ON co.id = c.contact_id
		WHERE m.id = $1 AND m.conversation_id = $2 AND m.account_id = $3
		  AND m.direction = 'outbound' AND m.provider_message_id IS NOT NULL
	`, messageID, conversationID, accountID).Scan(&target.channelID, &target.provider, &target.externalThread, &target.providerMessage, &target.contentType, &capabilityJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerMessageTarget{}, errors.New("provider message not found or is not mutable")
	}
	if err != nil {
		return providerMessageTarget{}, fmt.Errorf("load provider message: %w", err)
	}
	if err := json.Unmarshal(capabilityJSON, &target.capabilities); err != nil {
		return providerMessageTarget{}, fmt.Errorf("decode provider capabilities: %w", err)
	}
	return target, nil
}

func requireProviderCapability(capabilities messaging.Capabilities, kind messaging.CommandKind) error {
	supported := kind == messaging.CommandEditMessage && capabilities.Edits ||
		kind == messaging.CommandDeleteMessage && capabilities.Deletes ||
		kind == messaging.CommandChangeReaction && capabilities.Reactions
	if !supported {
		return errors.New("this messaging provider does not support that operation")
	}
	return nil
}
