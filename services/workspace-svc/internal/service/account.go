package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db/dbgen"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// GetAccount returns the account for the given ID.
func (svc *Service) GetAccount(ctx context.Context, accountID uuid.UUID) (*types.Account, error) {
	row, err := dbgen.New(svc.pool).GetAccountByID(ctx, accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("account not found")
		}
		return nil, fmt.Errorf("get account: %w", err)
	}
	return &types.Account{
		ID:          row.ID,
		Name:        row.Name,
		Plan:        row.Plan,
		ProductMode: row.ProductMode,
		Settings:    row.Settings,
		CreatedAt:   row.CreatedAt,
	}, nil
}

// DeleteAccount removes an account root. All tenant-owned data is removed by
// the database's ON DELETE CASCADE constraints, and all account sessions are revoked.
func (svc *Service) DeleteAccount(ctx context.Context, accountID uuid.UUID) error {
	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := dbgen.New(tx)
	command, err := q.DeleteAccountByID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("account not found")
	}

	err = q.DeleteSessionsByAccountID(ctx, accountID.String())
	if err != nil {
		return fmt.Errorf("revoke account sessions: %w", err)
	}

	return tx.Commit(ctx)
}

// UpdateAccountName changes the workspace name and records the administrative action.
func (svc *Service) UpdateAccountName(ctx context.Context, accountID, actorID uuid.UUID, name string) error {
	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	err = dbgen.New(tx).UpdateAccountName(ctx, dbgen.UpdateAccountNameParams{
		Name: name,
		ID:   accountID,
	})
	if err != nil {
		return fmt.Errorf("update account name: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &actorID,
		Action:      "account.name_updated",
		TargetType:  audit.TargetAccount,
		TargetID:    &accountID,
		Metadata:    map[string]any{"name": name},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// UpdateAccountSettings updates the non-sensitive settings JSONB column.
func (svc *Service) UpdateAccountSettings(ctx context.Context, accountID, actorID uuid.UUID, settings map[string]any) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	err = dbgen.New(tx).UpdateAccountSettings(ctx, dbgen.UpdateAccountSettingsParams{
		Settings: raw,
		ID:       accountID,
	})
	if err != nil {
		return fmt.Errorf("update settings: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &actorID,
		Action:      "account.settings_updated",
		TargetType:  audit.TargetAccount,
		TargetID:    &accountID,
		Metadata:    map[string]any{},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// MergeAccountSettings atomically updates only the supplied top-level keys.
// It is used by focused flows such as onboarding so unrelated preferences and
// onboarding progress cannot be lost through a stale read-modify-write cycle.
func (svc *Service) MergeAccountSettings(ctx context.Context, accountID, actorID uuid.UUID, patch map[string]any) error {
	patchRaw, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal settings patch: %w", err)
	}

	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	result, err := dbgen.New(tx).MergeAccountSettings(ctx, dbgen.MergeAccountSettingsParams{
		Settings: patchRaw,
		ID:       accountID,
	})
	if err != nil {
		return fmt.Errorf("merge settings: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("account not found")
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &actorID,
		Action:      "account.settings_updated",
		TargetType:  audit.TargetAccount,
		TargetID:    &accountID,
		Metadata:    map[string]any{"updated_keys": mapKeys(patch)},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// UpdateProductMode switches the product mode of the account, toggles lead_tracking_enabled, and audit logs it.
func (svc *Service) UpdateProductMode(ctx context.Context, accountID, actorID uuid.UUID, newMode string) error {
	if newMode != "full_workspace" && newMode != "chatbot_only" {
		return fmt.Errorf("invalid product mode: %s", newMode)
	}

	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := dbgen.New(tx)
	oldMode, err := q.GetAccountProductModeForUpdate(ctx, accountID)
	if err != nil {
		return fmt.Errorf("query account details: %w", err)
	}

	leadTrackingJSON := "false"
	if newMode == "full_workspace" {
		leadTrackingJSON = "true"
	}

	err = q.UpdateAccountProductMode(ctx, dbgen.UpdateAccountProductModeParams{
		ProductMode:         newMode,
		LeadTrackingEnabled: json.RawMessage(leadTrackingJSON),
		ID:                  accountID,
	})
	if err != nil {
		return fmt.Errorf("update product mode: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &actorID,
		Action:      "account.product_mode_updated",
		TargetType:  audit.TargetAccount,
		TargetID:    &accountID,
		Metadata: map[string]any{
			"old_mode": oldMode,
			"new_mode": newMode,
		},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// SetAccountSlug updates the unique slug identifier for the workspace.
func (svc *Service) SetAccountSlug(ctx context.Context, accountID, actorID uuid.UUID, slug string) error {
	slug = strings.TrimSpace(strings.ToLower(slug))
	if slug == "" {
		return fmt.Errorf("slug cannot be empty")
	}
	if len(slug) < 2 {
		return fmt.Errorf("slug must be at least 2 characters")
	}
	for _, ch := range slug {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-') {
			return fmt.Errorf("slug may only contain lowercase letters, numbers, and hyphens")
		}
	}

	err := dbgen.New(svc.pool).SetAccountSlug(ctx, dbgen.SetAccountSlugParams{
		Slug: &slug,
		ID:   accountID,
	})
	if err != nil {
		return fmt.Errorf("update slug: %w", err)
	}
	return nil
}

// GetAccountSlug retrieves the workspace slug for an account.
func (svc *Service) GetAccountSlug(ctx context.Context, accountID uuid.UUID) (string, error) {
	slug, err := dbgen.New(svc.pool).GetAccountSlug(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("get slug: %w", err)
	}
	if slug == nil {
		return "", nil
	}
	return *slug, nil
}
