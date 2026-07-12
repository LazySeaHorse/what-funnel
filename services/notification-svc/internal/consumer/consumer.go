package consumer

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/notification-svc/internal/server"
)

type Consumer struct {
	pool   *pgxpool.Pool
	ps     *pubsub.Client
	hub    *server.Hub
	logger *slog.Logger
}

func NewConsumer(pool *pgxpool.Pool, ps *pubsub.Client, hub *server.Hub, logger *slog.Logger) *Consumer {
	return &Consumer{
		pool:   pool,
		ps:     ps,
		hub:    hub,
		logger: logger,
	}
}

func (c *Consumer) Start(ctx context.Context, consumerName string) {
	go func() {
		c.logger.Info("starting conversation.updated stream consumer")
		err := c.ps.Consume(ctx, "conversation.updated", "notification-svc", consumerName, c.handleConversationUpdated)
		if err != nil && ctx.Err() == nil {
			c.logger.Error("conversation.updated consumer failed", "error", err)
		}
	}()

	go func() {
		c.logger.Info("starting conversation.assigned stream consumer")
		err := c.ps.Consume(ctx, "conversation.assigned", "notification-svc", consumerName, c.handleConversationAssigned)
		if err != nil && ctx.Err() == nil {
			c.logger.Error("conversation.assigned consumer failed", "error", err)
		}
	}()

	go func() {
		c.logger.Info("starting channel.status_changed stream consumer")
		err := c.ps.Consume(ctx, "channel.status_changed", "notification-svc", consumerName, c.handleChannelStatusChanged)
		if err != nil && ctx.Err() == nil {
			c.logger.Error("channel.status_changed consumer failed", "error", err)
		}
	}()

	go func() {
		c.logger.Info("starting lead.state_changed stream consumer")
		err := c.ps.Consume(ctx, "lead.state_changed", "notification-svc", consumerName, c.handleLeadStateChanged)
		if err != nil && ctx.Err() == nil {
			c.logger.Error("lead.state_changed consumer failed", "error", err)
		}
	}()

	go func() {
		c.logger.Info("starting ai.reply_ready stream consumer")
		err := c.ps.Consume(ctx, "ai.reply_ready", "notification-svc", consumerName, c.handleAIReplyReady)
		if err != nil && ctx.Err() == nil {
			c.logger.Error("ai.reply_ready consumer failed", "error", err)
		}
	}()

	go func() {
		c.logger.Info("starting automation_suggestion.created stream consumer")
		err := c.ps.Consume(ctx, "automation_suggestion.created", "notification-svc", consumerName, c.handleAutomationSuggestionCreated)
		if err != nil && ctx.Err() == nil {
			c.logger.Error("automation_suggestion.created consumer failed", "error", err)
		}
	}()

	go func() {
		c.logger.Info("starting conversation.summary_updated stream consumer")
		err := c.ps.Consume(ctx, "conversation.summary_updated", "notification-svc", consumerName, c.handleConversationSummaryUpdated)
		if err != nil && ctx.Err() == nil {
			c.logger.Error("conversation.summary_updated consumer failed", "error", err)
		}
	}()
}

func (c *Consumer) HandleConversationUpdatedForTest(ctx context.Context, id string, payload []byte) error {
	return c.handleConversationUpdated(ctx, id, payload)
}

func (c *Consumer) HandleLeadStateChangedForTest(ctx context.Context, id string, payload []byte) error {
	return c.handleLeadStateChanged(ctx, id, payload)
}

