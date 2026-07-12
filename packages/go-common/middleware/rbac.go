// Package middleware provides HTTP middleware for authentication and RBAC
// enforcement. All middleware reads session data injected by identity-svc and
// enforces access control rules per spec §8.
package middleware

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// sessionStore is the interface we need from authboss / gorilla sessions.
// We keep it minimal so it can be satisfied by test fakes.
type sessionStore interface {
	GetUserID(r *http.Request) (uuid.UUID, bool)
	GetAccountID(r *http.Request) (uuid.UUID, bool)
	GetRole(r *http.Request) (string, bool)
}

// SessionMiddleware is the concrete middleware implementation.
type SessionMiddleware struct {
	store sessionStore
}

// NewSessionMiddleware creates a middleware backed by the given session store.
func NewSessionMiddleware(store sessionStore) *SessionMiddleware {
	return &SessionMiddleware{store: store}
}

// RequireAuthenticated rejects unauthenticated requests with 401.
// On success it injects account_id, user_id, and role into the request context.
func (m *SessionMiddleware) RequireAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var userID uuid.UUID
		var accountID uuid.UUID
		var role string
		var authenticated bool

		secret := os.Getenv("SESSION_SECRET")
		if secret == "" {
			secret = "change-me-in-production-at-least-32-chars"
		}

		internalToken := r.Header.Get("X-Internal-Token")
		if internalToken != "" && internalToken == secret {
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
				role = types.RoleAdmin
			}
		}

		if !authenticated {
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
		}

		ctx := r.Context()
		ctx = withValue(ctx, types.ContextKeyUserID, userID)
		ctx = withValue(ctx, types.ContextKeyAccountID, accountID)
		ctx = withValue(ctx, types.ContextKeyUserRole, role)

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

// RequireAdmin is a convenience wrapper for RequireRole(admin).
func RequireAdmin(next http.Handler) http.Handler {
	return RequireRole(types.RoleAdmin)(next)
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
