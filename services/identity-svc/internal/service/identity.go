// Package service implements the business logic for account/user lifecycle:
// signup, login, logout, and user lookup. authboss handles password hashing
// (bcrypt); we handle account creation and tenant isolation setup.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	ab "github.com/aarondl/authboss/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/identity-svc/internal/session"
	"github.com/whatfunnel/whatfunnel/services/identity-svc/internal/store"
)

// Service handles auth lifecycle: signup, login, logout.
type Service struct {
	pool     *pgxpool.Pool
	sessions *session.Store
	ab       *ab.Authboss
}

// SignupRequest carries the fields needed to create an account + admin user,
// or to redeem an invite token.
type SignupRequest struct {
	AccountName string `json:"account_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	InviteToken string `json:"invite_token"`
}

// LoginRequest carries credentials for login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// New creates a Service wired to the given pool and session store.
func New(pool *pgxpool.Pool, sessions *session.Store) (*Service, error) {
	cfg := ab.New()
	cfg.Config.Paths.RootURL = "http://localhost:8081"
	// We use authboss only for its password-hashing primitives in this
	// integration. The auth modules (auth, register, etc.) are bypassed
	// in favour of our own HTTP handlers, which call authboss's bcrypt helpers.
	if err := cfg.Init(); err != nil {
		return nil, fmt.Errorf("service: authboss init: %w", err)
	}

	return &Service{
		pool:     pool,
		sessions: sessions,
		ab:       cfg,
	}, nil
}

// Signup creates an account, admin user, and default pipeline in one atomic
// transaction. If an invite token is provided, it associates the new user
// with the existing account and role instead.
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

	aw := audit.NewWriterFromTx(tx)

	var accountID uuid.UUID
	var userID uuid.UUID
	var userRole string
	isInvite := req.InviteToken != ""

	if isInvite {
		var invitedEmail string
		// 1. Look up and validate invite token
		err = tx.QueryRow(ctx,
			`SELECT account_id, email, role FROM invite_tokens WHERE token = $1`,
			req.InviteToken).Scan(&accountID, &invitedEmail, &userRole)
		if err != nil {
			return nil, fmt.Errorf("service: invalid or expired invite token")
		}

		if req.Email != invitedEmail {
			return nil, fmt.Errorf("service: email does not match the invitation")
		}

		// 2. Check email uniqueness within account
		var count int
		_ = tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM users WHERE account_id = $1 AND email = $2`,
			accountID, req.Email).Scan(&count)
		if count > 0 {
			return nil, fmt.Errorf("service: email already registered")
		}

		// 3. Create user with role from invite
		err = tx.QueryRow(ctx,
			`INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, $2, $3, $4) RETURNING id`,
			accountID, req.Email, hash, userRole).Scan(&userID)
		if err != nil {
			return nil, fmt.Errorf("service: create user from invite: %w", err)
		}

		// 4. Delete the redeemed token
		_, err = tx.Exec(ctx, `DELETE FROM invite_tokens WHERE token = $1`, req.InviteToken)
		if err != nil {
			return nil, fmt.Errorf("service: consume invite token: %w", err)
		}
	} else {
		userRole = types.RoleAdmin
		// 1. Create account
		err = tx.QueryRow(ctx,
			`INSERT INTO accounts (name, plan) VALUES ($1, $2) RETURNING id`,
			req.AccountName, types.PlanSelfHosted).Scan(&accountID)
		if err != nil {
			return nil, fmt.Errorf("service: create account: %w", err)
		}

		// 2. Check email uniqueness within account
		var count int
		_ = tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM users WHERE account_id = $1 AND email = $2`,
			accountID, req.Email).Scan(&count)
		if count > 0 {
			return nil, fmt.Errorf("service: email already registered")
		}

		// 3. Create admin user
		err = tx.QueryRow(ctx,
			`INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, $2, $3, $4) RETURNING id`,
			accountID, req.Email, hash, userRole).Scan(&userID)
		if err != nil {
			return nil, fmt.Errorf("service: create user: %w", err)
		}

		// 4. Seed default pipeline
		statesJSON, err := json.Marshal(types.DefaultPipelineStates)
		if err != nil {
			return nil, fmt.Errorf("service: marshal default states: %w", err)
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO lead_pipelines (account_id, name, states) VALUES ($1, $2, $3)`,
			accountID, "Default Pipeline", statesJSON)
		if err != nil {
			return nil, fmt.Errorf("service: seed default pipeline: %w", err)
		}

		// 5. Write account audit log
		if err := aw.Write(ctx, audit.Entry{
			AccountID:  accountID,
			ActorUserID: &userID,
			Action:     audit.ActionAccountCreated,
			TargetType: audit.TargetAccount,
			TargetID:   &accountID,
			Metadata:   map[string]any{"account_name": req.AccountName},
		}); err != nil {
			return nil, fmt.Errorf("service: audit account: %w", err)
		}
	}

	// Common User Creation Audit Log
	if err := aw.Write(ctx, audit.Entry{
		AccountID:  accountID,
		ActorUserID: &userID,
		Action:     audit.ActionUserCreated,
		TargetType: audit.TargetUser,
		TargetID:   &userID,
		Metadata:   map[string]any{"email": req.Email, "role": userRole},
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
		Role:      userRole,
		CreatedAt: time.Now(),
	}, nil
}

// Login verifies credentials and returns the user if valid.
func (svc *Service) Login(ctx context.Context, req LoginRequest) (*store.User, error) {
	u := &store.User{}
	err := svc.pool.QueryRow(ctx,
		`SELECT id, account_id, email, password_hash, role, created_at
		   FROM users WHERE email = $1 LIMIT 1`, req.Email).
		Scan(&u.ID, &u.AccountID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("service: invalid credentials")
		}
		return nil, fmt.Errorf("service: lookup user: %w", err)
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
	type execer struct{ pool *pgxpool.Pool }
	type execResult struct{ rows int64 }
	aw := audit.NewWriter(&pgxExecer{pool: svc.pool})
	return aw.Write(ctx, audit.Entry{
		AccountID:  accountID,
		ActorUserID: &userID,
		Action:     audit.ActionLogin,
		TargetType: audit.TargetUser,
		TargetID:   &userID,
		Metadata:   map[string]any{},
	})
}

// SetSession creates a session for the given user.
func (svc *Service) SetSession(w http.ResponseWriter, r *http.Request, u *store.User) error {
	return svc.sessions.SetSession(w, r, u.ID, u.AccountID, u.Role)
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
				AccountID:  accountID,
				ActorUserID: &userID,
				Action:     audit.ActionLogout,
				TargetType: audit.TargetUser,
				TargetID:   &userID,
				Metadata:   map[string]any{},
			})
		}
	}
	return nil
}

// pgxExecer wraps pgxpool.Pool to satisfy the audit.Writer's Exec interface.
type pgxExecer struct {
	pool *pgxpool.Pool
}

func (e *pgxExecer) Exec(ctx context.Context, sql string, args ...any) (interface{ RowsAffected() int64 }, error) {
	return e.pool.Exec(ctx, sql, args...)
}
