// Package store loads users for login from Postgres.
package store

import (
	"context"
	"strings"
	"time"

	ab "github.com/aarondl/authboss/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db/dbgen"
)

// User is a row of the users table as login needs it.
type User struct {
	ID           uuid.UUID
	AccountID    uuid.UUID
	Email        string
	Username     string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
}

// Store looks users up by login identifier.
type Store struct {
	queries *dbgen.Queries
}

// New creates a new Store backed by the given pool.
func New(pool *pgxpool.Pool) *Store {
	var q *dbgen.Queries
	if pool != nil {
		q = dbgen.New(pool)
	}
	return &Store{queries: q}
}

// LoadByIdentifier retrieves a user by email or slug/username identifier.
// Formats:
// - Email: user@domain.com
// - Slash-delimited: slug/username (unambiguous, index-backed)
// - Legacy hyphen-delimited: slug-username (supported with multi-tenant collision detection)
func (s *Store) LoadByIdentifier(ctx context.Context, identifier string) (*User, error) {
	if strings.Contains(identifier, "@") {
		row, err := s.queries.GetUserByEmail(ctx, identifier)
		if err != nil {
			return nil, ab.ErrUserNotFound
		}
		return &User{
			ID:           row.ID,
			AccountID:    row.AccountID,
			Email:        row.Email,
			Username:     row.Username,
			PasswordHash: row.PasswordHash,
			Role:         row.Role,
			CreatedAt:    row.CreatedAt,
		}, nil
	}

	if strings.Contains(identifier, "/") {
		slug, username, _ := strings.Cut(identifier, "/")
		if slug == "" || username == "" {
			return nil, ab.ErrUserNotFound
		}
		row, err := s.queries.GetUserBySlugAndUsername(ctx, dbgen.GetUserBySlugAndUsernameParams{
			Slug:     slug,
			Username: username,
		})
		if err != nil {
			return nil, ab.ErrUserNotFound
		}
		return &User{
			ID:           row.ID,
			AccountID:    row.AccountID,
			Email:        row.Email,
			Username:     row.Username,
			PasswordHash: row.PasswordHash,
			Role:         row.Role,
			CreatedAt:    row.CreatedAt,
		}, nil
	}

	// Legacy hyphenated identifier: query all matches to prevent cross-tenant collision.
	rows, err := s.queries.ListUsersByLegacySlugIdentifier(ctx, identifier)
	if err != nil || len(rows) == 0 {
		return nil, ab.ErrUserNotFound
	}
	if len(rows) > 1 {
		// Collision: multiple users across distinct tenants match this ambiguous identifier.
		// Reject ambiguous match to prevent cross-tenant account takeover / shadowing.
		return nil, ab.ErrUserNotFound
	}

	row := rows[0]
	u := User(row)
	return &u, nil
}
