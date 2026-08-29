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
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/crypto"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/onboarding"
	"golang.org/x/crypto/bcrypt"
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
		`SELECT id, name, plan, product_mode, settings, created_at FROM accounts WHERE id = $1`,
		accountID).Scan(&a.ID, &a.Name, &a.Plan, &a.ProductMode, &settingsRaw, &a.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("account not found")
		}
		return nil, fmt.Errorf("get account: %w", err)
	}
	a.Settings = settingsRaw
	return a, nil
}

// DeleteAccount removes an account root. All tenant-owned data is removed by
// the database's ON DELETE CASCADE constraints.
func (svc *Service) DeleteAccount(ctx context.Context, accountID uuid.UUID) error {
	command, err := svc.pool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, accountID)
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("account not found")
	}
	return nil
}

// UpdateAccountName changes the workspace name and records the administrative action.
func (svc *Service) UpdateAccountName(ctx context.Context, accountID, actorID uuid.UUID, name string) error {
	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx, `UPDATE accounts SET name = $1 WHERE id = $2`, name, accountID)
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

	result, err := tx.Exec(ctx, `UPDATE accounts SET settings = settings || $1::jsonb WHERE id = $2`, patchRaw, accountID)
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

	var oldMode string
	var settingsBytes []byte
	err = tx.QueryRow(ctx, `SELECT product_mode, settings FROM accounts WHERE id = $1`, accountID).Scan(&oldMode, &settingsBytes)
	if err != nil {
		return fmt.Errorf("query account details: %w", err)
	}

	settings := parseSettings(settingsBytes)

	if newMode == "chatbot_only" {
		settings["lead_tracking_enabled"] = false
	} else if newMode == "full_workspace" {
		settings["lead_tracking_enabled"] = true
	}

	rawSettings, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	_, err = tx.Exec(ctx, `UPDATE accounts SET product_mode = $1, settings = $2 WHERE id = $3`, newMode, rawSettings, accountID)
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

// HasAIProviderConfig reports configuration presence without exposing or
// decrypting provider credentials.
func (svc *Service) HasAIProviderConfig(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var configured bool
	err := svc.pool.QueryRow(ctx,
		`SELECT ai_provider_config IS NOT NULL AND ai_provider_config <> '' FROM accounts WHERE id = $1`,
		accountID).Scan(&configured)
	if err != nil {
		return false, fmt.Errorf("get ai provider status: %w", err)
	}
	return configured, nil
}

// -------------------------------------------------------------------------
// Users
// -------------------------------------------------------------------------

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

// ResetUserPassword updates the password of targetUserID.
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

// ChangeUserRole updates the role of targetUserID within the given account.
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

// SetAccountSlug updates the unique slug identifier for the workspace.
func (svc *Service) SetAccountSlug(ctx context.Context, accountID, actorID uuid.UUID, slug string) error {
	slug = strings.TrimSpace(strings.ToLower(slug))
	if slug == "" {
		return fmt.Errorf("slug cannot be empty")
	}
	for _, ch := range slug {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-') {
			return fmt.Errorf("slug may only contain lowercase letters, numbers, and hyphens")
		}
	}

	_, err := svc.pool.Exec(ctx, `UPDATE accounts SET slug = $1 WHERE id = $2`, slug, accountID)
	if err != nil {
		return fmt.Errorf("update slug: %w", err)
	}
	return nil
}

