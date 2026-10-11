// Package session is identity-svc's session store: it creates and destroys
// the server-side sessions every service reads through go-common/session.
// Deleting a row in the sessions table logs the user out.
package session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/config"
	commonsession "github.com/whatfunnel/whatfunnel/packages/go-common/session"
)

const csrfCookieName = "csrf_token"

// Store writes sessions; reading (GetSession) comes from the embedded shared Reader.
type Store struct {
	*commonsession.Reader
	pool    *pgxpool.Pool
	options *sessions.Options
}

// New creates a Store. secret must be >= 32 bytes.
// If secure is not provided, it defaults to true in production or when COOKIE_SECURE=true.
func New(pool *pgxpool.Pool, secret string, secure ...bool) *Store {
	isSecure := false
	if len(secure) > 0 {
		isSecure = secure[0]
	} else {
		isSecure = config.IsProduction() || os.Getenv("COOKIE_SECURE") == "true" || os.Getenv("COOKIE_SECURE") == "1"
	}

	return &Store{
		Reader: commonsession.NewReader(pool, secret),
		pool:   pool,
		options: &sessions.Options{
			Path:     "/",
			MaxAge:   int(commonsession.TTL.Seconds()),
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
		commonsession.KeyUserID:    userID.String(),
		commonsession.KeyAccountID: accountID.String(),
		commonsession.KeyUsername:  uName,
		commonsession.KeyRole:      role,
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("session: marshal: %w", err)
	}

	expiry := time.Now().Add(commonsession.TTL)
	_, err = s.pool.Exec(r.Context(),
		`INSERT INTO sessions (token, data, expiry) VALUES ($1, $2, $3)
		 ON CONFLICT (token) DO UPDATE SET data = EXCLUDED.data, expiry = EXCLUDED.expiry`,
		token, encoded, expiry)
	if err != nil {
		return fmt.Errorf("session: persist: %w", err)
	}

	// Write signed token to cookie
	encoded2, err := s.EncodeToken(token)
	if err != nil {
		return fmt.Errorf("session: encode cookie: %w", err)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     commonsession.CookieName,
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

// DestroySession invalidates the session (logout).
func (s *Store) DestroySession(w http.ResponseWriter, r *http.Request) error {
	token, err := s.Token(r)
	if err != nil {
		return nil // no (valid) session cookie: nothing to destroy
	}
	if s.pool != nil {
		_, err = s.pool.Exec(r.Context(), `DELETE FROM sessions WHERE token = $1`, token)
		if err != nil {
			return fmt.Errorf("session: destroy: %w", err)
		}
	}
	// Expire the session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     commonsession.CookieName,
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
// Housekeeping
// -------------------------------------------------------------------------

// PurgeExpired deletes expired sessions and returns the number of deleted records.
func (s *Store) PurgeExpired(ctx context.Context) (int64, error) {
	if s.pool == nil {
		return 0, fmt.Errorf("session: pool is nil")
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expiry < NOW()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Janitor handles background periodic purging of expired sessions.
type Janitor struct {
	store    *Store
	interval time.Duration
	logger   *slog.Logger
}

// NewJanitor creates a Janitor with the given interval and logger.
// Defaults to 1 hour if interval <= 0, and slog.Default() if logger is nil.
func NewJanitor(store *Store, interval time.Duration, logger *slog.Logger) *Janitor {
	if interval <= 0 {
		interval = time.Hour
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Janitor{
		store:    store,
		interval: interval,
		logger:   logger,
	}
}

// PurgeOnce runs a single pass of expired session purging.
func (j *Janitor) PurgeOnce(ctx context.Context) (int64, error) {
	return j.store.PurgeExpired(ctx)
}

// Run executes the janitor loop, performing an initial purge and then purging periodically
// at the configured interval until ctx is canceled.
func (j *Janitor) Run(ctx context.Context) error {
	// Initial purge on startup
	if count, err := j.PurgeOnce(ctx); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		j.logger.Error("failed initial purge of expired sessions", "error", err)
	} else if count > 0 {
		j.logger.Info("purged expired sessions", "count", count)
	} else {
		j.logger.Debug("session purge completed; no expired sessions")
	}

	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			count, err := j.PurgeOnce(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				j.logger.Error("failed to purge expired sessions", "error", err)
			} else if count > 0 {
				j.logger.Info("purged expired sessions", "count", count)
			} else {
				j.logger.Debug("session purge completed; no expired sessions")
			}
		}
	}
}

// newToken generates a cryptographically random session token.
func (s *Store) newToken() (string, error) {
	key := securecookie.GenerateRandomKey(32)
	if key == nil {
		return "", fmt.Errorf("session: failed to generate random token")
	}
	return base64.RawURLEncoding.EncodeToString(key), nil
}
