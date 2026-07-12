// Package handler implements the HTTP API for workspace-svc.
//
// Routes (all protected by RequireAuthenticated; admin routes additionally by RequireAdmin):
//
//   GET    /workspace/account              — get account details
//   PUT    /workspace/account/settings     — update account settings (admin)
//   PUT    /workspace/account/ai-config    — update AI provider config (admin)
//
//   GET    /workspace/users                — list users (admin)
//   POST   /workspace/users/invite         — invite a user (admin)
//   PUT    /workspace/users/{id}/role      — change user role (admin)
//
//   GET    /workspace/pipelines            — list pipelines (admin + member)
//   PUT    /workspace/pipelines/{id}       — update pipeline (admin)
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/service"
	wsession "github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/session"
)

// Handler holds HTTP handler dependencies.
type Handler struct {
	svc  *service.Service
	sess *wsession.Store
	mw   *middleware.SessionMiddleware
}

// New creates a Handler.
func New(svc *service.Service, sess *wsession.Store) *Handler {
	mw := middleware.NewSessionMiddleware(sess)
	return &Handler{svc: svc, sess: sess, mw: mw}
}

// RegisterRoutes mounts all workspace-svc routes on the given router.
func (h *Handler) RegisterRoutes(r *mux.Router) {
	auth := h.mw.RequireAuthenticated
	admin := middleware.RequireAdmin

	// Account
	r.Handle("/workspace/account", auth(http.HandlerFunc(h.GetAccount))).Methods(http.MethodGet)
	r.Handle("/workspace/account/settings", auth(admin(http.HandlerFunc(h.UpdateSettings)))).Methods(http.MethodPut)
	r.Handle("/workspace/account/ai-config", auth(admin(http.HandlerFunc(h.UpdateAIConfig)))).Methods(http.MethodPut)

	r.Handle("/workspace/users", auth(admin(http.HandlerFunc(h.ListUsers)))).Methods(http.MethodGet)
	r.Handle("/workspace/users/invite", auth(admin(http.HandlerFunc(h.InviteUser)))).Methods(http.MethodPost)
	r.Handle("/workspace/users/{id}/role", auth(admin(http.HandlerFunc(h.ChangeUserRole)))).Methods(http.MethodPut)
	r.Handle("/workspace/users/me/reply-mode", auth(http.HandlerFunc(h.UpdateMyReplyMode))).Methods(http.MethodPatch)
	r.Handle("/users/me/reply-mode", auth(http.HandlerFunc(h.UpdateMyReplyMode))).Methods(http.MethodPatch)

	// Pipelines
	r.Handle("/workspace/pipelines", auth(http.HandlerFunc(h.ListPipelines))).Methods(http.MethodGet)
	r.Handle("/workspace/pipelines/{id}", auth(admin(http.HandlerFunc(h.UpdatePipeline)))).Methods(http.MethodPut)
}

// -------------------------------------------------------------------------
// Account handlers
// -------------------------------------------------------------------------

func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "no account in session")
		return
	}
	account, err := h.svc.GetAccount(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, account)
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	var settings map[string]any
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.svc.UpdateAccountSettings(r.Context(), accountID, actorID, settings); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) UpdateAIConfig(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	var body struct {
		Config string `json:"config"` // raw JSON string — will be encrypted before storage
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.svc.UpdateAIProviderConfig(r.Context(), accountID, actorID, body.Config); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// -------------------------------------------------------------------------
// User handlers
// -------------------------------------------------------------------------

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	users, err := h.svc.ListUsers(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *Handler) InviteUser(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	result, err := h.svc.InviteUser(r.Context(), accountID, actorID, service.InviteUserRequest{
		Email: req.Email,
		Role:  req.Role,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// TODO: wire to email provider — currently returns token in response for testing
	writeJSON(w, http.StatusCreated, map[string]string{
		"invite_token": result.Token,
		"email":        result.Email,
		"role":         result.Role,
		"note":         "email delivery is stubbed; use invite_token to register",
	})
}

func (h *Handler) ChangeUserRole(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	vars := mux.Vars(r)
	targetID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if err := h.svc.ChangeUserRole(r.Context(), accountID, actorID, targetID, body.Role); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// -------------------------------------------------------------------------
// Pipeline handlers
// -------------------------------------------------------------------------

func (h *Handler) ListPipelines(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	pipelines, err := h.svc.ListPipelines(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pipelines)
}

func (h *Handler) UpdatePipeline(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	vars := mux.Vars(r)
	pipelineID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid pipeline id")
		return
	}

	var body struct {
		Name   string                 `json:"name"`
		States []types.PipelineState  `json:"states"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if err := h.svc.UpdatePipeline(r.Context(), accountID, actorID, pipelineID, service.UpdatePipelineRequest{
		Name:   body.Name,
		States: body.States,
	}); err != nil {
		if inUseErr, ok := err.(*service.ErrPipelineInUse); ok {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":    "in_use",
				"message":  inUseErr.Error(),
				"lead_ids": inUseErr.LeadIDs,
			})
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) UpdateMyReplyMode(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}
	userID, ok := middleware.UserIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user")
		return
	}

	var body struct {
		ReplyMode *string `json:"reply_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if body.ReplyMode != nil {
		mode := *body.ReplyMode
		if mode != "" && mode != "auto_send" && mode != "draft_only" {
			writeError(w, http.StatusBadRequest, "invalid reply_mode")
			return
		}
	}

	if err := h.svc.UpdateUserReplyMode(r.Context(), accountID, userID, body.ReplyMode); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
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