// GetAccountSlug retrieves the workspace slug for an account.
func (svc *Service) GetAccountSlug(ctx context.Context, accountID uuid.UUID) (string, error) {
	var slug *string
	err := svc.pool.QueryRow(ctx, `SELECT slug FROM accounts WHERE id = $1`, accountID).Scan(&slug)
	if err != nil {
		return "", fmt.Errorf("get slug: %w", err)
	}
	if slug == nil {
		return "", nil
	}
	return *slug, nil
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

func (svc *Service) UpdateUserReplyMode(ctx context.Context, accountID, userID uuid.UUID, replyMode *string) error {
	// 1. Fetch account settings to check if override is allowed
	var settingsBytes []byte
	err := svc.pool.QueryRow(ctx, "SELECT settings FROM accounts WHERE id = $1", accountID).Scan(&settingsBytes)
	if err != nil {
		return fmt.Errorf("lookup account settings: %w", err)
	}

	// default is true when the key is absent
	if !boolSetting(parseSettings(settingsBytes), "allow_member_reply_mode_override", true) {
		return fmt.Errorf("member reply mode overrides are not allowed by the administrator")
	}

	// 2. Update user override in DB
	_, err = svc.pool.Exec(ctx,
		"UPDATE users SET reply_mode_override = $1 WHERE id = $2 AND account_id = $3",
		replyMode, userID, accountID)
	if err != nil {
		return fmt.Errorf("update user reply mode: %w", err)
	}

	return nil
}

func (svc *Service) Pool() *pgxpool.Pool {
	return svc.pool
}

// -------------------------------------------------------------------------
// Onboarding
// -------------------------------------------------------------------------

// OnboardingState represents the current onboarding progress stored in
// accounts.settings["onboarding"].
type OnboardingState struct {
	BusinessType   *string  `json:"business_type"`   // "salon"|"photography"|"tutoring"|"home_services"|"other"|null
	CompletedSteps []string `json:"completed_steps"` // e.g. ["signup","mode_selected",...]
	SkippedSteps   []string `json:"skipped_steps"`   // e.g. ["channel_connect"]
	CompletedAt    *string  `json:"completed_at"`    // ISO timestamp or null
}

// GetOnboardingStatus reads the onboarding sub-key from accounts.settings.
// Pre-existing accounts with no onboarding key return an empty state — never 500.
func (svc *Service) GetOnboardingStatus(ctx context.Context, accountID uuid.UUID) (*OnboardingState, error) {
	var settingsRaw []byte
	err := svc.pool.QueryRow(ctx,
		`SELECT settings FROM accounts WHERE id = $1`, accountID).Scan(&settingsRaw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("account not found")
		}
		return nil, fmt.Errorf("get onboarding status: %w", err)
	}

	state := &OnboardingState{
		CompletedSteps: []string{},
		SkippedSteps:   []string{},
	}

	if len(settingsRaw) == 0 {
		return state, nil
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(settingsRaw, &settings); err != nil {
		// Corrupt settings — return empty state rather than 500
		return state, nil
	}

	raw, ok := settings["onboarding"]
	if !ok || raw == nil {
		return state, nil
	}

	if err := json.Unmarshal(raw, state); err != nil {
		// Unrecognisable onboarding block — return empty state
		return state, nil
	}

	// Ensure slices are never nil in the JSON response
	if state.CompletedSteps == nil {
		state.CompletedSteps = []string{}
	}
	if state.SkippedSteps == nil {
		state.SkippedSteps = []string{}
	}

	return state, nil
}

// PatchOnboardingStatus atomically updates the onboarding sub-key in accounts.settings.
// action must be "complete" or "skip".
// If step == "done", completed_at is set to the current UTC timestamp.
func (svc *Service) PatchOnboardingStatus(ctx context.Context, accountID uuid.UUID, step, action string) error {
	if action != "complete" && action != "skip" {
		return fmt.Errorf("invalid action %q: must be \"complete\" or \"skip\"", action)
	}

	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Read current settings atomically inside the transaction
	var settingsRaw []byte
	err = tx.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1 FOR UPDATE`, accountID).Scan(&settingsRaw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("account not found")
		}
		return fmt.Errorf("read settings: %w", err)
	}

	// Parse outer settings
	settings := parseSettings(settingsRaw)
	state := parseOnboardingState(settings)

	switch action {
	case "complete":
		state.CompletedSteps = appendDedup(state.CompletedSteps, step)
		state.SkippedSteps = removeItem(state.SkippedSteps, step)
	case "skip":
		state.SkippedSteps = appendDedup(state.SkippedSteps, step)
		state.CompletedSteps = removeItem(state.CompletedSteps, step)
	}

	// step == "done" marks the entire onboarding flow as finished
	if step == "done" {
		ts := time.Now().UTC().Format(time.RFC3339)
		state.CompletedAt = &ts
	}

	// Merge updated onboarding back into settings
	settings["onboarding"] = state

	rawSettings, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	_, err = tx.Exec(ctx, `UPDATE accounts SET settings = $1 WHERE id = $2`, rawSettings, accountID)
	if err != nil {
		return fmt.Errorf("update onboarding status: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:  accountID,
		Action:     "onboarding.step_updated",
		TargetType: audit.TargetAccount,
		TargetID:   &accountID,
		Metadata:   map[string]any{"step": step, "action": action},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetOnboardingTemplates returns all business templates in stable sorted order.
// Pure in-memory; no database access.
func (svc *Service) GetOnboardingTemplates() []onboarding.BusinessTemplate {
	return onboarding.SortedTemplates
}

// ApplyOnboardingTemplate applies a business-type template to the account.
// It updates summary_schema in settings, sets onboarding.business_type,
// and — when productMode is "full_workspace" — updates the first pipeline's
// states and name to match the template.
func (svc *Service) ApplyOnboardingTemplate(ctx context.Context, accountID, actorID uuid.UUID, businessType, productMode string) error {
	tmpl, ok := onboarding.Templates[businessType]
	if !ok {
		return fmt.Errorf("unknown business type: %q", businessType)
	}

	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Read current settings
	var settingsRaw []byte
	err = tx.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1 FOR UPDATE`, accountID).Scan(&settingsRaw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("account not found")
		}
		return fmt.Errorf("read settings: %w", err)
	}

	settings := parseSettings(settingsRaw)

	// Merge summary_schema into settings
	settings["summary_schema"] = tmpl.SummarySchema

	// Update onboarding.business_type
	onboardingMap := parseSettings(marshalAny(settings["onboarding"]))
	onboardingMap["business_type"] = businessType
	settings["onboarding"] = onboardingMap

	rawSettings, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	_, err = tx.Exec(ctx, `UPDATE accounts SET settings = $1 WHERE id = $2`, rawSettings, accountID)
	if err != nil {
		return fmt.Errorf("update settings: %w", err)
	}

	// For full_workspace mode, update the first pipeline with template states
	if productMode == "full_workspace" {
		var pipelineID uuid.UUID
		err = tx.QueryRow(ctx,
			`SELECT id FROM lead_pipelines WHERE account_id = $1 ORDER BY created_at ASC LIMIT 1`,
			accountID).Scan(&pipelineID)
		if err != nil && err != pgx.ErrNoRows {
			return fmt.Errorf("query first pipeline: %w", err)
		}
		if err != pgx.ErrNoRows {
			statesJSON, err := json.Marshal(tmpl.PipelineStates)
			if err != nil {
				return fmt.Errorf("marshal pipeline states: %w", err)
			}
			_, err = tx.Exec(ctx,
				`UPDATE lead_pipelines SET name = $1, states = $2 WHERE id = $3 AND account_id = $4`,
				tmpl.Label, statesJSON, pipelineID, accountID)
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
				Metadata:    map[string]any{"name": tmpl.Label, "template": businessType},
			}); err != nil {
				return err
			}
		}
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &actorID,
		Action:      "onboarding.template_applied",
		TargetType:  audit.TargetAccount,
		TargetID:    &accountID,
		Metadata:    map[string]any{"business_type": businessType, "product_mode": productMode},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// -------------------------------------------------------------------------
