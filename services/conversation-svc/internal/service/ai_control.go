package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

type aiControlCommand struct {
	action           types.AIControlAction
	replyOverride    types.AIReplyOverride
	hasAction        bool
	hasReplyOverride bool
}

type aiControlTransition struct {
	state        types.AIState
	reason       types.AIStateReason
	stateChanged bool
}

func parseAIControlCommand(action, replyOverride string) (aiControlCommand, error) {
	if action == "" && replyOverride == "" {
		return aiControlCommand{}, errors.New("an action or reply_override is required")
	}

	cmd := aiControlCommand{}
	if action != "" {
		cmd.action = types.AIControlAction(action)
		if !cmd.action.Valid() {
			return aiControlCommand{}, errors.New("invalid AI control action")
		}
		cmd.hasAction = true
	}
	if replyOverride != "" {
		cmd.replyOverride = types.AIReplyOverride(replyOverride)
		if !cmd.replyOverride.Valid() {
			return aiControlCommand{}, errors.New("invalid reply_override")
		}
		cmd.hasReplyOverride = true
	}
	return cmd, nil
}

func planAIControlTransition(current types.AIState, cmd aiControlCommand, canUnblockSpam bool) (aiControlTransition, error) {
	transition := aiControlTransition{state: current}
	if !cmd.hasAction {
		return transition, nil
	}
	if cmd.action == types.AIControlActionResume && current == types.AIStateBlockedSpam && !canUnblockSpam {
		return aiControlTransition{}, errors.New("manager role required to unblock suspected spam")
	}

	transition.stateChanged = true
	switch cmd.action {
	case types.AIControlActionPause:
		transition.state = types.AIStatePausedHuman
		transition.reason = types.AIStateReasonManualPause
	case types.AIControlActionResume:
		transition.state = types.AIStateActive
		transition.reason = types.AIStateReasonManualResume
	case types.AIControlActionBlock:
		transition.state = types.AIStateBlockedManual
		transition.reason = types.AIStateReasonManualBlock
	}
	return transition, nil
}

func lockConversationAIState(ctx context.Context, tx pgx.Tx, accountID, conversationID uuid.UUID) (types.AIState, error) {
	var state types.AIState
	err := tx.QueryRow(ctx, `
		SELECT state
		FROM conversation_ai_state
		WHERE conversation_id = $1 AND account_id = $2
		FOR UPDATE
	`, conversationID, accountID).Scan(&state)
	if err == pgx.ErrNoRows {
		return "", errors.New("conversation not found")
	}
	if err != nil {
		return "", fmt.Errorf("lock conversation AI state: %w", err)
	}
	return state, nil
}

func applyAIControlTransition(
	ctx context.Context,
	tx pgx.Tx,
	accountID, userID, conversationID uuid.UUID,
	previous types.AIState,
	cmd aiControlCommand,
	transition aiControlTransition,
) (*types.ConversationAIState, error) {
	reason := string(transition.reason)
	replyOverride := ""
	if cmd.hasReplyOverride {
		replyOverride = string(cmd.replyOverride)
	}

	var state types.ConversationAIState
	err := tx.QueryRow(ctx, `
		UPDATE conversation_ai_state
		SET state = $1,
		    state_reason = CASE WHEN $2 = '' THEN state_reason ELSE NULLIF($2, '') END,
		    reply_override = CASE WHEN $3 = '' THEN reply_override ELSE $3 END,
		    run_state = CASE WHEN $2 = '' THEN run_state ELSE 'idle' END,
		    run_started_at = CASE WHEN $2 = '' THEN run_started_at ELSE NULL END,
		    generation_epoch = generation_epoch + CASE WHEN $2 = '' THEN 0 ELSE 1 END,
		    cooldown_level = CASE WHEN $2 = 'manual_resume' THEN 0 ELSE cooldown_level END,
		    next_review_at = CASE WHEN $2 = 'manual_resume' THEN NULL ELSE next_review_at END,
		    unanswered_count = CASE WHEN $2 = 'manual_resume' THEN 0 ELSE unanswered_count END,
		    unanswered_window_started_at = CASE WHEN $2 = 'manual_resume' THEN NULL ELSE unanswered_window_started_at END,
		    blocked_at = CASE WHEN $1 IN ('blocked_spam', 'blocked_manual') THEN NOW() ELSE NULL END,
		    version = version + 1,
		    updated_at = NOW()
		WHERE conversation_id = $4 AND account_id = $5
		RETURNING state, state_reason, reply_override, run_state, next_review_at
	`, transition.state, reason, replyOverride, conversationID, accountID).Scan(
		&state.State, &state.StateReason, &state.ReplyOverride, &state.RunState, &state.NextReviewAt,
	)
	if err != nil {
		return nil, fmt.Errorf("update conversation AI state: %w", err)
	}

	if transition.stateChanged {
		_, err = tx.Exec(ctx, `
			INSERT INTO conversation_ai_state_events (
				account_id, conversation_id, actor_user_id, from_state, to_state, reason
			) VALUES ($1, $2, $3, $4, $5, $6)
		`, accountID, conversationID, userID, previous, transition.state, transition.reason)
		if err != nil {
			return nil, fmt.Errorf("record conversation AI state event: %w", err)
		}
	}
	return &state, nil
}

// UpdateConversationAIControl atomically changes AI ownership and/or the
// per-chat reply policy. A state change invalidates every in-flight generation
// by advancing generation_epoch; workers must check that epoch before sending.
func (s *Service) UpdateConversationAIControl(ctx context.Context, accountID, userID, conversationID uuid.UUID, role, action, replyOverride string) (*types.ConversationAIState, error) {
	if err := s.canSeeConversation(ctx, accountID, userID, conversationID, role); err != nil {
		return nil, err
	}
	cmd, err := parseAIControlCommand(action, replyOverride)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin AI control tx: %w", err)
	}
	defer tx.Rollback(ctx)

	previous, err := lockConversationAIState(ctx, tx, accountID, conversationID)
	if err != nil {
		return nil, err
	}
	canUnblockSpam := role == types.RoleAdmin || role == types.RoleManager
	transition, err := planAIControlTransition(previous, cmd, canUnblockSpam)
	if err != nil {
		return nil, err
	}
	state, err := applyAIControlTransition(ctx, tx, accountID, userID, conversationID, previous, cmd, transition)
	if err != nil {
		return nil, err
	}

	aw := audit.NewWriterFromTx(tx)
	if err = aw.Write(ctx, audit.Entry{
		AccountID: accountID, ActorUserID: &userID,
		Action: "conversation.ai_control_updated", TargetType: "conversation", TargetID: &conversationID,
		Metadata: map[string]any{"action": action, "reply_override": replyOverride, "state": transition.state},
	}); err != nil {
		return nil, fmt.Errorf("write audit log: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit AI control tx: %w", err)
	}

	if _, publishErr := s.pubsub.Publish(ctx, "ai.control.updated", map[string]any{
		"account_id": accountID, "conversation_id": conversationID,
		"state": state.State, "state_reason": state.StateReason, "run_state": state.RunState,
	}); publishErr != nil {
		fmt.Printf("failed to publish AI control update: %v\n", publishErr)
	}
	return state, nil
}
