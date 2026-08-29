// Package handler implements the HTTP API for the identity service.
// Routes:
//   POST /auth/signup   — create account + admin user
//   POST /auth/login    — authenticate
//   POST /auth/logout   — invalidate session
//   GET  /auth/me       — return current user info (requires auth)
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/services/identity-svc/internal/service"
	"github.com/whatfunnel/whatfunnel/services/identity-svc/internal/session"
)

// Handler holds HTTP handler dependencies.
type Handler struct {
	svc     *service.Service
	session *session.Store
	mw      *middleware.SessionMiddleware
}

// New creates a Handler.
func New(svc *service.Service, sess *session.Store) *Handler {
	mw := middleware.NewSessionMiddleware(sess)
	return &Handler{svc: svc, session: sess, mw: mw}
}

// RegisterRoutes mounts all identity-svc routes on the given router.
func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/auth/signup", h.Signup).Methods(http.MethodPost)
	r.HandleFunc("/auth/login", h.Login).Methods(http.MethodPost)
	r.HandleFunc("/auth/logout", h.Logout).Methods(http.MethodPost)
	r.Handle("/auth/me", h.mw.RequireAuthenticated(http.HandlerFunc(h.Me))).Methods(http.MethodGet)
}

// Signup handles POST /auth/signup.
func (h *Handler) Signup(w http.ResponseWriter, r *http.Request) {
	var req service.SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AccountName == "" || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "account_name, email, and password are required")
		return
	}

	user, err := h.svc.Signup(r.Context(), req)
	if err != nil {
		// TODO: differentiate duplicate email from other errors
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"user_id":    user.ID,
		"account_id": user.AccountID,
		"email":      user.Email,
		"username":   user.Username,
		"role":       user.Role,
	})
}

// Login handles POST /auth/login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req service.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if (req.Identifier == "" && req.Email == "") || req.Password == "" {
		writeError(w, http.StatusBadRequest, "identifier and password are required")
		return
	}

	user, err := h.svc.Login(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := h.svc.SetSession(w, r, user); err != nil {
		writeError(w, http.StatusInternalServerError, "session error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":    user.ID,
		"account_id": user.AccountID,
		"email":      user.Email,
		"username":   user.Username,
		"role":       user.Role,
	})
}

// Logout handles POST /auth/logout.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Logout(w, r); err != nil {
		writeError(w, http.StatusInternalServerError, "logout error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// Me handles GET /auth/me — returns the current user from session.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r)
	accountID, _ := middleware.AccountIDFromContext(r)
	username, _ := middleware.UsernameFromContext(r)
	role, _ := middleware.RoleFromContext(r)

	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":    userID,
		"account_id": accountID,
		"username":   username,
		"role":       role,
	})
}

// -------------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
