// Package handler implements the HTTP API for workspace-svc.
//
// Routes (all protected by RequireAuthenticated; admin routes additionally by RequireAdmin):
//
//	GET    /workspace/account              — get account details
//	PUT    /workspace/account/settings     — update account settings (admin)
//	PATCH  /workspace/account/settings     — merge account settings (admin)
//	PUT    /workspace/account/ai-config    — update AI provider config (admin)
//	GET    /workspace/account/ai-config/status — report whether AI is configured
//
//	GET    /workspace/users                — list users (admin)
//	POST   /workspace/users/invite         — invite a user (admin)
//	PUT    /workspace/users/{id}/role      — change user role (admin)
//
//	GET    /workspace/pipelines            — list pipelines (admin + member)
//	PUT    /workspace/pipelines/{id}       — update pipeline (admin)
//
//	GET    /onboarding/status              — get onboarding progress (auth)
//	PATCH  /onboarding/status              — patch onboarding step (auth)
//	GET    /onboarding/templates           — list business-type templates (auth)
//	POST   /onboarding/apply-template      — apply a template (admin)
package handler

import (
	"encoding/json"
	"net/http"
	"strings"

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
	mw := middleware.NewSessionMiddlewareWithDB(sess, svc.Pool())
	return &Handler{svc: svc, sess: sess, mw: mw}
}

// RegisterRoutes mounts all workspace-svc routes on the given router.
func (h *Handler) RegisterRoutes(r *mux.Router) {
	auth := h.mw.RequireAuthenticated
	admin := middleware.RequireAdmin
	fullWorkspace := h.mw.RequireProductMode("full_workspace")

	// Account
	r.Handle("/workspace/account", auth(http.HandlerFunc(h.GetAccount))).Methods(http.MethodGet)
	r.Handle("/workspace/account", auth(admin(http.HandlerFunc(h.UpdateAccountName)))).Methods(http.MethodPatch)
	r.Handle("/workspace/account", auth(admin(http.HandlerFunc(h.DeleteAccount)))).Methods(http.MethodDelete)
	r.Handle("/workspace/account/slug", auth(admin(http.HandlerFunc(h.SetAccountSlug)))).Methods(http.MethodPut)
	r.Handle("/workspace/account/slug", auth(http.HandlerFunc(h.GetAccountSlug))).Methods(http.MethodGet)
	r.Handle("/workspace/account/settings", auth(admin(http.HandlerFunc(h.UpdateSettings)))).Methods(http.MethodPut)
	r.Handle("/workspace/account/settings", auth(admin(http.HandlerFunc(h.PatchSettings)))).Methods(http.MethodPatch)
	r.Handle("/workspace/account/ai-config", auth(admin(http.HandlerFunc(h.UpdateAIConfig)))).Methods(http.MethodPut)
	r.Handle("/workspace/account/ai-config/status", auth(http.HandlerFunc(h.GetAIConfigStatus))).Methods(http.MethodGet)
	r.Handle("/workspace/account/product-mode", auth(admin(http.HandlerFunc(h.UpdateProductMode)))).Methods(http.MethodPatch)
	r.Handle("/account/product-mode", auth(admin(http.HandlerFunc(h.UpdateProductMode)))).Methods(http.MethodPatch)

	// Users
	r.Handle("/workspace/users", auth(admin(http.HandlerFunc(h.ListUsers)))).Methods(http.MethodGet)
	r.Handle("/workspace/users", auth(admin(http.HandlerFunc(h.CreateUser)))).Methods(http.MethodPost)
	r.Handle("/workspace/users/{id}", auth(admin(http.HandlerFunc(h.DeleteUser)))).Methods(http.MethodDelete)
	r.Handle("/workspace/users/{id}/password", auth(admin(http.HandlerFunc(h.ResetUserPassword)))).Methods(http.MethodPut)
	r.Handle("/workspace/users/{id}/role", auth(admin(http.HandlerFunc(h.ChangeUserRole)))).Methods(http.MethodPut)
	r.Handle("/workspace/users/me/reply-mode", auth(http.HandlerFunc(h.UpdateMyReplyMode))).Methods(http.MethodPatch)
	r.Handle("/users/me/reply-mode", auth(http.HandlerFunc(h.UpdateMyReplyMode))).Methods(http.MethodPatch)

	// Pipelines
	r.Handle("/workspace/pipelines", auth(fullWorkspace(http.HandlerFunc(h.ListPipelines)))).Methods(http.MethodGet)
	r.Handle("/workspace/pipelines/{id}", auth(admin(fullWorkspace(http.HandlerFunc(h.UpdatePipeline))))).Methods(http.MethodPut)

	// Onboarding
	r.Handle("/onboarding/status", auth(http.HandlerFunc(h.GetOnboardingStatus))).Methods(http.MethodGet)
	r.Handle("/onboarding/status", auth(http.HandlerFunc(h.PatchOnboardingStatus))).Methods(http.MethodPatch)
	r.Handle("/onboarding/templates", auth(http.HandlerFunc(h.GetOnboardingTemplates))).Methods(http.MethodGet)
	r.Handle("/onboarding/apply-template", auth(admin(http.HandlerFunc(h.ApplyOnboardingTemplate)))).Methods(http.MethodPost)
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

func (h *Handler) UpdateAccountName(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "workspace name is required")
		return
	}
	if err := h.svc.UpdateAccountName(r.Context(), accountID, actorID, name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteAccount permanently removes the current admin's account and all tenant data.
// The database foreign keys cascade through tenant-owned records.
func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)

	var body struct {
		Confirmation string `json:"confirmation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	account, err := h.svc.GetAccount(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if body.Confirmation != account.Name {
		writeError(w, http.StatusBadRequest, "workspace name confirmation does not match")
		return
	}
	if err := h.svc.DeleteAccount(r.Context(), accountID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
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

func (h *Handler) PatchSettings(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	var settings map[string]any
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(settings) == 0 {
		writeError(w, http.StatusBadRequest, "at least one setting is required")
		return
	}
	if err := h.svc.MergeAccountSettings(r.Context(), accountID, actorID, settings); err != nil {
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
	config, err := mergeAIProviderConfig(body.Config, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, "AI provider config must be valid JSON")
		return
	}
	if !aiProviderConfigHasKey(config) {
		existing, err := h.svc.GetAIProviderConfig(r.Context(), accountID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		config, err = mergeAIProviderConfig(body.Config, existing)
		if err != nil {
			writeError(w, http.StatusBadRequest, "AI provider config must be valid JSON")
			return
		}
		if !aiProviderConfigHasKey(config) {
			writeError(w, http.StatusBadRequest, "AI provider API key is required")
			return
		}
	}
	if err := h.svc.UpdateAIProviderConfig(r.Context(), accountID, actorID, config); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) GetAIConfigStatus(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	config, err := h.svc.GetAIProviderConfig(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publicAIProviderConfig(config))
}

func mergeAIProviderConfig(nextJSON, existingJSON string) (string, error) {
	var next map[string]any
	if err := json.Unmarshal([]byte(nextJSON), &next); err != nil {
		return "", err
	}
	mergedConfig := make(map[string]any)
	if strings.TrimSpace(existingJSON) != "" {
		if err := json.Unmarshal([]byte(existingJSON), &mergedConfig); err != nil {
			return "", err
		}
	}
	for key, value := range next {
		if key == "api_key" {
			if apiKey, _ := value.(string); strings.TrimSpace(apiKey) == "" {
				continue
			}
		}
		mergedConfig[key] = value
	}
	merged, err := json.Marshal(mergedConfig)
	return string(merged), err
}

func aiProviderConfigHasKey(configJSON string) bool {
	var config struct {
		APIKey string `json:"api_key"`
	}
	return json.Unmarshal([]byte(configJSON), &config) == nil && strings.TrimSpace(config.APIKey) != ""
}

func publicAIProviderConfig(configJSON string) map[string]any {
	response := map[string]any{"configured": false}
	if configJSON == "" {
		return response
	}
	var config struct {
		APIKey          string `json:"api_key"`
		BaseURL         string `json:"base_url"`
		CompletionModel string `json:"completion_model"`
		EmbeddingModel  string `json:"embedding_model"`
	}
	if json.Unmarshal([]byte(configJSON), &config) != nil {
		return response
	}
	response["configured"] = strings.TrimSpace(config.APIKey) != ""
	if response["configured"] != true {
		return response
	}
	response["base_url"] = config.BaseURL
	response["completion_model"] = config.CompletionModel
	response["embedding_model"] = config.EmbeddingModel
	return response
}

func (h *Handler) UpdateProductMode(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	var body struct {
		ProductMode string `json:"product_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.ProductMode != "full_workspace" && body.ProductMode != "chatbot_only" {
		writeError(w, http.StatusBadRequest, "invalid product mode")
		return
	}
	if err := h.svc.UpdateProductMode(r.Context(), accountID, actorID, body.ProductMode); err != nil {
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

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	var req service.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	result, err := h.svc.CreateUser(r.Context(), accountID, actorID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	vars := mux.Vars(r)
	targetID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	if err := h.svc.DeleteUser(r.Context(), accountID, actorID, targetID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) ResetUserPassword(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	vars := mux.Vars(r)
	targetID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if err := h.svc.ResetUserPassword(r.Context(), accountID, actorID, targetID, req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "password_reset"})
}

func (h *Handler) SetAccountSlug(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	var req struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if err := h.svc.SetAccountSlug(r.Context(), accountID, actorID, req.Slug); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"slug": req.Slug, "status": "updated"})
}

