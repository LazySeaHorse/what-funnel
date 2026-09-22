package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// WorkspaceProvisioner decouples identity account creation from CRM workspace resources.
// It initializes workspace-domain entities (such as CRM pipelines) upon account signup.
type WorkspaceProvisioner interface {
	ProvisionWorkspace(ctx context.Context, tx pgx.Tx, accountID uuid.UUID, productMode string) error
}

// DefaultWorkspaceProvisioner seeds default CRM lead pipeline stages for the workspace.
type DefaultWorkspaceProvisioner struct{}

// NewDefaultWorkspaceProvisioner creates a DefaultWorkspaceProvisioner.
func NewDefaultWorkspaceProvisioner() *DefaultWorkspaceProvisioner {
	return &DefaultWorkspaceProvisioner{}
}

// ProvisionWorkspace creates default CRM pipeline for the account unless chatbot-only mode is selected.
func (p *DefaultWorkspaceProvisioner) ProvisionWorkspace(ctx context.Context, tx pgx.Tx, accountID uuid.UUID, productMode string) error {
	statesJSON, err := json.Marshal(types.DefaultPipelineStates)
	if err != nil {
		return fmt.Errorf("workspace provisioner: marshal default states: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO lead_pipelines (account_id, name, states) VALUES ($1, $2, $3)`,
		accountID, "Default Pipeline", statesJSON)
	if err != nil {
		return fmt.Errorf("workspace provisioner: seed default pipeline: %w", err)
	}
	return nil
}
