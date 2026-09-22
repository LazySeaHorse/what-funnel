package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// InitialPipeline holds the ID and the default first state of a lead pipeline.
type InitialPipeline struct {
	ID            uuid.UUID
	FirstStateKey string
}

// PipelineResolver abstracts fetching initial pipeline configuration for an account,
// preventing direct raw SQL coupling between conversation-svc and workspace-svc's lead_pipelines schema.
type PipelineResolver interface {
	ResolveInitialPipeline(ctx context.Context, tx pgx.Tx, accountID uuid.UUID) (*InitialPipeline, error)
}

// DBLeadPipelineResolver provides default pipeline lookup.
type DBLeadPipelineResolver struct{}

// ResolveInitialPipeline resolves the first configured pipeline for an account.
func (r *DBLeadPipelineResolver) ResolveInitialPipeline(ctx context.Context, tx pgx.Tx, accountID uuid.UUID) (*InitialPipeline, error) {
	var pipelineID uuid.UUID
	var statesJSON []byte
	err := tx.QueryRow(ctx,
		`SELECT id, states FROM lead_pipelines WHERE account_id = $1 ORDER BY created_at ASC LIMIT 1`,
		accountID).Scan(&pipelineID, &statesJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve initial lead pipeline: %w", err)
	}

	var states []types.PipelineState
	if err := json.Unmarshal(statesJSON, &states); err != nil {
		return nil, fmt.Errorf("unmarshal pipeline states: %w", err)
	}
	if len(states) == 0 {
		return nil, errors.New("pipeline has no states configured")
	}

	return &InitialPipeline{
		ID:            pipelineID,
		FirstStateKey: states[0].Key,
	}, nil
}
