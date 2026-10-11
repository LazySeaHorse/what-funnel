// Package session reads the server-side sessions identity-svc creates.
// The cookie holds only a signed random token; the session data lives in the
// Postgres sessions table, so deleting a row logs the user out everywhere.
package session

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// CookieName is the session cookie carrying the signed token.
	CookieName = "whatfunnel_session"
	// TTL is how long a session (and its cookie) stays valid.
	TTL = 30 * 24 * time.Hour

	KeyUserID    = "user_id"
	KeyAccountID = "account_id"
	KeyUsername  = "username"
	KeyRole      = "role"
)

// Reader validates session cookies and loads the session data.
type Reader struct {
	pool  *pgxpool.Pool
	codec *securecookie.SecureCookie
}

// NewReader creates a Reader. secret must match the one identity-svc signs cookies with.
func NewReader(pool *pgxpool.Pool, secret string) *Reader {
	return &Reader{pool: pool, codec: securecookie.New([]byte(secret), nil)}
}

// Token returns the session token from the request's signed cookie.
func (s *Reader) Token(r *http.Request) (string, error) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return "", fmt.Errorf("session: no cookie: %w", err)
	}
	var token string
	if err := s.codec.Decode(CookieName, cookie.Value, &token); err != nil {
		return "", fmt.Errorf("session: invalid cookie signature: %w", err)
	}
	return token, nil
}

// EncodeToken signs a session token for the cookie value.
func (s *Reader) EncodeToken(token string) (string, error) {
	return s.codec.Encode(CookieName, token)
}

// GetSession reads and validates the session from the request cookie.
// Returns an error if the session is absent, expired, or tampered.
func (s *Reader) GetSession(r *http.Request) (map[string]string, error) {
	token, err := s.Token(r)
	if err != nil {
		return nil, err
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