func (c *Consumer) handleConversationUpdated(ctx context.Context, id string, payload []byte) error {
	var ev struct {
		AccountID      uuid.UUID `json:"account_id"`
		ConversationID uuid.UUID `json:"conversation_id"`
		MessageID      uuid.UUID `json:"message_id"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		c.logger.Error("failed to unmarshal conversation.updated event", "error", err)
		return nil
	}

	// 1. Fetch conversation details (specifically assigned_user_ids)
	var assignedUserIDs []uuid.UUID
	err := c.pool.QueryRow(ctx, `SELECT assigned_user_ids FROM conversations WHERE id = $1 AND account_id = $2`, ev.ConversationID, ev.AccountID).Scan(&assignedUserIDs)
	if err != nil {
		if err == pgx.ErrNoRows {
			c.logger.Warn("conversation not found for update event", "convo_id", ev.ConversationID)
			return nil
		}
		return err
	}

	// 2. Fetch account settings
	var settingsBytes []byte
	err = c.pool.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1`, ev.AccountID).Scan(&settingsBytes)
	if err != nil {
		return err
	}
	unassignedVisible := types.IsUnassignedVisible(settingsBytes)

	// 3. Fetch full message details
	msg := &types.Message{}
	err = c.pool.QueryRow(ctx, `
		SELECT id, account_id, conversation_id, direction, sender_type, sender_user_id, content_type, content, external_message_id, created_at
		FROM messages WHERE id = $1 AND account_id = $2
	`, ev.MessageID, ev.AccountID).Scan(
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
		if err == pgx.ErrNoRows {
			c.logger.Warn("message not found for update event", "msg_id", ev.MessageID)
			return nil
		}
		return err
	}

	// Determine payload type
	eventType := "message.received"
	if msg.Direction == "outbound" {
		eventType = "message.sent"
	}

	wsEvent := server.WSMessageEvent{
		Type:           eventType,
		ConversationID: ev.ConversationID.String(),
		Message:        msg,
	}

	// 4. Broadcast to users in the same account with visibility filtering
	c.hub.BroadcastToAccount(ev.AccountID, wsEvent, func(userID uuid.UUID, role string) bool {
		return types.CanSeeConversation(role, userID, assignedUserIDs, unassignedVisible)
	})

	return nil
}

func (c *Consumer) handleConversationAssigned(ctx context.Context, id string, payload []byte) error {
	var ev struct {
		AccountID       uuid.UUID   `json:"account_id"`
		ConversationID  uuid.UUID   `json:"conversation_id"`
		AssignedUserIDs []uuid.UUID `json:"assigned_user_ids"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		c.logger.Error("failed to unmarshal conversation.assigned event", "error", err)
		return nil
	}

	// Fetch account settings
	var settingsBytes []byte
	err := c.pool.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1`, ev.AccountID).Scan(&settingsBytes)
	if err != nil {
		return err
	}
	unassignedVisible := types.IsUnassignedVisible(settingsBytes)

	assignedStrings := make([]string, len(ev.AssignedUserIDs))
	for i, uid := range ev.AssignedUserIDs {
		assignedStrings[i] = uid.String()
	}

	wsEvent := server.WSConversationAssignedEvent{
		Type:            "conversation.assigned",
		ConversationID:  ev.ConversationID.String(),
		AssignedUserIDs: assignedStrings,
	}

	// Broadcast with visibility filtering (check if they can see after assignment change)
	c.hub.BroadcastToAccount(ev.AccountID, wsEvent, func(userID uuid.UUID, role string) bool {
		return types.CanSeeConversation(role, userID, ev.AssignedUserIDs, unassignedVisible)
	})

	return nil
}

