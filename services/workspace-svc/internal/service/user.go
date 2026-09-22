package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
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

// CreateUser directly creates a new user by delegating credential lifecycle to IdentityProvisioner.
func (svc *Service) CreateUser(ctx context.Context, accountID, actorID uuid.UUID, req CreateUserRequest) (*CreateUserResult, error) {
	return svc.identity.CreateUser(ctx, accountID, actorID, req)
}

// DeleteUser removes a user from an account, unassigning conversations in workspace domain
// and delegating user/session deletion to IdentityProvisioner.
func (svc *Service) DeleteUser(ctx context.Context, accountID, actorID, targetUserID uuid.UUID) error {
	if actorID == targetUserID {
		return fmt.Errorf("cannot delete own account")
	}

	// 1. Workspace domain cleanup: unassign from conversations
	_, err := svc.pool.Exec(ctx,
		`UPDATE conversations SET assigned_user_ids = array_remove(assigned_user_ids, $1) WHERE account_id = $2 AND $1 = ANY(assigned_user_ids)`,
		targetUserID, accountID)
	if err != nil {
		return fmt.Errorf("unassign conversations: %w", err)
	}

	// 2. Identity domain lifecycle: delete user credentials and revoke sessions
	return svc.identity.DeleteUser(ctx, accountID, actorID, targetUserID)
}

// ResetUserPassword updates the password of targetUserID and revokes existing sessions via IdentityProvisioner.
func (svc *Service) ResetUserPassword(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newPassword string) error {
	return svc.identity.ResetUserPassword(ctx, accountID, actorID, targetUserID, newPassword)
}

// ChangeUserRole updates the role of targetUserID within the given account and revokes old sessions via IdentityProvisioner.
func (svc *Service) ChangeUserRole(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newRole string) error {
	return svc.identity.ChangeUserRole(ctx, accountID, actorID, targetUserID, newRole)
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
