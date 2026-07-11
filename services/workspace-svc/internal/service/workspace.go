// Package service implements workspace-svc business logic:
// user management (list, invite, role change), account settings
// (with encrypted ai_provider_config), and pipeline management.
// All state-changing operations write audit_logs rows.
package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/crypto"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// Service handles workspace operations.
type Service struct {
	pool   *pgxpool.Pool
	cipher *crypto.Cipher
}

// New creates a workspace Service.
// encryptionKey is a 32-byte raw string or 64-char hex key for AES-256-GCM.
func New(pool *pgxpool.Pool, encryptionKey string) (*Service, error) {
	cipher, err := crypto.NewCipherFromHex(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("workspace service: %w", err)
	}
	return &Service{pool: pool, cipher: cipher}, nil
}

// -------------------------------------------------------------------------
// Account
// -------------------------------------------------------------------------

// GetAccount returns the account for the given ID.
func (svc *Service) GetAccount(ctx context.Context, accountID uuid.UUID) (*types.Account, error) {
	a := &types.Account{}
	var settingsRaw []byte
	err := svc.pool.QueryRow(ctx,
		`SELECT id, name, plan, settings, created_at FROM accounts WHERE id = $1`,
		accountID).Scan(&a.ID, &a.Name, &a.Plan, &settingsRaw, &a.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("account not found")
		}
		return nil, fmt.Errorf("get account: %w", err)
	}
	a.Settings = settingsRaw
	return a, nil
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

	_, err = tx.Exec(ctx, `UPDATE accounts SET settings = $1 WHERE id = $2`, raw, accountID)
	if err != nil {
		return fmt.Errorf("update settings: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:  accountID,
		ActorUserID: &actorID,
		Action:     "account.settings_updated",
		TargetType: audit.TargetAccount,
		TargetID:   &accountID,
		Metadata:   map[string]any{},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// UpdateAIProviderConfig encrypts and stores the AI provider config.
// The plaintext is never stored; only the AES-256-GCM ciphertext reaches Postgres.
func (svc *Service) UpdateAIProviderConfig(ctx context.Context, accountID, actorID uuid.UUID, plaintext string) error {
	encrypted, err := svc.cipher.Encrypt([]byte(plaintext))
	if err != nil {
		return fmt.Errorf("encrypt ai_provider_config: %w", err)
	}

	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx, `UPDATE accounts SET ai_provider_config = $1 WHERE id = $2`, encrypted, accountID)
	if err != nil {
		return fmt.Errorf("store ai_provider_config: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:  accountID,
		ActorUserID: &actorID,
		Action:     "account.ai_provider_config_updated",
		TargetType: audit.TargetAccount,
		TargetID:   &accountID,
		Metadata:   map[string]any{"note": "encrypted value stored"},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetAIProviderConfig decrypts and returns the AI provider config plaintext.
// Returns empty string if not configured.
func (svc *Service) GetAIProviderConfig(ctx context.Context, accountID uuid.UUID) (string, error) {
	var encrypted *string
	err := svc.pool.QueryRow(ctx,
		`SELECT ai_provider_config FROM accounts WHERE id = $1`, accountID).
		Scan(&encrypted)
	if err != nil {
		return "", fmt.Errorf("get ai_provider_config: %w", err)
	}
	if encrypted == nil || *encrypted == "" {
		return "", nil
	}
	plain, err := svc.cipher.Decrypt(*encrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt ai_provider_config: %w", err)
	}
	return string(plain), nil
}

// -------------------------------------------------------------------------
// Users
// -------------------------------------------------------------------------

// ListUsers returns all users for the given account, ordered by created_at.
func (svc *Service) ListUsers(ctx context.Context, accountID uuid.UUID) ([]*types.User, error) {
	rows, err := svc.pool.Query(ctx,
		`SELECT id, account_id, email, role, created_at
		   FROM users WHERE account_id = $1 ORDER BY created_at ASC`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*types.User
	for rows.Next() {
		u := &types.User{}
		if err := rows.Scan(&u.ID, &u.AccountID, &u.Email, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// InviteUserRequest carries invite parameters.
type InviteUserRequest struct {
	Email string
	Role  string
}

// InviteResult is returned by InviteUser.
type InviteResult struct {
	Token string
	Email string
	Role  string
}

// InviteUser generates an invite token for the given email/role.
// Actual email delivery is stubbed — the token is returned in the API response.
// TODO: wire to email provider
func (svc *Service) InviteUser(ctx context.Context, accountID, actorID uuid.UUID, req InviteUserRequest) (*InviteResult, error) {
	if req.Role != types.RoleAdmin && req.Role != types.RoleMember {
		return nil, fmt.Errorf("invalid role: %q", req.Role)
	}

	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generate invite token: %w", err)
	}

	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx,
		`INSERT INTO invite_tokens (token, account_id, email, role) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (token) DO NOTHING`,
		token, accountID, req.Email, req.Role)
	if err != nil {
		return nil, fmt.Errorf("store invite token: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:  accountID,
		ActorUserID: &actorID,
		Action:     audit.ActionUserInvited,
		TargetType: audit.TargetUser,
		Metadata:   map[string]any{"email": req.Email, "role": req.Role},
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// TODO: wire to email provider — send token to req.Email
	return &InviteResult{Token: token, Email: req.Email, Role: req.Role}, nil
}

// ChangeUserRole updates the role of targetUserID within the given account.
// Only admin callers may invoke this (enforced at the HTTP layer via RequireAdmin).
func (svc *Service) ChangeUserRole(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newRole string) error {
	if newRole != types.RoleAdmin && newRole != types.RoleMember {
		return fmt.Errorf("invalid role: %q", newRole)
	}

	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Verify target user belongs to this account
	var count int
	err = tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE id = $1 AND account_id = $2`, targetUserID, accountID).
		Scan(&count)
	if err != nil || count == 0 {
		return fmt.Errorf("user not found in account")
	}

	_, err = tx.Exec(ctx,
		`UPDATE users SET role = $1 WHERE id = $2 AND account_id = $3`, newRole, targetUserID, accountID)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:  accountID,
		ActorUserID: &actorID,
		Action:     audit.ActionUserRoleChanged,
		TargetType: audit.TargetUser,
		TargetID:   &targetUserID,
		Metadata:   map[string]any{"new_role": newRole},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// -------------------------------------------------------------------------
// Lead Pipelines
// -------------------------------------------------------------------------

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

	// Verify pipeline belongs to this account
	var count int
	err = tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM lead_pipelines WHERE id = $1 AND account_id = $2`,
		pipelineID, accountID).Scan(&count)
	if err != nil || count == 0 {
		return fmt.Errorf("pipeline not found in account")
	}

	_, err = tx.Exec(ctx,
		`UPDATE lead_pipelines SET name = $1, states = $2 WHERE id = $3 AND account_id = $4`,
		req.Name, statesJSON, pipelineID, accountID)
	if err != nil {
		return fmt.Errorf("update pipeline: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:  accountID,
		ActorUserID: &actorID,
		Action:     audit.ActionPipelineUpdated,
		TargetType: audit.TargetPipeline,
		TargetID:   &pipelineID,
		Metadata:   map[string]any{"name": req.Name},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// -------------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------------

// generateToken creates a cryptographically random URL-safe token.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// pgxExecer adapts pgxpool.Pool to the audit.Writer's Exec interface.
type pgxExecer struct {
	pool *pgxpool.Pool
}

func (e *pgxExecer) Exec(ctx context.Context, sql string, args ...any) (interface{ RowsAffected() int64 }, error) {
	return e.pool.Exec(ctx, sql, args...)
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
