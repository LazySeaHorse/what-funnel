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

	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
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
	r.Handle("/workspace/account/ai-config/test", auth(admin(http.HandlerFunc(h.TestAIConfig)))).Methods(http.MethodPost)
	r.Handle("/workspace/account/ai-config/status", auth(http.HandlerFunc(h.GetAIConfigStatus))).Methods(http.MethodGet)
	r.Handle("/workspace/account/product-mode", auth(admin(http.HandlerFunc(h.UpdateProductMode)))).Methods(http.MethodPatch)
	r.Handle("/account/product-mode", auth(admin(http.HandlerFunc(h.UpdateProductMode)))).Methods(http.MethodPatch)

	// Users
	r.Handle("/workspace/users", auth(admin(http.HandlerFunc(h.ListUsers)))).Methods(http.MethodGet)
	r.Handle("/workspace/users", auth(admin(http.HandlerFunc(h.CreateUser)))).Methods(http.MethodPost)
	r.Handle("/workspace/users/{id}", auth(admin(http.HandlerFunc(h.DeleteUser)))).Methods(http.MethodDelete)
	r.Handle("/workspace/users/{id}/password", auth(admin(http.HandlerFunc(h.ResetUserPassword)))).Methods(http.MethodPut)
	r.Handle("/workspace/users/{id}/role", auth(admin(http.HandlerFunc(h.ChangeUserRole)))).Methods(http.MethodPut)
	r.Handle("/workspace/users/me/reply-mode", auth(http.HandlerFunc(h.GetMyReplyMode))).Methods(http.MethodGet)
	r.Handle("/workspace/users/me/reply-mode", auth(http.HandlerFunc(h.UpdateMyReplyMode))).Methods(http.MethodPatch)
	r.Handle("/users/me/reply-mode", auth(http.HandlerFunc(h.GetMyReplyMode))).Methods(http.MethodGet)
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

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
