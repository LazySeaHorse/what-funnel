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
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	sessionName         = "whatfunnel_session"
	csrfCookieName      = "csrf_token"
	sessionKeyUserID    = "user_id"
	sessionKeyAccountID = "account_id"
	sessionKeyUsername  = "username"
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
// If secure is not provided, it defaults to true in production or when COOKIE_SECURE=true.
func New(pool *pgxpool.Pool, secret string, secure ...bool) *Store {
	codec := securecookie.New([]byte(secret), nil)
	isSecure := false
	if len(secure) > 0 {
		isSecure = secure[0]
	} else {
		env := strings.ToLower(os.Getenv("ENV"))
		if env == "" {
			env = strings.ToLower(os.Getenv("ENVIRONMENT"))
		}
		if env == "" {
			env = strings.ToLower(os.Getenv("APP_ENV"))
		}
		isSecure = env == "production" || env == "prod" || os.Getenv("COOKIE_SECURE") == "true" || os.Getenv("COOKIE_SECURE") == "1"
	}

	return &Store{
		pool:  pool,
		codec: codec,
		options: &sessions.Options{
			Path:     "/",
			MaxAge:   int(sessionTTL.Seconds()),
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   isSecure,
		},
	}
}

// -------------------------------------------------------------------------
// Low-level session operations
// -------------------------------------------------------------------------

// SetSession writes user identity into the session and persists it.
func (s *Store) SetSession(w http.ResponseWriter, r *http.Request,
	userID, accountID uuid.UUID, role string, username ...string) error {

	token, err := s.newToken()
	if err != nil {
		return err
	}

	uName := ""
	if len(username) > 0 {
		uName = username[0]
	}

	data := map[string]string{
		sessionKeyUserID:    userID.String(),
		sessionKeyAccountID: accountID.String(),
		sessionKeyUsername:  uName,
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
		Secure:   s.options.Secure,
	})

	// Set readable CSRF token cookie for double-submit protection
	csrfToken, err := s.newToken()
	if err == nil {
		http.SetCookie(w, &http.Cookie{
			Name:     csrfCookieName,
			Value:    csrfToken,
			Path:     s.options.Path,
			MaxAge:   s.options.MaxAge,
			HttpOnly: false, // Accessible to client JavaScript
			SameSite: s.options.SameSite,
			Secure:   s.options.Secure,
		})
	}
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
	if s.pool != nil {
		_, err = s.pool.Exec(r.Context(), `DELETE FROM sessions WHERE token = $1`, token)
		if err != nil {
			return fmt.Errorf("session: destroy: %w", err)
		}
	}
	// Expire the session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     sessionName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.options.Secure,
	})
	// Expire the CSRF cookie
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: false,
		Secure:   s.options.Secure,
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

// GetUsername extracts the username from session.
func (s *Store) GetUsername(r *http.Request) (string, bool) {
	data, err := s.GetSession(r)
	if err != nil {
		return "", false
	}
	username, ok := data[sessionKeyUsername]
	return username, ok
}

// -------------------------------------------------------------------------
// Housekeeping
// -------------------------------------------------------------------------

// PurgeExpired deletes expired sessions. Call from a periodic goroutine or cron.
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

// newToken generates a cryptographically random session token.
func (s *Store) newToken() (string, error) {
	key := securecookie.GenerateRandomKey(32)
	if key == nil {
		return "", fmt.Errorf("session: failed to generate random token")
	}
	return base64.RawURLEncoding.EncodeToString(key), nil
}
