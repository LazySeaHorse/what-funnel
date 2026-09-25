package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db/dbgen"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// ListPipelines returns all pipelines for the account.
func (svc *Service) ListPipelines(ctx context.Context, accountID uuid.UUID) ([]*types.LeadPipeline, error) {
	rows, err := dbgen.New(svc.pool).ListPipelinesByAccount(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list pipelines: %w", err)
	}

	var pipelines []*types.LeadPipeline
	for _, r := range rows {
		p := &types.LeadPipeline{
			ID:        r.ID,
			AccountID: r.AccountID,
			Name:      r.Name,
			CreatedAt: r.CreatedAt,
		}
		if len(r.States) > 0 {
			if err := json.Unmarshal(r.States, &p.States); err != nil {
				return nil, fmt.Errorf("unmarshal states: %w", err)
			}
		}
		pipelines = append(pipelines, p)
	}
	return pipelines, nil
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

	q := dbgen.New(tx)

	// Verify pipeline belongs to this account and get its current states
	currentStatesRaw, err := q.GetPipelineStates(ctx, dbgen.GetPipelineStatesParams{
		ID:        pipelineID,
		AccountID: accountID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
		leadIDs, err := q.FindLeadsInStateKeys(ctx, dbgen.FindLeadsInStateKeysParams{
			AccountID: accountID,
			StateKeys: removedKeys,
		})
		if err != nil {
			return fmt.Errorf("check active leads: %w", err)
		}
		if len(leadIDs) > 0 {
			return &ErrPipelineInUse{
				StateKeys: removedKeys,
				LeadIDs:   leadIDs,
			}
		}
	}

	err = q.UpdatePipeline(ctx, dbgen.UpdatePipelineParams{
		Name:      req.Name,
		States:    statesJSON,
		ID:        pipelineID,
		AccountID: accountID,
	})
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
