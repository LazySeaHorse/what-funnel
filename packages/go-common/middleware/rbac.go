// Package middleware provides HTTP middleware for authentication and RBAC
// enforcement. All middleware reads session data injected by identity-svc and
// enforces access control rules per spec §8.
package middleware

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// DefaultInternalServiceToken is the placeholder secret that must never be accepted.
const DefaultInternalServiceToken = "change-me-in-production-at-least-32-chars"

// MinInternalServiceTokenLength is the minimum accepted length of INTERNAL_SERVICE_TOKEN.
const MinInternalServiceTokenLength = 32

// InternalServiceSecret retrieves the configured secret for internal
// service-to-service communication. It is sourced only from
// INTERNAL_SERVICE_TOKEN; SESSION_SECRET is never reused for this purpose.
func InternalServiceSecret() string {
	return strings.TrimSpace(os.Getenv("INTERNAL_SERVICE_TOKEN"))
}

// ValidateInternalServiceToken verifies that a usable INTERNAL_SERVICE_TOKEN is
// configured (at least 32 chars and not the known placeholder). Services that
// rely on internal auth call this at startup. Setting
// ALLOW_INSECURE_INTERNAL_AUTH=true bypasses the check (development only).
func ValidateInternalServiceToken() error {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ALLOW_INSECURE_INTERNAL_AUTH")), "true") {
		return nil
	}
	secret := InternalServiceSecret()
	if secret == "" {
		return errors.New("INTERNAL_SERVICE_TOKEN is required")
	}
	if secret == DefaultInternalServiceToken {
		return errors.New("INTERNAL_SERVICE_TOKEN must not be the default placeholder")
	}
	if len(secret) < MinInternalServiceTokenLength {
		return fmt.Errorf("INTERNAL_SERVICE_TOKEN must be at least %d characters", MinInternalServiceTokenLength)
	}
	return nil
}

// IsAuthorizedInternalCall checks whether the provided internal token matches the expected service secret
// in constant time, and fails closed if token is empty or if expected secret is empty or the insecure default.
func IsAuthorizedInternalCall(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	secret := InternalServiceSecret()
	if secret == "" || secret == DefaultInternalServiceToken {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(secret)) == 1
}

// Queryer is a minimal interface matching pgxpool.Pool QueryRow.
type Queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// sessionStore is the interface we need from authboss / gorilla sessions.
// We keep it minimal so it can be satisfied by test fakes.
type sessionStore interface {
	GetUserID(r *http.Request) (uuid.UUID, bool)
	GetAccountID(r *http.Request) (uuid.UUID, bool)
	GetRole(r *http.Request) (string, bool)
}

// fullSessionStore is optionally implemented by stores that can return the whole
// session in one lookup, avoiding one backing-store query per field.
type fullSessionStore interface {
	GetSession(r *http.Request) (map[string]string, error)
}

// SessionMiddleware is the concrete middleware implementation.
type SessionMiddleware struct {
	store sessionStore
	pool  Queryer
}

// NewSessionMiddleware creates a middleware backed by the given session store.
func NewSessionMiddleware(store sessionStore) *SessionMiddleware {
	return &SessionMiddleware{store: store}
}

// NewSessionMiddlewareWithDB creates a middleware backed by the given session store and db.
func NewSessionMiddlewareWithDB(store sessionStore, pool Queryer) *SessionMiddleware {
	return &SessionMiddleware{store: store, pool: pool}
}

