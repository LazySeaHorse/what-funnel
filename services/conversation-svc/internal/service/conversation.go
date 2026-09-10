package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// ---------------------------------------------------------------------------
// RBAC helper — lightweight visibility check
// ---------------------------------------------------------------------------

// canSeeConversation is a cheap 2-column query used as a guard before any
// operation that requires conversation visibility. It replaces the previous
// pattern of calling GetConversation (a 25-column lateral JOIN) just to
// throw away the result.
func (s *Service) canSeeConversation(ctx context.Context, accountID, userID uuid.UUID, convoID uuid.UUID, role string) error {
	var assignedUserIDs []uuid.UUID
	var settingsBytes []byte
	err := s.pool.QueryRow(ctx, `
		SELECT c.assigned_user_ids, a.settings
		FROM conversations c
		JOIN accounts a ON c.account_id = a.id
		WHERE c.id = $1 AND c.account_id = $2
	`, convoID, accountID).Scan(&assignedUserIDs, &settingsBytes)
	if err == pgx.ErrNoRows {
		return errors.New("conversation not found")
	}
	if err != nil {
		return fmt.Errorf("check conversation visibility: %w", err)
	}
	if !types.CanSeeConversation(role, userID, assignedUserIDs, types.IsUnassignedVisible(settingsBytes)) {
		return errors.New("conversation not found")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Shared scan helper — eliminates the duplicated 14-variable scan block that
// was copy-pasted between ListConversations and GetConversation.
// ---------------------------------------------------------------------------

// conversationScanDest holds the nullable temporaries required to scan one
// conversation row from the shared list/get SQL projection.
type conversationScanDest struct {
	item       *types.ConversationListItem
	lastReadAt *time.Time
	// message fields
	msgID          *uuid.UUID
	msgDirection   *string
	msgSenderType  *types.MessageSender
	msgSenderUID   *uuid.UUID
	msgContentType *string
	msgContent     []byte
	msgExternalID  *string
	msgCreatedAt   *time.Time
	// lead fields
	leadID         *uuid.UUID
	leadPipelineID *uuid.UUID
	leadStateKey   *string
	leadTags       []string
	leadCreatedBy  *uuid.UUID
	leadCreatedAt  *time.Time
	leadUpdatedAt  *time.Time
}

// scanConversationRow scans one row from the shared SQL projection and
// assembles LastMessagePreview and Lead if the optional JOIN columns are
// present. It mutates d.item in place and returns the first scan error.
func scanConversationRow(scanner interface {
	Scan(dest ...any) error
}, d *conversationScanDest) error {
	if err := scanner.Scan(
		&d.item.Conversation.ID,
		&d.item.Conversation.AccountID,
		&d.item.Conversation.ContactID,
		&d.item.Conversation.ChannelID,
		&d.item.Conversation.Status,
		&d.item.Conversation.AssignedUserIDs,
		&d.item.Conversation.LastMessageAt,
		&d.item.Conversation.AIControl.State,
		&d.item.Conversation.AIControl.StateReason,
		&d.item.Conversation.AIControl.ReplyOverride,
		&d.item.Conversation.AIControl.RunState,
		&d.item.Conversation.AIControl.NextReviewAt,
		&d.item.Conversation.CreatedAt,
		&d.item.ContactName,
		&d.item.ContactAvatarURL,
		&d.lastReadAt,
		&d.item.ChannelType,
		&d.msgID, &d.msgDirection, &d.msgSenderType, &d.msgSenderUID,
		&d.msgContentType, &d.msgContent, &d.msgExternalID, &d.msgCreatedAt,
		&d.leadID, &d.leadPipelineID, &d.leadStateKey, &d.leadTags,
		&d.leadCreatedBy, &d.leadCreatedAt, &d.leadUpdatedAt,
	); err != nil {
		return err
	}

	// Compute unread flag.
	if d.item.Conversation.LastMessageAt != nil {
		d.item.Unread = d.lastReadAt == nil || d.item.Conversation.LastMessageAt.After(*d.lastReadAt)
	}

	// Build last message preview.
	if d.msgID != nil {
		d.item.LastMessagePreview = &types.Message{
			ID:                *d.msgID,
			AccountID:         d.item.Conversation.AccountID,
			ConversationID:    d.item.Conversation.ID,
			Direction:         *d.msgDirection,
			SenderType:        *d.msgSenderType,
			SenderUserID:      d.msgSenderUID,
			ContentType:       *d.msgContentType,
			Content:           d.msgContent,
			ExternalMessageID: d.msgExternalID,
			CreatedAt:         *d.msgCreatedAt,
		}
	}

	// Build lead summary.
	if d.leadID != nil {
		d.item.Lead = &types.Lead{
			ID:              *d.leadID,
			AccountID:       d.item.Conversation.AccountID,
			ConversationID:  d.item.Conversation.ID,
			PipelineID:      *d.leadPipelineID,
			CurrentStateKey: *d.leadStateKey,
			Tags:            d.leadTags,
			CreatedBy:       d.leadCreatedBy,
			CreatedAt:       *d.leadCreatedAt,
			UpdatedAt:       *d.leadUpdatedAt,
		}
	}

	return nil
}

// sharedConversationSQL is the common SELECT / JOIN block used by both
// ListConversations and GetConversation. Callers append their own WHERE clause.
const sharedConversationSQL = `
	SELECT c.id, c.account_id, c.contact_id, c.channel_id, c.status, c.assigned_user_ids, c.last_message_at,
	       ais.state, ais.state_reason, ais.reply_override, ais.run_state, ais.next_review_at, c.created_at,
	       co.display_name, co.avatar_url,
	       cr.last_read_at,
	       ch.type as channel_type,
	       m.id, m.direction, m.sender_type, m.sender_user_id,
	       m.content_type, m.content, m.external_message_id, m.created_at,
	       l.id, l.pipeline_id, l.current_state_key, l.tags, l.created_by, l.created_at, l.updated_at
	FROM conversations c
	JOIN conversation_ai_state ais ON ais.conversation_id = c.id AND ais.account_id = c.account_id
	JOIN contacts co ON c.contact_id = co.id
	LEFT JOIN channels ch ON c.channel_id = ch.id
	LEFT JOIN conversation_reads cr ON c.id = cr.conversation_id AND cr.user_id = $1
	LEFT JOIN leads l ON c.id = l.conversation_id
	LEFT JOIN LATERAL (
	    SELECT id, direction, sender_type, sender_user_id, content_type, content, external_message_id, created_at
	    FROM messages
	    WHERE conversation_id = c.id
	    ORDER BY created_at DESC, id DESC
	    LIMIT 1
	) m ON TRUE
`

// ---------------------------------------------------------------------------
// Conversation queries
// ---------------------------------------------------------------------------

// ListConversations returns conversations visible to the given user, with
// optional filter (all/mine/unassigned) and lead state filtering.
func (s *Service) ListConversations(ctx context.Context, accountID, userID uuid.UUID, userRole string, filter string, leadState string) ([]*types.ConversationListItem, error) {
	var settingsBytes []byte
	if err := s.pool.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1`, accountID).Scan(&settingsBytes); err != nil {
		return nil, fmt.Errorf("get account settings: %w", err)
	}
	unassignedVisible := types.IsUnassignedVisible(settingsBytes)

	sqlQuery := sharedConversationSQL + `
		WHERE c.account_id = $2 AND (
		    $3 = 'admin' OR $3 = 'manager' OR
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
		d := &conversationScanDest{item: &types.ConversationListItem{}}
		if err := scanConversationRow(rows, d); err != nil {
			return nil, fmt.Errorf("scan conversation row: %w", err)
		}
		list = append(list, d.item)
	}
	return list, rows.Err()
}

// GetConversation fetches a single conversation, enforcing RBAC visibility.
func (s *Service) GetConversation(ctx context.Context, accountID, userID, conversationID uuid.UUID, userRole string) (*types.ConversationListItem, error) {
	var settingsBytes []byte
	if err := s.pool.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1`, accountID).Scan(&settingsBytes); err != nil {
		return nil, fmt.Errorf("get account settings: %w", err)
	}
	unassignedVisible := types.IsUnassignedVisible(settingsBytes)

	d := &conversationScanDest{item: &types.ConversationListItem{}}
	err := scanConversationRow(
		s.pool.QueryRow(ctx, sharedConversationSQL+`WHERE c.id = $2 AND c.account_id = $3`,
			userID, conversationID, accountID),
		d,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.New("conversation not found")
		}
		return nil, err
	}

	if !types.CanSeeConversation(userRole, userID, d.item.Conversation.AssignedUserIDs, unassignedVisible) {
		return nil, errors.New("conversation not found")
	}

	return d.item, nil
}

// GetConversationMessages returns paginated messages for a conversation.
// Visibility is checked via the lightweight canSeeConversation guard.
func (s *Service) GetConversationMessages(ctx context.Context, accountID, userID, conversationID uuid.UUID, userRole string, beforeCursor string, limit int) ([]*types.Message, string, error) {
	if err := s.canSeeConversation(ctx, accountID, userID, conversationID, userRole); err != nil {
		return nil, "", err
	}

	args := []any{conversationID, accountID}
	sqlQuery := `
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
		if err := rows.Scan(
			&msg.ID, &msg.AccountID, &msg.ConversationID, &msg.Direction,
			&msg.SenderType, &msg.SenderUserID, &msg.ContentType, &msg.Content,
			&msg.ExternalMessageID, &msg.CreatedAt,
		); err != nil {
			return nil, "", fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if len(messages) > 0 && len(messages) == limit {
		last := messages[len(messages)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	return messages, nextCursor, nil
}

// AssignConversation sets the assigned users on a conversation.
func (s *Service) AssignConversation(ctx context.Context, accountID, conversationID uuid.UUID, assignedUserIDs []uuid.UUID, actorUserID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		UPDATE conversations
		SET assigned_user_ids = $1
		WHERE id = $2 AND account_id = $3
	`, assignedUserIDs, conversationID, accountID)
	if err != nil {
		return fmt.Errorf("update assignment: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err = aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &actorUserID,
		Action:      "conversation.assigned",
		TargetType:  "conversation",
		TargetID:    &conversationID,
		Metadata:    map[string]any{"assigned_user_ids": assignedUserIDs},
	}); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

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

// ReadConversation upserts a read-receipt for the given user.
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

// CloseConversation marks a conversation as closed and resets AI control for
// the next customer contact,
// records an audit log, and publishes conversation.closed and conversation.updated events.
func (s *Service) CloseConversation(ctx context.Context, accountID, userID, conversationID uuid.UUID, role string) error {
	if err := s.canSeeConversation(ctx, accountID, userID, conversationID, role); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		UPDATE conversations
		SET status = 'closed'
		WHERE id = $1 AND account_id = $2
	`, conversationID, accountID)
	if err != nil {
		return fmt.Errorf("update conversation close: %w", err)
	}
	_, err = tx.Exec(ctx, `
		WITH previous AS (
			SELECT state FROM conversation_ai_state
			WHERE conversation_id = $1 AND account_id = $2
			FOR UPDATE
		), updated AS (
			UPDATE conversation_ai_state
			SET state = 'active', state_reason = 'conversation_closed', run_state = 'idle',
			    run_started_at = NULL,
			    generation_epoch = generation_epoch + 1, cooldown_level = 0,
			    next_review_at = NULL, unanswered_count = 0,
			    unanswered_window_started_at = NULL, blocked_at = NULL,
			    version = version + 1, updated_at = NOW()
			WHERE conversation_id = $1 AND account_id = $2
		)
		INSERT INTO conversation_ai_state_events (
			account_id, conversation_id, actor_user_id, from_state, to_state, reason
		)
		SELECT $2, $1, $3, state, 'active', 'conversation_closed' FROM previous
	`, conversationID, accountID, userID)
	if err != nil {
		return fmt.Errorf("reset conversation AI state: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err = aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &userID,
		Action:      "conversation.closed",
		TargetType:  "conversation",
		TargetID:    &conversationID,
		Metadata:    map[string]any{},
	}); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit close tx: %w", err)
	}

	// Publish conversation.closed event for AI debounced summary and workers
	event := map[string]any{
		"account_id":      accountID.String(),
		"conversation_id": conversationID.String(),
		"closed_at":       time.Now().Format(time.RFC3339),
	}
	if _, err := s.pubsub.Publish(ctx, "conversation.closed", event); err != nil {
		fmt.Printf("failed to publish conversation.closed: %v\n", err)
	}

	// Publish conversation.updated event for real-time WebSocket clients
	if _, err := s.pubsub.Publish(ctx, "conversation.updated", ConversationUpdatedEvent{
		AccountID:      accountID,
		ConversationID: conversationID,
	}); err != nil {
		fmt.Printf("failed to publish conversation.updated on close: %v\n", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Cursor helpers
// ---------------------------------------------------------------------------

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