// Onboarding helpers
// -------------------------------------------------------------------------

// appendDedup appends item to slice only if it isn't already present.
func appendDedup(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

// removeItem returns a new slice with all occurrences of item removed.
func removeItem(slice []string, item string) []string {
	out := slice[:0:0]
	for _, s := range slice {
		if s != item {
			out = append(out, s)
		}
	}
	if out == nil {
		out = []string{}
	}
	return out
}

// ---------------------------------------------------------------------------
// Settings helpers
// ---------------------------------------------------------------------------

// parseSettings safely unmarshals an accounts.settings JSONB blob into a
// map[string]any. If the bytes are empty or unparseable it returns an empty
// map so callers never need to nil-check or branch on length.
func parseSettings(raw []byte) map[string]any {
	out := make(map[string]any)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out) // silently ignore corrupt JSON
	}
	return out
}

func mapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// parseOnboardingState extracts the "onboarding" sub-key from a parsed
// settings map and returns it as an *OnboardingState with non-nil slices.
func parseOnboardingState(settings map[string]any) *OnboardingState {
	state := &OnboardingState{
		CompletedSteps: []string{},
		SkippedSteps:   []string{},
	}
	raw, ok := settings["onboarding"]
	if !ok || raw == nil {
		return state
	}
	if b, err := json.Marshal(raw); err == nil {
		_ = json.Unmarshal(b, state)
	}
	if state.CompletedSteps == nil {
		state.CompletedSteps = []string{}
	}
	if state.SkippedSteps == nil {
		state.SkippedSteps = []string{}
	}
	return state
}

// boolSetting reads a boolean key from a parsed settings map. If the key is
// absent or not a bool, defaultVal is returned.
func boolSetting(settings map[string]any, key string, defaultVal bool) bool {
	v, ok := settings[key]
	if !ok {
		return defaultVal
	}
	b, ok := v.(bool)
	if !ok {
		return defaultVal
	}
	return b
}

// marshalAny round-trips an any value through JSON to produce []byte suitable
// for parseSettings. Returns nil (which parseSettings handles gracefully) on error.
func marshalAny(v any) []byte {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