// RequireAuthenticated rejects unauthenticated requests with 401.
// On success it injects account_id, user_id, and role into the request context.
func (m *SessionMiddleware) RequireAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var userID uuid.UUID
		var accountID uuid.UUID
		var role string
		var authenticated bool

		internalToken := r.Header.Get("X-Internal-Token")
		if IsAuthorizedInternalCall(internalToken) {
			if acctIDStr := r.Header.Get("X-Account-ID"); acctIDStr != "" {
				if aid, err := uuid.Parse(acctIDStr); err == nil {
					accountID = aid
					authenticated = true
				}
			}
			if userIDStr := r.Header.Get("X-User-ID"); userIDStr != "" {
				if uid, err := uuid.Parse(userIDStr); err == nil {
					userID = uid
				}
			}
			role = r.Header.Get("X-User-Role")
			if role == "" {
				role = types.RoleManager
			}
		}

		var username string
		if !authenticated {
			// Prefer a single session lookup per request when the store supports it.
			if full, ok := m.store.(fullSessionStore); ok {
				data, err := full.GetSession(r)
				if err != nil {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
					return
				}
				var perr error
				if userID, perr = uuid.Parse(data["user_id"]); perr != nil {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
					return
				}
				if accountID, perr = uuid.Parse(data["account_id"]); perr != nil {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated: missing account"})
					return
				}
				var ok bool
				if role, ok = data["role"]; !ok {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated: missing role"})
					return
				}
				username = data["username"]
			} else {
				var ok bool
				userID, ok = m.store.GetUserID(r)
				if !ok {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
					return
				}
				accountID, ok = m.store.GetAccountID(r)
				if !ok {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated: missing account"})
					return
				}
				role, ok = m.store.GetRole(r)
				if !ok {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated: missing role"})
					return
				}
				if uStore, ok := m.store.(interface {
					GetUsername(r *http.Request) (string, bool)
				}); ok {
					username, _ = uStore.GetUsername(r)
				}
			}
		}

		ctx := r.Context()
		ctx = withValue(ctx, types.ContextKeyUserID, userID)
		ctx = withValue(ctx, types.ContextKeyAccountID, accountID)
		ctx = withValue(ctx, types.ContextKeyUserRole, role)
		if username != "" {
			ctx = withValue(ctx, types.ContextKeyUsername, username)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole rejects requests where the authenticated user's role does not
// match one of the allowed roles. Must be chained after RequireAuthenticated.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := r.Context().Value(types.ContextKeyUserRole).(string)
			if !ok || !allowed[role] {
				writeJSON(w, http.StatusForbidden, map[string]string{
					"error": "forbidden: insufficient role",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireManager is a convenience wrapper for RequireRole(manager).
func RequireManager(next http.Handler) http.Handler {
	return RequireRole(types.RoleManager)(next)
}

// RequireAdmin is an alias for RequireManager.
func RequireAdmin(next http.Handler) http.Handler {
	return RequireManager(next)
}

// RequireProductMode rejects requests if the account's product mode is not in the allowed list.
// Must be chained after RequireAuthenticated.
func (m *SessionMiddleware) RequireProductMode(allowedModes ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedModes))
	for _, mode := range allowedModes {
		allowed[mode] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			accountID, ok := AccountIDFromContext(r)
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated: missing account"})
				return
			}
			if m.pool == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "product mode verification: db pool not configured"})
				return
			}
			var productMode string
			err := m.pool.QueryRow(r.Context(), `SELECT product_mode FROM accounts WHERE id = $1`, accountID).Scan(&productMode)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify product mode: " + err.Error()})
				return
			}
			if !allowed[productMode] {
				writeJSON(w, http.StatusForbidden, map[string]string{
					"error": "forbidden: feature not available in current product mode",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AccountIDFromContext extracts the account_id from the request context.
// Returns uuid.Nil and false if not present.
func AccountIDFromContext(r *http.Request) (uuid.UUID, bool) {
	v, ok := r.Context().Value(types.ContextKeyAccountID).(uuid.UUID)
	return v, ok
}

// UserIDFromContext extracts the user_id from the request context.
func UserIDFromContext(r *http.Request) (uuid.UUID, bool) {
	v, ok := r.Context().Value(types.ContextKeyUserID).(uuid.UUID)
	return v, ok
}

// UsernameFromContext extracts the username from the request context.
func UsernameFromContext(r *http.Request) (string, bool) {
	v, ok := r.Context().Value(types.ContextKeyUsername).(string)
	return v, ok
}

// RoleFromContext extracts the role from the request context.
func RoleFromContext(r *http.Request) (string, bool) {
	v, ok := r.Context().Value(types.ContextKeyUserRole).(string)
	return v, ok
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body) //nolint:errcheck
}
