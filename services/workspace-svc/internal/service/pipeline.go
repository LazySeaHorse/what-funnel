package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// ListPipelines returns all pipelines for the account.
func (svc *Service) ListPipelines(ctx context.Context, accountID uuid.UUID) ([]*types.LeadPipeline, error) {
	rows, err := svc.pool.Query(ctx,
		`SELECT id, account_id, name, states, created_at
		   FROM lead_pipelines WHERE account_id = $1 ORDER BY created_at ASC`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list pipelines: %w", err)
	}
	defer rows.Close()

	var pipelines []*types.LeadPipeline
	for rows.Next() {
		p := &types.LeadPipeline{}
		var statesRaw []byte
		if err := rows.Scan(&p.ID, &p.AccountID, &p.Name, &statesRaw, &p.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(statesRaw, &p.States); err != nil {
			return nil, fmt.Errorf("unmarshal states: %w", err)
		}
		pipelines = append(pipelines, p)
	}
	return pipelines, rows.Err()
}

// UpdatePipelineRequest carries fields for a pipeline update.
type UpdatePipelineRequest struct {
	Name   string
	States []types.PipelineState
}

// ErrPipelineInUse is returned when attempting to delete pipeline states that are still referenced by active leads.
type ErrPipelineInUse struct {
	StateKeys []string
	LeadIDs   []uuid.UUID
}

func (e *ErrPipelineInUse) Error() string {
	return fmt.Sprintf("%d leads are currently in state(s) %v, move them first", len(e.LeadIDs), e.StateKeys)
}

// UpdatePipeline updates a pipeline's name and states.
func (svc *Service) UpdatePipeline(ctx context.Context, accountID, actorID, pipelineID uuid.UUID, req UpdatePipelineRequest) error {
	statesJSON, err := json.Marshal(req.States)
	if err != nil {
		return fmt.Errorf("marshal states: %w", err)
	}

	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Verify pipeline belongs to this account and get its current states
	var currentStatesRaw []byte
	err = tx.QueryRow(ctx,
		`SELECT states FROM lead_pipelines WHERE id = $1 AND account_id = $2`,
		pipelineID, accountID).Scan(&currentStatesRaw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("pipeline not found in account")
		}
		return fmt.Errorf("query pipeline: %w", err)
	}

	var currentStates []types.PipelineState
	if len(currentStatesRaw) > 0 {
		if err := json.Unmarshal(currentStatesRaw, &currentStates); err != nil {
			return fmt.Errorf("unmarshal current states: %w", err)
		}
	}

	// Diff states to find removed keys
	newKeys := make(map[string]bool)
	for _, s := range req.States {
		newKeys[s.Key] = true
	}

	var removedKeys []string
	for _, oldState := range currentStates {
		if !newKeys[oldState.Key] {
			removedKeys = append(removedKeys, oldState.Key)
		}
	}

	if len(removedKeys) > 0 {
		rows, err := tx.Query(ctx, `SELECT id FROM leads WHERE account_id = $1 AND current_state_key = ANY($2)`, accountID, removedKeys)
		if err != nil {
			return fmt.Errorf("check active leads: %w", err)
		}
		defer rows.Close()
		var leadIDs []uuid.UUID
		for rows.Next() {
			var lid uuid.UUID
			if err := rows.Scan(&lid); err != nil {
				return err
			}
			leadIDs = append(leadIDs, lid)
		}
		if len(leadIDs) > 0 {
			return &ErrPipelineInUse{
				StateKeys: removedKeys,
				LeadIDs:   leadIDs,
			}
		}
	}

	_, err = tx.Exec(ctx,
		`UPDATE lead_pipelines SET name = $1, states = $2 WHERE id = $3 AND account_id = $4`,
		req.Name, statesJSON, pipelineID, accountID)
	if err != nil {
		return fmt.Errorf("update pipeline: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &actorID,
		Action:      audit.ActionPipelineUpdated,
		TargetType:  audit.TargetPipeline,
		TargetID:    &pipelineID,
		Metadata:    map[string]any{"name": req.Name},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
