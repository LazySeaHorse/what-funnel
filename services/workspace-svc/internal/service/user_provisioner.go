package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"golang.org/x/crypto/bcrypt"
)

// IdentityProvisioner decouples workspace-svc from direct user credential hashing and identity storage.
// It delegates user provisioning, password resets, role changes, and identity deletion to the Identity domain.
type IdentityProvisioner interface {
	CreateUser(ctx context.Context, accountID, actorID uuid.UUID, req CreateUserRequest) (*CreateUserResult, error)
	ResetUserPassword(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newPassword string) error
	DeleteUser(ctx context.Context, accountID, actorID, targetUserID uuid.UUID) error
	ChangeUserRole(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newRole string) error
}

// NewIdentityProvisioner returns an HTTPIdentityProvisioner if identityURL is non-empty,
// or falls back to DirectIdentityProvisioner for tests and single-binary environments.
func NewIdentityProvisioner(pool *pgxpool.Pool, identityURL string) IdentityProvisioner {
	trimmed := strings.TrimRight(strings.TrimSpace(identityURL), "/")
	if trimmed != "" {
		return NewHTTPIdentityProvisioner(trimmed)
	}
	return NewDirectIdentityProvisioner(pool)
}

// DirectIdentityProvisioner executes user operations directly against the database.
// It acts as the local adapter for monolithic or testing environments.
type DirectIdentityProvisioner struct {
	pool *pgxpool.Pool
}

// NewDirectIdentityProvisioner creates a DirectIdentityProvisioner.
func NewDirectIdentityProvisioner(pool *pgxpool.Pool) *DirectIdentityProvisioner {
	return &DirectIdentityProvisioner{pool: pool}
}

func (d *DirectIdentityProvisioner) CreateUser(ctx context.Context, accountID, actorID uuid.UUID, req CreateUserRequest) (*CreateUserResult, error) {
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

	return &CreateUserResult{
		ID:                userID,
		Username:          req.Username,
		Role:              req.Role,
		PlaintextPassword: req.Password,
	}, nil
}

func (d *DirectIdentityProvisioner) ResetUserPassword(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newPassword string) error {
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

func (d *DirectIdentityProvisioner) DeleteUser(ctx context.Context, accountID, actorID, targetUserID uuid.UUID) error {
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

func (d *DirectIdentityProvisioner) ChangeUserRole(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newRole string) error {
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

// HTTPIdentityProvisioner calls identity-svc over HTTP to manage user credentials.
type HTTPIdentityProvisioner struct {
	baseURL string
	client  *http.Client
}

// NewHTTPIdentityProvisioner creates an HTTPIdentityProvisioner for the given identity-svc base URL.
func NewHTTPIdentityProvisioner(baseURL string) *HTTPIdentityProvisioner {
	return &HTTPIdentityProvisioner{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *HTTPIdentityProvisioner) CreateUser(ctx context.Context, accountID, actorID uuid.UUID, req CreateUserRequest) (*CreateUserResult, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal create user request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, h.baseURL+"/auth/users", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Account-ID", accountID.String())
	httpReq.Header.Set("X-Actor-ID", actorID.String())

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("identity-svc create user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return nil, fmt.Errorf("identity-svc: %s", errResp.Error)
		}
		return nil, fmt.Errorf("identity-svc returned status %d", resp.StatusCode)
	}

	var res CreateUserResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode create user response: %w", err)
	}
	return &res, nil
}

func (h *HTTPIdentityProvisioner) ResetUserPassword(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newPassword string) error {
	body, err := json.Marshal(map[string]string{"password": newPassword})
	if err != nil {
		return fmt.Errorf("marshal reset password request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("%s/auth/users/%s/password", h.baseURL, targetUserID), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Account-ID", accountID.String())
	httpReq.Header.Set("X-Actor-ID", actorID.String())

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("identity-svc reset password: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return fmt.Errorf("identity-svc: %s", errResp.Error)
		}
		return fmt.Errorf("identity-svc returned status %d", resp.StatusCode)
	}
	return nil
}

func (h *HTTPIdentityProvisioner) DeleteUser(ctx context.Context, accountID, actorID, targetUserID uuid.UUID) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("%s/auth/users/%s", h.baseURL, targetUserID), nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("X-Account-ID", accountID.String())
	httpReq.Header.Set("X-Actor-ID", actorID.String())

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("identity-svc delete user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return fmt.Errorf("identity-svc: %s", errResp.Error)
		}
		return fmt.Errorf("identity-svc returned status %d", resp.StatusCode)
	}
	return nil
}

func (h *HTTPIdentityProvisioner) ChangeUserRole(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newRole string) error {
	body, err := json.Marshal(map[string]string{"role": newRole})
	if err != nil {
		return fmt.Errorf("marshal change role request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("%s/auth/users/%s/role", h.baseURL, targetUserID), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Account-ID", accountID.String())
	httpReq.Header.Set("X-Actor-ID", actorID.String())

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("identity-svc change role: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return fmt.Errorf("identity-svc: %s", errResp.Error)
		}
		return fmt.Errorf("identity-svc returned status %d", resp.StatusCode)
	}
	return nil
}
