package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// leadStateChangedEvent is published to the lead.state_changed stream.
type leadStateChangedEvent struct {
	Type           string  `json:"type"`
	ConversationID string  `json:"conversation_id"`
	LeadID         string  `json:"lead_id"`
	FromState      *string `json:"from_state"`
	ToState        string  `json:"to_state"`
}

// getLeadAndCheckVisibility fetches a lead by ID (scoped to the account) and
// verifies the requesting user can see the associated conversation. Returns
// "lead not found" for both missing leads and invisible ones to avoid leaking
// existence.
func (s *Service) getLeadAndCheckVisibility(ctx context.Context, accountID, userID uuid.UUID, leadID uuid.UUID, userRole string) (*types.Lead, error) {
	var lead types.Lead
	var leadCreatedBy *uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT id, account_id, conversation_id, pipeline_id, current_state_key, tags, created_by, created_at, updated_at
		FROM leads WHERE id = $1 AND account_id = $2
	`, leadID, accountID).Scan(
		&lead.ID, &lead.AccountID, &lead.ConversationID, &lead.PipelineID,
		&lead.CurrentStateKey, &lead.Tags, &leadCreatedBy, &lead.CreatedAt, &lead.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("lead not found")
		}
		return nil, err
	}
	lead.CreatedBy = leadCreatedBy

	if err := s.canSeeConversation(ctx, accountID, userID, lead.ConversationID, userRole); err != nil {
		return nil, fmt.Errorf("lead not found")
	}
	return &lead, nil
}

// CreateLead creates a lead for the given conversation, placing it in the first
// state of the account's default pipeline. If a lead already exists for the
// conversation it is returned as-is (idempotent).
func (s *Service) CreateLead(ctx context.Context, accountID, userID, convoID uuid.UUID, userRole string) (*types.Lead, error) {
	if err := s.canSeeConversation(ctx, accountID, userID, convoID, userRole); err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Return existing lead if present (idempotent).
	var existing types.Lead
	var existingCreatedBy *uuid.UUID
	scanExisting := func(q interface{ QueryRow(context.Context, string, ...any) pgx.Row }) (*types.Lead, error) {
		err := q.QueryRow(ctx, `
			SELECT id, account_id, conversation_id, pipeline_id, current_state_key, tags, created_by, created_at, updated_at
			FROM leads WHERE conversation_id = $1 AND account_id = $2
		`, convoID, accountID).Scan(
			&existing.ID, &existing.AccountID, &existing.ConversationID, &existing.PipelineID,
			&existing.CurrentStateKey, &existing.Tags, &existingCreatedBy, &existing.CreatedAt, &existing.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		existing.CreatedBy = existingCreatedBy
		return &existing, nil
	}

	if lead, err := scanExisting(tx); err == nil {
		return lead, nil
	} else if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("check existing lead: %w", err)
	}

	// Fetch the first pipeline state.
	var pipelineID uuid.UUID
	var statesJSON []byte
	if err := tx.QueryRow(ctx,
		`SELECT id, states FROM lead_pipelines WHERE account_id = $1 ORDER BY created_at ASC LIMIT 1`,
		accountID).Scan(&pipelineID, &statesJSON); err != nil {
		return nil, fmt.Errorf("get lead pipeline: %w", err)
	}

	var states []types.PipelineState
	if err := json.Unmarshal(statesJSON, &states); err != nil {
		return nil, fmt.Errorf("unmarshal pipeline states: %w", err)
	}
	if len(states) == 0 {
		return nil, fmt.Errorf("pipeline has no states configured")
	}
	firstStateKey := states[0].Key

	// Insert lead.
	var leadID uuid.UUID
	var createdAt, updatedAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO leads (account_id, conversation_id, pipeline_id, current_state_key, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`, accountID, convoID, pipelineID, firstStateKey, userID).Scan(&leadID, &createdAt, &updatedAt)
	if err != nil {
		// Race: another request inserted the lead between our check and insert.
		if lead, err2 := scanExisting(tx); err2 == nil {
			return lead, nil
		}
		return nil, fmt.Errorf("create lead: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO lead_state_history (account_id, lead_id, from_state, to_state, changed_by)
		VALUES ($1, $2, NULL, $3, $4)
	`, accountID, leadID, firstStateKey, userID)
	if err != nil {
		return nil, fmt.Errorf("insert lead state history: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err = aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &userID,
		Action:      "lead.created",
		TargetType:  "lead",
		TargetID:    &leadID,
		Metadata: map[string]any{
			"conversation_id":   convoID,
			"current_state_key": firstStateKey,
			"auto":              false,
		},
	}); err != nil {
		return nil, fmt.Errorf("write audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create lead tx: %w", err)
	}

	return &types.Lead{
		ID:              leadID,
		AccountID:       accountID,
		ConversationID:  convoID,
		PipelineID:      pipelineID,
		CurrentStateKey: firstStateKey,
		Tags:            []string{},
		CreatedBy:       &userID,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}, nil
}

// UpdateLeadState transitions a lead to a new pipeline state.
func (s *Service) UpdateLeadState(ctx context.Context, accountID, userID uuid.UUID, leadID uuid.UUID, userRole string, targetStateKey string) (*types.Lead, error) {
	lead, err := s.getLeadAndCheckVisibility(ctx, accountID, userID, leadID, userRole)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Validate targetStateKey exists in the pipeline.
	var statesJSON []byte
	if err = tx.QueryRow(ctx,
		`SELECT states FROM lead_pipelines WHERE id = $1 AND account_id = $2`,
		lead.PipelineID, accountID,
	).Scan(&statesJSON); err != nil {
		return nil, fmt.Errorf("get pipeline: %w", err)
	}

	var states []types.PipelineState
	if err := json.Unmarshal(statesJSON, &states); err != nil {
		return nil, fmt.Errorf("unmarshal pipeline states: %w", err)
	}
	validState := false
	for _, st := range states {
		if st.Key == targetStateKey {
			validState = true
			break
		}
	}
	if !validState {
		return nil, fmt.Errorf("invalid state key: %q", targetStateKey)
	}

	fromState := lead.CurrentStateKey
	var updatedAt time.Time
	if err = tx.QueryRow(ctx, `
		UPDATE leads
		SET current_state_key = $1, updated_at = NOW()
		WHERE id = $2 AND account_id = $3
		RETURNING updated_at
	`, targetStateKey, leadID, accountID).Scan(&updatedAt); err != nil {
		return nil, fmt.Errorf("update lead state: %w", err)
	}
	lead.CurrentStateKey = targetStateKey
	lead.UpdatedAt = updatedAt

	_, err = tx.Exec(ctx, `
		INSERT INTO lead_state_history (account_id, lead_id, from_state, to_state, changed_by)
		VALUES ($1, $2, $3, $4, $5)
	`, accountID, leadID, fromState, targetStateKey, userID)
	if err != nil {
		return nil, fmt.Errorf("insert lead state history: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err = aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &userID,
		Action:      "lead.state_changed",
		TargetType:  "lead",
		TargetID:    &leadID,
		Metadata:    map[string]any{"from_state": fromState, "to_state": targetStateKey},
	}); err != nil {
		return nil, fmt.Errorf("write audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update lead state tx: %w", err)
	}

	if _, err = s.pubsub.Publish(ctx, "lead.state_changed", leadStateChangedEvent{
		Type:           "lead.state_changed",
		ConversationID: lead.ConversationID.String(),
		LeadID:         lead.ID.String(),
		FromState:      &fromState,
		ToState:        targetStateKey,
	}); err != nil {
		fmt.Printf("failed to publish lead.state_changed: %v\n", err)
	}

	return lead, nil
}

// UpdateLeadTags replaces the tag set on a lead.
func (s *Service) UpdateLeadTags(ctx context.Context, accountID, userID uuid.UUID, leadID uuid.UUID, userRole string, tags []string) (*types.Lead, error) {
	lead, err := s.getLeadAndCheckVisibility(ctx, accountID, userID, leadID, userRole)
	if err != nil {
		return nil, err
	}
	if tags == nil {
		tags = []string{}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var updatedAt time.Time
	if err = tx.QueryRow(ctx, `
		UPDATE leads
		SET tags = $1, updated_at = NOW()
		WHERE id = $2 AND account_id = $3
		RETURNING updated_at
	`, tags, leadID, accountID).Scan(&updatedAt); err != nil {
		return nil, fmt.Errorf("update lead tags: %w", err)
	}
	lead.Tags = tags
	lead.UpdatedAt = updatedAt

	aw := audit.NewWriterFromTx(tx)
	if err = aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &userID,
		Action:      "lead.tags_updated",
		TargetType:  "lead",
		TargetID:    &leadID,
		Metadata:    map[string]any{"tags": tags},
	}); err != nil {
		return nil, fmt.Errorf("write audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update lead tags tx: %w", err)
	}
	return lead, nil
}

// CreateLeadNote adds a note to a lead.
func (s *Service) CreateLeadNote(ctx context.Context, accountID, userID uuid.UUID, leadID uuid.UUID, userRole string, body string) (*types.LeadNote, error) {
	if _, err := s.getLeadAndCheckVisibility(ctx, accountID, userID, leadID, userRole); err != nil {
		return nil, err
	}
	if body == "" {
		return nil, fmt.Errorf("note body cannot be empty")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var noteID uuid.UUID
	var createdAt time.Time
	if err = tx.QueryRow(ctx, `
		INSERT INTO lead_notes (account_id, lead_id, author_user_id, body)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`, accountID, leadID, userID, body).Scan(&noteID, &createdAt); err != nil {
		return nil, fmt.Errorf("insert lead note: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err = aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &userID,
		Action:      "lead.note_added",
		TargetType:  "lead",
		TargetID:    &leadID,
		Metadata:    map[string]any{"note_id": noteID},
	}); err != nil {
		return nil, fmt.Errorf("write audit log: %w", err)
	}

	var authorEmail string
	if err = tx.QueryRow(ctx, `SELECT COALESCE(email, username, '') FROM users WHERE id = $1`, userID).Scan(&authorEmail); err != nil {
		return nil, fmt.Errorf("resolve author email: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create lead note tx: %w", err)
	}

	return &types.LeadNote{
		ID:           noteID,
		AccountID:    accountID,
		LeadID:       leadID,
		AuthorUserID: &userID,
		AuthorEmail:  authorEmail,
		Body:         body,
		CreatedAt:    createdAt,
	}, nil
}

// ListLeadNotes returns all notes for a lead, ordered chronologically.
func (s *Service) ListLeadNotes(ctx context.Context, accountID, userID uuid.UUID, leadID uuid.UUID, userRole string) ([]*types.LeadNote, error) {
	if _, err := s.getLeadAndCheckVisibility(ctx, accountID, userID, leadID, userRole); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT ln.id, ln.account_id, ln.lead_id, ln.author_user_id, ln.body, ln.created_at, COALESCE(u.email, u.username, '')
		FROM lead_notes ln
		LEFT JOIN users u ON ln.author_user_id = u.id
		WHERE ln.lead_id = $1 AND ln.account_id = $2
		ORDER BY ln.created_at ASC
	`, leadID, accountID)
	if err != nil {
		return nil, fmt.Errorf("query lead notes: %w", err)
	}
	defer rows.Close()

	notes := []*types.LeadNote{}
	for rows.Next() {
		var n types.LeadNote
		var authorUserID *uuid.UUID
		if err := rows.Scan(&n.ID, &n.AccountID, &n.LeadID, &authorUserID, &n.Body, &n.CreatedAt, &n.AuthorEmail); err != nil {
			return nil, err
		}
		n.AuthorUserID = authorUserID
		notes = append(notes, &n)
	}
	return notes, rows.Err()
}