func (c *Consumer) handleChannelStatusChanged(ctx context.Context, id string, payload []byte) error {
	var ev struct {
		AccountID uuid.UUID `json:"account_id"`
		ChannelID uuid.UUID `json:"channel_id"`
		Status    string    `json:"status"`
		Detail    string    `json:"detail"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		c.logger.Error("failed to unmarshal channel.status_changed event", "error", err)
		return nil
	}

	wsEvent := server.WSChannelStatusChangedEvent{
		Type:      "channel.status_changed",
		ChannelID: ev.ChannelID.String(),
		Status:    ev.Status,
		Detail:    ev.Detail,
	}

	// Broadcast to anyone in the same account
	c.hub.BroadcastToAccount(ev.AccountID, wsEvent, nil)

	return nil
}

func (c *Consumer) handleLeadStateChanged(ctx context.Context, id string, payload []byte) error {
	var ev struct {
		Type           string    `json:"type"`
		ConversationID uuid.UUID `json:"conversation_id"`
		LeadID         uuid.UUID `json:"lead_id"`
		FromState      *string   `json:"from_state"`
		ToState        string    `json:"to_state"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		c.logger.Error("failed to unmarshal lead.state_changed event", "error", err)
		return nil
	}

	// 1. Resolve account_id and assigned_user_ids from the conversation
	var accountID uuid.UUID
	var assignedUserIDs []uuid.UUID
	err := c.pool.QueryRow(ctx, `SELECT account_id, assigned_user_ids FROM conversations WHERE id = $1`, ev.ConversationID).Scan(&accountID, &assignedUserIDs)
	if err != nil {
		if err == pgx.ErrNoRows {
			c.logger.Warn("conversation not found for lead state change event", "convo_id", ev.ConversationID)
			return nil
		}
		return err
	}

	// 2. Fetch account settings
	var settingsBytes []byte
	err = c.pool.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1`, accountID).Scan(&settingsBytes)
	if err != nil {
		return err
	}
	unassignedVisible := types.IsUnassignedVisible(settingsBytes)

	// 3. WS Event Payload
	wsEvent := map[string]any{
		"type":            "lead.state_changed",
		"conversation_id": ev.ConversationID.String(),
		"lead_id":         ev.LeadID.String(),
		"from_state":      ev.FromState,
		"to_state":        ev.ToState,
	}

	// 4. Broadcast to users in the same account with visibility filtering
	c.hub.BroadcastToAccount(accountID, wsEvent, func(userID uuid.UUID, role string) bool {
		return types.CanSeeConversation(role, userID, assignedUserIDs, unassignedVisible)
	})

	return nil
}

func (c *Consumer) handleAIReplyReady(ctx context.Context, id string, payload []byte) error {
	var ev struct {
		AccountID      uuid.UUID `json:"account_id"`
		ConversationID uuid.UUID `json:"conversation_id"`
		Action         string    `json:"action"`
		DraftText      string    `json:"draft_text"`
		MessageID      uuid.UUID `json:"message_id"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		c.logger.Error("failed to unmarshal ai.reply_ready event", "error", err)
		return nil
	}

	var assignedUserIDs []uuid.UUID
	err := c.pool.QueryRow(ctx, `SELECT assigned_user_ids FROM conversations WHERE id = $1 AND account_id = $2`, ev.ConversationID, ev.AccountID).Scan(&assignedUserIDs)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	}

	var settingsBytes []byte
	err = c.pool.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1`, ev.AccountID).Scan(&settingsBytes)
	if err != nil {
		return err
	}
	unassignedVisible := types.IsUnassignedVisible(settingsBytes)

	wsEvent := map[string]any{
		"type":            "ai.reply_ready",
		"conversation_id": ev.ConversationID.String(),
		"action":          ev.Action,
		"draft_text":      ev.DraftText,
		"message_id":      ev.MessageID.String(),
	}

	c.hub.BroadcastToAccount(ev.AccountID, wsEvent, func(userID uuid.UUID, role string) bool {
		return types.CanSeeConversation(role, userID, assignedUserIDs, unassignedVisible)
	})

	return nil
}

func (c *Consumer) handleAutomationSuggestionCreated(ctx context.Context, id string, payload []byte) error {
	var ev struct {
		AccountID    uuid.UUID       `json:"account_id"`
		SuggestionID uuid.UUID       `json:"suggestion_id"`
		Type         string          `json:"type"`
		Payload      json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		c.logger.Error("failed to unmarshal automation_suggestion.created event", "error", err)
		return nil
	}

	wsEvent := map[string]any{
		"type":          "automation_suggestion.created",
		"suggestion_id": ev.SuggestionID.String(),
		"type_payload":  ev.Type,
		"payload":       ev.Payload,
	}

	c.hub.BroadcastToAccount(ev.AccountID, wsEvent, func(userID uuid.UUID, role string) bool {
		return role == types.RoleAdmin
	})

	return nil
}

func (c *Consumer) handleConversationSummaryUpdated(ctx context.Context, id string, payload []byte) error {
	var ev struct {
		AccountID      uuid.UUID       `json:"account_id"`
		ConversationID uuid.UUID       `json:"conversation_id"`
		SummaryFields  json.RawMessage `json:"summary_fields"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		c.logger.Error("failed to unmarshal conversation.summary_updated event", "error", err)
		return nil
	}

	var assignedUserIDs []uuid.UUID
	err := c.pool.QueryRow(ctx, `SELECT assigned_user_ids FROM conversations WHERE id = $1 AND account_id = $2`, ev.ConversationID, ev.AccountID).Scan(&assignedUserIDs)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	}

	var settingsBytes []byte
	err = c.pool.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1`, ev.AccountID).Scan(&settingsBytes)
	if err != nil {
		return err
	}
	unassignedVisible := types.IsUnassignedVisible(settingsBytes)

	wsEvent := map[string]any{
		"type":            "conversation.summary_updated",
		"conversation_id": ev.ConversationID.String(),
		"summary_fields":  ev.SummaryFields,
	}

	c.hub.BroadcastToAccount(ev.AccountID, wsEvent, func(userID uuid.UUID, role string) bool {
		return types.CanSeeConversation(role, userID, assignedUserIDs, unassignedVisible)
	})

	return nil
}
