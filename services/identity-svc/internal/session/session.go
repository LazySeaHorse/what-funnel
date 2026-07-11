// Package session provides a gorilla/sessions-based session store for authboss,
// using a Postgres-backed store for server-side sessions (not cookie-only).
// This enables trivial revocation — delete the row to log the user out.
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
	sessionName      = "whatfunnel_session"
	sessionKeyUserID    = "user_id"
	sessionKeyAccountID = "account_id"
	sessionKeyRole      = "role"
	sessionTTL          = 30 * 24 * time.Hour // 30 days
)

// Store wraps gorilla/sessions and Postgres for server-side session storage.
// The cookie holds only a random token; the session data lives in Postgres.
type Store struct {
	pool    *pgxpool.Pool
	codec   *securecookie.SecureCookie
	options *sessions.Options
}

// New creates a Store. secret must be >= 32 bytes.
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
			// Secure: true in production (behind TLS)
		},
	}
}

// -------------------------------------------------------------------------
// Low-level session operations
// -------------------------------------------------------------------------

// SetSession writes user identity into the session and persists it.
func (s *Store) SetSession(w http.ResponseWriter, r *http.Request,
	userID, accountID uuid.UUID, role string) error {

	token, err := s.newToken()
	if err != nil {
		return err
	}

	data := map[string]string{
		sessionKeyUserID:    userID.String(),
		sessionKeyAccountID: accountID.String(),
		sessionKeyRole:      role,
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("session: marshal: %w", err)
	}

	expiry := time.Now().Add(sessionTTL)
	_, err = s.pool.Exec(r.Context(),
		`INSERT INTO sessions (token, data, expiry) VALUES ($1, $2, $3)
		 ON CONFLICT (token) DO UPDATE SET data = EXCLUDED.data, expiry = EXCLUDED.expiry`,
		token, encoded, expiry)
	if err != nil {
		return fmt.Errorf("session: persist: %w", err)
	}

	// Write signed token to cookie
	encoded2, err := s.codec.Encode(sessionName, token)
	if err != nil {
		return fmt.Errorf("session: encode cookie: %w", err)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionName,
		Value:    encoded2,
		Path:     s.options.Path,
		MaxAge:   s.options.MaxAge,
		HttpOnly: s.options.HttpOnly,
		SameSite: s.options.SameSite,
	})
	return nil
}

// GetSession reads and validates the session from the request cookie.
// Returns an error if the session is absent, expired, or tampered.
func (s *Store) GetSession(r *http.Request) (map[string]string, error) {
	cookie, err := r.Cookie(sessionName)
	if err != nil {
		return nil, fmt.Errorf("session: no cookie: %w", err)
	}

	var token string
	if err := s.codec.Decode(sessionName, cookie.Value, &token); err != nil {
		return nil, fmt.Errorf("session: invalid cookie signature: %w", err)
	}

	var raw []byte
	var expiry time.Time
	err = s.pool.QueryRow(r.Context(),
		`SELECT data, expiry FROM sessions WHERE token = $1`, token).
		Scan(&raw, &expiry)
	if err != nil {
		return nil, fmt.Errorf("session: not found in store: %w", err)
	}
	if time.Now().After(expiry) {
		return nil, fmt.Errorf("session: expired")
	}

	var data map[string]string
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("session: unmarshal: %w", err)
	}
	return data, nil
}

// DestroySession invalidates the session (logout).
func (s *Store) DestroySession(w http.ResponseWriter, r *http.Request) error {
	cookie, err := r.Cookie(sessionName)
	if err != nil {
		return nil // no session to destroy
	}
	var token string
	if err := s.codec.Decode(sessionName, cookie.Value, &token); err != nil {
		return nil // can't decode — treat as already gone
	}
	_, err = s.pool.Exec(r.Context(), `DELETE FROM sessions WHERE token = $1`, token)
	if err != nil {
		return fmt.Errorf("session: destroy: %w", err)
	}
	// Expire the cookie
	http.SetCookie(w, &http.Cookie{
		Name:     sessionName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	return nil
}

// -------------------------------------------------------------------------
// Helpers used by go-common middleware (sessionStore interface)
// -------------------------------------------------------------------------

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

// -------------------------------------------------------------------------
// Housekeeping
// -------------------------------------------------------------------------

// PurgeExpired deletes expired sessions. Call from a periodic goroutine or cron.
func (s *Store) PurgeExpired(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expiry < NOW()`)
	return err
}

// newToken generates a cryptographically random session token.
func (s *Store) newToken() (string, error) {
	key := securecookie.GenerateRandomKey(32)
	if key == nil {
		return "", fmt.Errorf("session: failed to generate random token")
	}
	return base64.RawURLEncoding.EncodeToString(key), nil
}
