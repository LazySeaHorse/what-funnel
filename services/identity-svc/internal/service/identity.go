// Package service implements the business logic for account/user lifecycle:
// signup, login, logout, and user lookup. authboss handles password hashing
// (bcrypt); we handle account creation and tenant isolation setup.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	ab "github.com/aarondl/authboss/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db/dbgen"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/identity-svc/internal/session"
	"github.com/whatfunnel/whatfunnel/services/identity-svc/internal/store"
)

// Option configures an identity Service.
type Option func(*Service)

// WithWorkspaceProvisioner injects a custom WorkspaceProvisioner into Service.
func WithWorkspaceProvisioner(wp WorkspaceProvisioner) Option {
	return func(s *Service) {
		s.workspaceProvisioner = wp
	}
}

// Service handles auth lifecycle: signup, login, logout, and user provisioning.
type Service struct {
	pool                 *pgxpool.Pool
	sessions             *session.Store
	ab                   *ab.Authboss
	workspaceProvisioner WorkspaceProvisioner
}

// SignupRequest carries the fields needed to create an account + manager user.
type SignupRequest struct {
	AccountName string `json:"account_name"`
	Email       string `json:"email"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	ProductMode string `json:"product_mode"`
}

// LoginRequest carries credentials for login. Identifier can be email or slug-username.
type LoginRequest struct {
	Identifier string `json:"identifier"`
	Email      string `json:"email"`
	Password   string `json:"password"`
}

// New creates a Service wired to the given pool and session store.
func New(pool *pgxpool.Pool, sessions *session.Store, opts ...Option) (*Service, error) {
	cfg := ab.New()
	cfg.Config.Paths.RootURL = "http://localhost:8081"
	// We use authboss only for its password-hashing primitives in this
	// integration. The auth modules (auth, register, etc.) are bypassed
	// in favour of our own HTTP handlers, which call authboss's bcrypt helpers.
	if err := cfg.Init(); err != nil {
		return nil, fmt.Errorf("service: authboss init: %w", err)
	}

	svc := &Service{
		pool:                 pool,
		sessions:             sessions,
		ab:                   cfg,
		workspaceProvisioner: NewDefaultWorkspaceProvisioner(),
	}
	for _, opt := range opts {
		opt(svc)
	}

	return svc, nil
}

// Signup creates an account, manager user, and default pipeline in one atomic
// transaction.
func (svc *Service) Signup(ctx context.Context, req SignupRequest) (*types.User, error) {
	// Hash password via authboss (bcrypt)
	hash, err := svc.ab.Config.Core.Hasher.GenerateHash(req.Password)
	if err != nil {
		return nil, fmt.Errorf("service: hash password: %w", err)
	}

	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("service: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := dbgen.New(tx)
	aw := audit.NewWriterFromTx(tx)

	var accountID uuid.UUID
	var userID uuid.UUID
	userRole := types.RoleManager

	if req.ProductMode == "" {
		req.ProductMode = "full_workspace"
	}
	if req.ProductMode != "full_workspace" && req.ProductMode != "chatbot_only" {
		return nil, fmt.Errorf("invalid product mode: %s", req.ProductMode)
	}

	// 1. Create account with default settings
	defaultSettings, err := json.Marshal(map[string]any{
		"ai_enabled":                             true,
		"ai_reply_mode_default":                  "draft_only",
		"allow_member_reply_mode_override":       true,
		"ai_may_auto_answer_mixed_conversations": false,
		"lead_tracking_enabled":                  req.ProductMode == "full_workspace",
		"summary_schema": []map[string]string{
			{"key": "customer_wants", "label": "Customer Wants", "description": "What the customer is looking for"},
			{"key": "preferred_timeframe", "label": "Preferred Timeframe", "description": "When the customer wants it"},
			{"key": "objections", "label": "Objections", "description": "Customer doubts or objections"},
			{"key": "next_action", "label": "Next Action", "description": "What needs to be done next"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("service: marshal default settings: %w", err)
	}

	accountID, err = qtx.CreateAccount(ctx, dbgen.CreateAccountParams{
		Name:        req.AccountName,
		Plan:        types.PlanSelfHosted,
		Settings:    defaultSettings,
		ProductMode: req.ProductMode,
	})
	if err != nil {
		return nil, fmt.Errorf("service: create account: %w", err)
	}

	// 2. Check global email uniqueness across all accounts if email provided
	if req.Email != "" {
		count, _ := qtx.CountUsersByEmail(ctx, req.Email)
		if count > 0 {
			return nil, fmt.Errorf("service: email already registered")
		}
	}

	// 3. Create manager user
	createdUser, err := qtx.CreateUser(ctx, dbgen.CreateUserParams{
		AccountID:    accountID,
		Email:        req.Email,
		Username:     req.Username,
		PasswordHash: hash,
		Role:         userRole,
	})
	if err != nil {
		return nil, fmt.Errorf("service: create user: %w", err)
	}
	userID = createdUser.ID

	// 4. Provision workspace / CRM domain entities via WorkspaceProvisioner
	if svc.workspaceProvisioner != nil {
		if err := svc.workspaceProvisioner.ProvisionWorkspace(ctx, tx, accountID, req.ProductMode); err != nil {
			return nil, fmt.Errorf("service: provision workspace: %w", err)
		}
	}

	// 5. Write account audit log
	if err := aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &userID,
		Action:      audit.ActionAccountCreated,
		TargetType:  audit.TargetAccount,
		TargetID:    &accountID,
		Metadata:    map[string]any{"account_name": req.AccountName},
	}); err != nil {
		return nil, fmt.Errorf("service: audit account: %w", err)
	}

	// User Creation Audit Log
	userMeta := map[string]any{"role": userRole}
	if req.Email != "" {
		userMeta["email"] = req.Email
	}
	if req.Username != "" {
		userMeta["username"] = req.Username
	}

	if err := aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &userID,
		Action:      audit.ActionUserCreated,
		TargetType:  audit.TargetUser,
		TargetID:    &userID,
		Metadata:    userMeta,
	}); err != nil {
		return nil, fmt.Errorf("service: audit user: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("service: commit: %w", err)
	}

	return &types.User{
		ID:        userID,
		AccountID: accountID,
		Email:     req.Email,
		Username:  req.Username,
		Role:      userRole,
		CreatedAt: time.Now(),
	}, nil
}

// Login verifies credentials and returns the user if valid.
func (svc *Service) Login(ctx context.Context, req LoginRequest) (*store.User, error) {
	ident := req.Identifier
	if ident == "" {
		ident = req.Email
	}
	if ident == "" {
		return nil, fmt.Errorf("service: invalid credentials")
	}

	userStore := store.New(svc.pool)
	u, err := userStore.LoadByIdentifier(ctx, ident)
	if err != nil {
		return nil, fmt.Errorf("service: invalid credentials")
	}

	// Verify password via authboss (bcrypt)
	if err := svc.ab.Config.Core.Hasher.CompareHashAndPassword(u.PasswordHash, req.Password); err != nil {
		return nil, fmt.Errorf("service: invalid credentials")
	}

	// Write audit log (best-effort; don't fail the login on log failure)
	_ = svc.writeLoginAudit(ctx, u.AccountID, u.ID)

	return u, nil
}

func (svc *Service) writeLoginAudit(ctx context.Context, accountID, userID uuid.UUID) error {
	aw := audit.NewWriter(&pgxExecer{pool: svc.pool})
	return aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &userID,
		Action:      audit.ActionLogin,
		TargetType:  audit.TargetUser,
		TargetID:    &userID,
		Metadata:    map[string]any{},
	})
}

// SetSession creates a session for the given user.
func (svc *Service) SetSession(w http.ResponseWriter, r *http.Request, u *store.User) error {
	return svc.sessions.SetSession(w, r, u.ID, u.AccountID, u.Role, u.Username)
}

// Logout destroys the user's session and writes an audit log.
func (svc *Service) Logout(w http.ResponseWriter, r *http.Request) error {
	// Read session data before destroying so we can audit
	data, _ := svc.sessions.GetSession(r)

	if err := svc.sessions.DestroySession(w, r); err != nil {
		return fmt.Errorf("service: destroy session: %w", err)
	}

	// Write logout audit (best-effort)
	if data != nil {
		accountID, _ := uuid.Parse(data["account_id"])
		userID, _ := uuid.Parse(data["user_id"])
		if accountID != uuid.Nil && userID != uuid.Nil {
			aw := audit.NewWriter(&pgxExecer{pool: svc.pool})
			_ = aw.Write(r.Context(), audit.Entry{
				AccountID:   accountID,
				ActorUserID: &userID,
				Action:      audit.ActionLogout,
				TargetType:  audit.TargetUser,
				TargetID:    &userID,
				Metadata:    map[string]any{},
			})
		}
	}
	return nil
}

// CreateUserRequest carries fields needed to create a user.
type CreateUserRequest struct {
	Email    string `json:"email,omitempty"`
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// CreateUserResult is returned by CreateUser.
type CreateUserResult struct {
	ID                uuid.UUID `json:"id"`
	Email             string    `json:"email,omitempty"`
	Username          string    `json:"username"`
	Role              string    `json:"role"`
	PlaintextPassword string    `json:"password,omitempty"`
}

// CreateUser creates a user under an account, enforcing username rules and hashing password via Authboss.
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

	hash, err := svc.ab.Config.Core.Hasher.GenerateHash(req.Password)
	if err != nil {
		return nil, fmt.Errorf("service: hash password: %w", err)
	}

	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := dbgen.New(tx)

	if req.Email != "" {
		count, _ := qtx.CountUsersByEmail(ctx, req.Email)
		if count > 0 {
			return nil, fmt.Errorf("service: email already registered")
		}
	}

	createdUser, err := qtx.CreateUser(ctx, dbgen.CreateUserParams{
		AccountID:    accountID,
		Email:        req.Email,
		Username:     req.Username,
		PasswordHash: hash,
		Role:         req.Role,
	})
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	userID := createdUser.ID

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
		Email:             req.Email,
		Username:          req.Username,
		Role:              req.Role,
		PlaintextPassword: req.Password,
	}, nil
}

// ResetUserPassword hashes the new password with Authboss, updates users table, and revokes sessions.
func (svc *Service) ResetUserPassword(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newPassword string) error {
	if newPassword == "" {
		return fmt.Errorf("password cannot be empty")
	}

	hash, err := svc.ab.Config.Core.Hasher.GenerateHash(newPassword)
	if err != nil {
		return fmt.Errorf("service: hash password: %w", err)
	}

	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := dbgen.New(tx)

	exists, err := qtx.CheckUserExistsInAccount(ctx, dbgen.CheckUserExistsInAccountParams{
		ID:        targetUserID,
		AccountID: accountID,
	})
	if err != nil || !exists {
		return fmt.Errorf("user not found in account")
	}

	err = qtx.UpdateUserPassword(ctx, dbgen.UpdateUserPasswordParams{
		PasswordHash: hash,
		ID:           targetUserID,
		AccountID:    accountID,
	})
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	// Revoke all existing sessions for targetUserID
	err = qtx.DeleteSessionsByUserID(ctx, targetUserID.String())
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

// DeleteUser removes a user from an account and revokes existing sessions.
func (svc *Service) DeleteUser(ctx context.Context, accountID, actorID, targetUserID uuid.UUID) error {
	if actorID == targetUserID {
		return fmt.Errorf("cannot delete own account")
	}

	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := dbgen.New(tx)

	exists, err := qtx.CheckUserExistsInAccount(ctx, dbgen.CheckUserExistsInAccountParams{
		ID:        targetUserID,
		AccountID: accountID,
	})
	if err != nil || !exists {
		return fmt.Errorf("user not found in account")
	}

	err = qtx.DeleteUserFromAccount(ctx, dbgen.DeleteUserFromAccountParams{
		ID:        targetUserID,
		AccountID: accountID,
	})
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	// Revoke all existing sessions for targetUserID
	err = qtx.DeleteSessionsByUserID(ctx, targetUserID.String())
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

// ChangeUserRole updates the role of a user and revokes old sessions.
func (svc *Service) ChangeUserRole(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newRole string) error {
	if newRole != types.RoleManager && newRole != types.RoleAgent {
		return fmt.Errorf("invalid role: %q", newRole)
	}

	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := dbgen.New(tx)

	exists, err := qtx.CheckUserExistsInAccount(ctx, dbgen.CheckUserExistsInAccountParams{
		ID:        targetUserID,
		AccountID: accountID,
	})
	if err != nil || !exists {
		return fmt.Errorf("user not found in account")
	}

	err = qtx.UpdateUserRole(ctx, dbgen.UpdateUserRoleParams{
		Role:      newRole,
		ID:        targetUserID,
		AccountID: accountID,
	})
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}

	err = qtx.DeleteSessionsByUserID(ctx, targetUserID.String())
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

// pgxExecer wraps pgxpool.Pool to satisfy the audit.Writer's Exec interface.
type pgxExecer struct {
	pool *pgxpool.Pool
}

func (e *pgxExecer) Exec(ctx context.Context, sql string, args ...any) (interface{ RowsAffected() int64 }, error) {
	return e.pool.Exec(ctx, sql, args...)
}