// ListLeadHistory returns the state-change history for a lead.
func (s *Service) ListLeadHistory(ctx context.Context, accountID, userID uuid.UUID, leadID uuid.UUID, userRole string) ([]*types.LeadStateHistory, error) {
	if _, err := s.getLeadAndCheckVisibility(ctx, accountID, userID, leadID, userRole); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT lsh.id, lsh.account_id, lsh.lead_id, lsh.from_state, lsh.to_state, lsh.changed_by, lsh.changed_at, COALESCE(u.email, u.username, '')
		FROM lead_state_history lsh
		LEFT JOIN users u ON lsh.changed_by = u.id
		WHERE lsh.lead_id = $1 AND lsh.account_id = $2
		ORDER BY lsh.changed_at ASC
	`, leadID, accountID)
	if err != nil {
		return nil, fmt.Errorf("query lead history: %w", err)
	}
	defer rows.Close()

	history := []*types.LeadStateHistory{}
	for rows.Next() {
		var h types.LeadStateHistory
		var fromState *string
		var changedBy *uuid.UUID
		if err := rows.Scan(&h.ID, &h.AccountID, &h.LeadID, &fromState, &h.ToState, &changedBy, &h.ChangedAt, &h.ActorEmail); err != nil {
			return nil, err
		}
		h.FromState = fromState
		h.ChangedBy = changedBy
		history = append(history, &h)
	}
	return history, rows.Err()
}
