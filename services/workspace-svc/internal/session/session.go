// Package session provides a read-only session reader for workspace-svc.
// workspace-svc does not create sessions (that's identity-svc's job), but it
// needs to read them for RBAC middleware integration.
// The implementation is identical to identity-svc's session package —
// we duplicate it rather than share it through go-common so each service
// remains independently deployable with its own session configuration.
package session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	sessionName     = "whatfunnel_session"
	sessionKeyUserID    = "user_id"
	sessionKeyAccountID = "account_id"
	sessionKeyRole      = "role"
	sessionTTL          = 30 * 24 * time.Hour
)

// Store provides session reading (and writing, for future use or shared middleware).
type Store struct {
	pool    *pgxpool.Pool
	codec   *securecookie.SecureCookie
	options *sessions.Options
}

// New creates a Store. secret must match the one used in identity-svc.
func New(pool *pgxpool.Pool, secret string) *Store {
	codec := securecookie.New([]byte(secret), nil)
	return &Store{
		pool:  pool,
		codec: codec,
		options: &sessions.Options{
			Path:     "/",
			MaxAge:   int(sessionTTL.Seconds()),
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		},
	}
}

// GetSession reads and validates the session from the request cookie.
func (s *Store) GetSession(r *http.Request) (map[string]string, error) {
	cookie, err := r.Cookie(sessionName)
	if err != nil {
		return nil, fmt.Errorf("session: no cookie")
	}

	var token string
	if err := s.codec.Decode(sessionName, cookie.Value, &token); err != nil {
		return nil, fmt.Errorf("session: invalid signature")
	}

	var raw []byte
	var expiry time.Time
	err = s.pool.QueryRow(r.Context(),
		`SELECT data, expiry FROM sessions WHERE token = $1`, token).
		Scan(&raw, &expiry)
	if err != nil {
		return nil, fmt.Errorf("session: not found")
	}
	if time.Now().After(expiry) {
		return nil, fmt.Errorf("session: expired")
	}

	var data map[string]string
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("session: unmarshal error")
	}
	return data, nil
}

// GetUserID extracts user_id from session.
func (s *Store) GetUserID(r *http.Request) (uuid.UUID, bool) {
	data, err := s.GetSession(r)
	if err != nil {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(data[sessionKeyUserID])
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// GetAccountID extracts account_id from session.
func (s *Store) GetAccountID(r *http.Request) (uuid.UUID, bool) {
	data, err := s.GetSession(r)
	if err != nil {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(data[sessionKeyAccountID])
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// GetRole extracts the user role from session.
func (s *Store) GetRole(r *http.Request) (string, bool) {
	data, err := s.GetSession(r)
	if err != nil {
		return "", false
	}
	role, ok := data[sessionKeyRole]
	return role, ok
}

// PurgeExpired deletes expired sessions.
func (s *Store) PurgeExpired(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expiry < NOW()`)
	return err
}

// RevokeUserSessions deletes all active sessions for the given user.
func (s *Store) RevokeUserSessions(ctx context.Context, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM sessions WHERE convert_from(data, 'UTF8')::jsonb->>'user_id' = $1`,
		userID.String())
	return err
}

// RevokeAccountSessions deletes all active sessions for the given account.
func (s *Store) RevokeAccountSessions(ctx context.Context, accountID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM sessions WHERE convert_from(data, 'UTF8')::jsonb->>'account_id' = $1`,
		accountID.String())
	return err
}

// unused but kept to avoid "imported and not used" errors when building
var _ = base64.StdEncoding
