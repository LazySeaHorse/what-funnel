package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"golang.org/x/crypto/bcrypt"
)

// ListUsers returns all users for the given account, ordered by created_at.
func (svc *Service) ListUsers(ctx context.Context, accountID uuid.UUID) ([]*types.User, error) {
	rows, err := svc.pool.Query(ctx,
		`SELECT id, account_id, COALESCE(email, ''), COALESCE(username, ''), role, created_at
		   FROM users WHERE account_id = $1 ORDER BY created_at ASC`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*types.User
	for rows.Next() {
		u := &types.User{}
		if err := rows.Scan(&u.ID, &u.AccountID, &u.Email, &u.Username, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// CreateUserRequest carries creation parameters.
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// CreateUserResult is returned by CreateUser.
type CreateUserResult struct {
	ID                uuid.UUID `json:"id"`
	Username          string    `json:"username"`
	Role              string    `json:"role"`
	PlaintextPassword string    `json:"password,omitempty"`
}

// CreateUser directly creates a new user under the account.
func (svc *Service) CreateUser(ctx context.Context, accountID, actorID uuid.UUID, req CreateUserRequest) (*CreateUserResult, error) {
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

	tx, err := svc.pool.Begin(ctx)
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

	return &CreateUserResult{
		ID:                userID,
		Username:          req.Username,
		Role:              req.Role,
		PlaintextPassword: req.Password,
	}, nil
}

// DeleteUser removes a user from an account and clears assignments in conversations.
func (svc *Service) DeleteUser(ctx context.Context, accountID, actorID, targetUserID uuid.UUID) error {
	if actorID == targetUserID {
		return fmt.Errorf("cannot delete own account")
	}

	tx, err := svc.pool.Begin(ctx)
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

	// Unassign from any conversations
	_, err = tx.Exec(ctx,
		`UPDATE conversations SET assigned_user_ids = array_remove(assigned_user_ids, $1) WHERE account_id = $2 AND $1 = ANY(assigned_user_ids)`,
		targetUserID, accountID)
	if err != nil {
		return fmt.Errorf("unassign conversations: %w", err)
	}

	// Delete user
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

// ResetUserPassword updates the password of targetUserID and revokes existing sessions.
func (svc *Service) ResetUserPassword(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newPassword string) error {
	if newPassword == "" {
		return fmt.Errorf("password cannot be empty")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	tx, err := svc.pool.Begin(ctx)
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

// ChangeUserRole updates the role of targetUserID within the given account and revokes old sessions.
// Only manager callers may invoke this.
func (svc *Service) ChangeUserRole(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newRole string) error {
	if newRole != types.RoleManager && newRole != types.RoleAgent {
		return fmt.Errorf("invalid role: %q", newRole)
	}

	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Verify target user belongs to this account.
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

	// Revoke all existing sessions for targetUserID so old role privileges do not persist
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
		Metadata:    map[string]any{"new_role": newRole},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// VerifyUserBelongsToAccount is a convenience helper used in tests.
func (svc *Service) VerifyUserBelongsToAccount(ctx context.Context, accountID, userID uuid.UUID) (bool, error) {
	var count int
	err := svc.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE id = $1 AND account_id = $2`, userID, accountID).Scan(&count)
	return count > 0, err
}

// GetUserByID retrieves a user by ID, scoped to an account.
func (svc *Service) GetUserByID(ctx context.Context, accountID, userID uuid.UUID) (*types.User, error) {
	u := &types.User{}
	err := svc.pool.QueryRow(ctx,
		`SELECT id, account_id, email, role, created_at
		   FROM users WHERE id = $1 AND account_id = $2`,
		userID, accountID).
		Scan(&u.ID, &u.AccountID, &u.Email, &u.Role, &u.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}

	// Verify timestamp is set
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}

	return u, nil
}
