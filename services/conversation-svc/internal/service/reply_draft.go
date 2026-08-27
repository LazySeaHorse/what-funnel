package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// GetPendingReplyDraft returns the one active suggestion for a conversation.
// Conversation visibility is checked before draft data is queried.
func (s *Service) GetPendingReplyDraft(ctx context.Context, accountID, userID, conversationID uuid.UUID, role string) (*types.AIReplyDraft, error) {
	if err := s.canSeeConversation(ctx, accountID, userID, conversationID, role); err != nil {
		return nil, err
	}

	draft := &types.AIReplyDraft{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, conversation_id, source_message_id, draft_text, stage_matched,
		       confidence, status, created_at, updated_at
		FROM ai_reply_drafts
		WHERE account_id = $1 AND conversation_id = $2 AND status = 'pending'
		LIMIT 1
	`, accountID, conversationID).Scan(
		&draft.ID, &draft.ConversationID, &draft.SourceMessageID,
		&draft.DraftText, &draft.StageMatched, &draft.Confidence,
		&draft.Status, &draft.CreatedAt, &draft.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get pending AI reply draft: %w", err)
	}
	return draft, nil
}

// DismissReplyDraft marks a pending draft as dismissed and broadcasts the
// lifecycle change so all agents viewing the conversation stay in sync.
func (s *Service) DismissReplyDraft(ctx context.Context, accountID, userID, conversationID, draftID uuid.UUID, role string) error {
	if err := s.canSeeConversation(ctx, accountID, userID, conversationID, role); err != nil {
		return err
	}

	var updatedID uuid.UUID
	err := s.pool.QueryRow(ctx, `
		UPDATE ai_reply_drafts
		SET status = 'dismissed', updated_at = NOW()
		WHERE id = $1 AND account_id = $2 AND conversation_id = $3 AND status = 'pending'
		RETURNING id
	`, draftID, accountID, conversationID).Scan(&updatedID)
	if err == pgx.ErrNoRows {
		return errors.New("reply draft not found")
	}
	if err != nil {
		return fmt.Errorf("dismiss AI reply draft: %w", err)
	}

	if _, err := s.pubsub.Publish(ctx, "ai.reply_draft.updated", AIReplyDraftUpdatedEvent{
		AccountID: accountID, ConversationID: conversationID,
		DraftID: &updatedID, Action: "dismissed",
	}); err != nil {
		fmt.Printf("failed to publish AI reply draft dismissal: %v\n", err)
	}
	return nil
}
