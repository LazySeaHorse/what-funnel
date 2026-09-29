package service_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/service"
	"golang.org/x/crypto/bcrypt"
)

// directIdentityProvisioner executes user operations directly against the database.
// It is a test-only fake standing in for identity-svc.
type directIdentityProvisioner struct {
	pool *pgxpool.Pool
}

// newDirectIdentityProvisioner creates a directIdentityProvisioner.
func newDirectIdentityProvisioner(pool *pgxpool.Pool) *directIdentityProvisioner {
	return &directIdentityProvisioner{pool: pool}
}

func (d *directIdentityProvisioner) CreateUser(ctx context.Context, accountID, actorID uuid.UUID, req service.CreateUserRequest) (*service.CreateUserResult, error) {
	if req.Role != types.RoleManager && req.Role != types.RoleAgent {
		return nil, fmt.Errorf("invalid role: %q", req.Role)
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if len(req.Username) < 2 {
		return nil, fmt.Errorf("username must be at least 2 characters")
	}
	for _, ch := range req.Username {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return nil, fmt.Errorf("username may only contain alphanumeric characters, hyphens, and underscores")
		}
	}
	if req.Password == "" {
		return nil, fmt.Errorf("password is required")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var userID uuid.UUID
	err = tx.QueryRow(ctx,
		`INSERT INTO users (account_id, username, password_hash, role) VALUES ($1, $2, $3, $4) RETURNING id`,
		accountID, req.Username, string(hash), req.Role).Scan(&userID)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &actorID,
		Action:      audit.ActionUserCreatedByAdmin,
		TargetType:  audit.TargetUser,
		TargetID:    &userID,
		Metadata:    map[string]any{"username": req.Username, "role": req.Role},
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &service.CreateUserResult{
		ID:                userID,
		Username:          req.Username,
		Role:              req.Role,
		PlaintextPassword: req.Password,
	}, nil
}

func (d *directIdentityProvisioner) ResetUserPassword(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newPassword string) error {
	if newPassword == "" {
		return fmt.Errorf("password cannot be empty")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var exists bool
	err = tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND account_id = $2)`, targetUserID, accountID).
		Scan(&exists)
	if err != nil || !exists {
		return fmt.Errorf("user not found in account")
	}

	_, err = tx.Exec(ctx,
		`UPDATE users SET password_hash = $1 WHERE id = $2 AND account_id = $3`, string(hash), targetUserID, accountID)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	// Revoke all existing sessions for targetUserID
	_, err = tx.Exec(ctx, `DELETE FROM sessions WHERE convert_from(data, 'UTF8')::jsonb->>'user_id' = $1`, targetUserID.String())
	if err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &actorID,
		Action:      audit.ActionUserPasswordReset,
		TargetType:  audit.TargetUser,
		TargetID:    &targetUserID,
		Metadata:    map[string]any{},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (d *directIdentityProvisioner) DeleteUser(ctx context.Context, accountID, actorID, targetUserID uuid.UUID) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var exists bool
	err = tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND account_id = $2)`, targetUserID, accountID).
		Scan(&exists)
	if err != nil || !exists {
		return fmt.Errorf("user not found in account")
	}

	_, err = tx.Exec(ctx, `DELETE FROM users WHERE id = $1 AND account_id = $2`, targetUserID, accountID)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	// Revoke all existing sessions for targetUserID
	_, err = tx.Exec(ctx, `DELETE FROM sessions WHERE convert_from(data, 'UTF8')::jsonb->>'user_id' = $1`, targetUserID.String())
	if err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &actorID,
		Action:      audit.ActionUserDeleted,
		TargetType:  audit.TargetUser,
		TargetID:    &targetUserID,
		Metadata:    map[string]any{},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (d *directIdentityProvisioner) ChangeUserRole(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newRole string) error {
	if newRole != types.RoleManager && newRole != types.RoleAgent {
		return fmt.Errorf("invalid role: %q", newRole)
	}

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var exists bool
	err = tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND account_id = $2)`, targetUserID, accountID).
		Scan(&exists)
	if err != nil || !exists {
		return fmt.Errorf("user not found in account")
	}

	_, err = tx.Exec(ctx,
		`UPDATE users SET role = $1 WHERE id = $2 AND account_id = $3`, newRole, targetUserID, accountID)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}

	_, err = tx.Exec(ctx, `DELETE FROM sessions WHERE convert_from(data, 'UTF8')::jsonb->>'user_id' = $1`, targetUserID.String())
	if err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &actorID,
		Action:      audit.ActionUserRoleChanged,
		TargetType:  audit.TargetUser,
		TargetID:    &targetUserID,
		Metadata:    map[string]any{"role": newRole},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