func (h *Handler) GetAccountSlug(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	slug, err := h.svc.GetAccountSlug(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"slug": slug})
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
// Onboarding handlers
// -------------------------------------------------------------------------

func (h *Handler) GetOnboardingStatus(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "no account in session")
		return
	}
	state, err := h.svc.GetOnboardingStatus(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *Handler) PatchOnboardingStatus(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "no account in session")
		return
	}

	var body struct {
		Step   string `json:"step"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Action != "complete" && body.Action != "skip" {
		writeError(w, http.StatusBadRequest, `action must be "complete" or "skip"`)
		return
	}
	if body.Step == "" {
		writeError(w, http.StatusBadRequest, "step is required")
		return
	}

	if err := h.svc.PatchOnboardingStatus(r.Context(), accountID, body.Step, body.Action); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) GetOnboardingTemplates(w http.ResponseWriter, r *http.Request) {
	templates := h.svc.GetOnboardingTemplates()
	writeJSON(w, http.StatusOK, templates)
}

func (h *Handler) ApplyOnboardingTemplate(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "no account in session")
		return
	}
	actorID, _ := middleware.UserIDFromContext(r)

	var body struct {
		BusinessType string `json:"business_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.BusinessType == "" {
		writeError(w, http.StatusBadRequest, "business_type is required")
		return
	}

	// Fetch account to read product_mode
	account, err := h.svc.GetAccount(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := h.svc.ApplyOnboardingTemplate(r.Context(), accountID, actorID, body.BusinessType, account.ProductMode); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "applied"})
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
