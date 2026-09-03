package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

type SessionStore interface {
	GetUserID(r *http.Request) (uuid.UUID, bool)
	GetAccountID(r *http.Request) (uuid.UUID, bool)
	GetRole(r *http.Request) (string, bool)
}

type Handler struct {
	svc  *service.Service
	sess SessionStore
	mw   *middleware.SessionMiddleware
}

func New(svc *service.Service, sess SessionStore) *Handler {
	mw := middleware.NewSessionMiddlewareWithDB(sess, svc.Pool())
	return &Handler{svc: svc, sess: sess, mw: mw}
}

func (h *Handler) RegisterRoutes(r *mux.Router) {
	auth := h.mw.RequireAuthenticated
	admin := middleware.RequireAdmin
	fullWorkspace := h.mw.RequireProductMode("full_workspace")

	// Outbound Send
	r.Handle("/internal/conversations/{id}/send", auth(http.HandlerFunc(h.SendMessage))).Methods(http.MethodPost)

	// Channel lifecycle
	r.Handle("/channels", auth(admin(http.HandlerFunc(h.ListChannels)))).Methods(http.MethodGet)
	r.Handle("/channels", auth(admin(http.HandlerFunc(h.CreateChannel)))).Methods(http.MethodPost)
	r.Handle("/channels/{id}", auth(admin(http.HandlerFunc(h.GetChannel)))).Methods(http.MethodGet)
	r.Handle("/channels/{id}/disconnect", auth(admin(http.HandlerFunc(h.DisconnectChannel)))).Methods(http.MethodPost)
	// Guided mautrix bridge setup. These are intentionally separate from the
	// low-level channel CRUD endpoints so a pending channel cannot be mistaken
	// for a connected account.
	r.Handle("/bridge-connections", auth(admin(http.HandlerFunc(h.ListBridgeConnections)))).Methods(http.MethodGet)
	r.Handle("/bridge-connections", auth(admin(http.HandlerFunc(h.StartBridgeConnection)))).Methods(http.MethodPost)
	r.Handle("/bridge-connections/{id}", auth(admin(http.HandlerFunc(h.GetBridgeConnection)))).Methods(http.MethodGet)
	r.Handle("/bridge-connections/{id}/qr", auth(admin(http.HandlerFunc(h.GetBridgeQRCode)))).Methods(http.MethodGet)
	r.Handle("/bridge-connections/{id}/session", auth(admin(http.HandlerFunc(h.SubmitBridgeSession)))).Methods(http.MethodPost)
	r.Handle("/bridge-connections/{id}/code", auth(admin(http.HandlerFunc(h.SubmitBridgeCode)))).Methods(http.MethodPost)

	// Conversations listing & details
	r.Handle("/conversations", auth(http.HandlerFunc(h.ListConversations))).Methods(http.MethodGet)
	r.Handle("/conversations/{id}", auth(http.HandlerFunc(h.GetConversation))).Methods(http.MethodGet)
	r.Handle("/conversations/{id}/messages", auth(http.HandlerFunc(h.GetConversationMessages))).Methods(http.MethodGet)
	r.Handle("/conversations/{id}/reply-draft", auth(http.HandlerFunc(h.GetReplyDraft))).Methods(http.MethodGet)
	r.Handle("/conversations/{id}/reply-draft/{draft_id}/dismiss", auth(http.HandlerFunc(h.DismissReplyDraft))).Methods(http.MethodPost)
	r.Handle("/conversations/{id}/assign", auth(admin(http.HandlerFunc(h.AssignConversation)))).Methods(http.MethodPatch)
	r.Handle("/conversations/{id}/ai-control", auth(http.HandlerFunc(h.UpdateConversationAIControl))).Methods(http.MethodPatch)
	r.Handle("/conversations/{id}/read", auth(http.HandlerFunc(h.ReadConversation))).Methods(http.MethodPost)
	r.Handle("/conversations/{id}/close", auth(http.HandlerFunc(h.CloseConversation))).Methods(http.MethodPost)

	// Dev/test simulation endpoints (admin only)
	r.Handle("/simulate-inbound", auth(admin(http.HandlerFunc(h.SimulateInbound)))).Methods(http.MethodPost)
	r.Handle("/simulate/channels", auth(admin(http.HandlerFunc(h.ListChannelsForSimulator)))).Methods(http.MethodGet)

	// Platform Webhooks (Native external inbound messages from Telegram, WhatsApp, Meta)
	r.HandleFunc("/webhooks/{platform}", h.HandlePlatformWebhookVerification).Methods(http.MethodGet)
	r.HandleFunc("/webhooks/{platform}/{channel_id}", h.HandlePlatformWebhookVerification).Methods(http.MethodGet)
	r.HandleFunc("/webhooks/{platform}", h.HandlePlatformWebhook).Methods(http.MethodPost)
	r.HandleFunc("/webhooks/{platform}/{channel_id}", h.HandlePlatformWebhook).Methods(http.MethodPost)

	// Lead management
	r.Handle("/conversations/{id}/lead", auth(fullWorkspace(http.HandlerFunc(h.CreateLead)))).Methods(http.MethodPost)
	r.Handle("/leads/{id}/state", auth(fullWorkspace(http.HandlerFunc(h.UpdateLeadState)))).Methods(http.MethodPatch)
	r.Handle("/leads/{id}/tags", auth(fullWorkspace(http.HandlerFunc(h.UpdateLeadTags)))).Methods(http.MethodPatch)
	r.Handle("/leads/{id}/notes", auth(fullWorkspace(http.HandlerFunc(h.CreateLeadNote)))).Methods(http.MethodPost)
	r.Handle("/leads/{id}/notes", auth(fullWorkspace(http.HandlerFunc(h.ListLeadNotes)))).Methods(http.MethodGet)
	r.Handle("/leads/{id}/history", auth(fullWorkspace(http.HandlerFunc(h.ListLeadHistory)))).Methods(http.MethodGet)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
