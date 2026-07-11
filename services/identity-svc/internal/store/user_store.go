// Package store implements the authboss.ServerStorer interface backed by Postgres.
// It also implements authboss.CreatingServerStorer so authboss can create users.
package store

import (
	"context"
	"fmt"
	"time"

	ab "github.com/aarondl/authboss/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// User implements authboss.User and holds all fields authboss needs.
type User struct {
	ID           uuid.UUID
	AccountID    uuid.UUID
	Email        string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
}

// GetPID returns the principal ID (email) used by authboss.
func (u *User) GetPID() string { return u.Email }

// PutPID sets the principal ID (email).
func (u *User) PutPID(pid string) { u.Email = pid }

// GetPassword returns the bcrypt hash.
func (u *User) GetPassword() string { return u.PasswordHash }

// PutPassword sets the bcrypt hash.
func (u *User) PutPassword(hash string) { u.PasswordHash = hash }

// Store implements authboss.ServerStorer and authboss.CreatingServerStorer.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a new Store backed by the given pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Load retrieves a user by PID (email). Returns ab.ErrUserNotFound if not found.
// NOTE: This searches across all accounts because authboss doesn't know about
// multi-tenancy. Callers in identity-svc should always validate the account_id
// from the session after Load.
func (s *Store) Load(ctx context.Context, key string) (ab.User, error) {
	u := &User{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, account_id, email, password_hash, role, created_at
		   FROM users WHERE email = $1 LIMIT 1`, key).
		Scan(&u.ID, &u.AccountID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err != nil {
		// pgx returns pgx.ErrNoRows on not found
		return nil, ab.ErrUserNotFound
	}
	return u, nil
}

// Save updates a user's mutable fields (only password_hash in our case,
// since authboss may update it during password change).
func (s *Store) Save(ctx context.Context, user ab.User) error {
	u, ok := user.(*User)
	if !ok {
		return fmt.Errorf("store: unexpected user type %T", user)
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET password_hash = $1 WHERE id = $2`,
		u.PasswordHash, u.ID)
	if err != nil {
		return fmt.Errorf("store: save user: %w", err)
	}
	return nil
}

// New returns a blank user struct for authboss to populate during registration.
func (s *Store) New(ctx context.Context) ab.User {
	return &User{}
}

// Create inserts a new user into the users table.
// Note: we do not call this directly from signup — the identity service
// uses a custom transaction that creates account + user + pipeline atomically.
// This method exists to satisfy the authboss.CreatingServerStorer interface.
func (s *Store) Create(ctx context.Context, user ab.User) error {
	u, ok := user.(*User)
	if !ok {
		return fmt.Errorf("store: unexpected user type %T", user)
	}
	if u.AccountID == uuid.Nil {
		return fmt.Errorf("store: account_id is required for user creation")
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO users (account_id, email, password_hash, role)
		 VALUES ($1, $2, $3, $4)`,
		u.AccountID, u.Email, u.PasswordHash, u.Role)
	if err != nil {
		return fmt.Errorf("store: create user: %w", err)
	}
	return nil
}
