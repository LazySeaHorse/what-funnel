package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/packages/go-common/webhooks"
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
	r.Handle("/conversations/{id}/read", auth(http.HandlerFunc(h.ReadConversation))).Methods(http.MethodPost)
	r.Handle("/conversations/{id}/close", auth(http.HandlerFunc(h.CloseConversation))).Methods(http.MethodPost)

	// Dev/test simulation endpoints (admin only)
	r.Handle("/simulate-inbound", auth(admin(http.HandlerFunc(h.SimulateInbound)))).Methods(http.MethodPost)
	r.Handle("/simulate/channels", auth(admin(http.HandlerFunc(h.ListChannelsForSimulator)))).Methods(http.MethodGet)

	// Platform Webhooks (Native external inbound messages from Telegram, WhatsApp, Meta)
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

func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}

	vars := mux.Vars(r)
	convoID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid conversation ID")
		return
	}

	var body struct {
		ContentType    string `json:"content_type"`
		Text           string `json:"text"`
		MediaURL       string `json:"media_url"`
		SenderType     string `json:"sender_type"`
		SenderUserID   string `json:"sender_user_id"`
		AIReplyDraftID string `json:"ai_reply_draft_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if body.ContentType == "" {
		body.ContentType = "text"
	}

	var senderUserID *uuid.UUID
	if body.SenderUserID != "" {
		uid, err := uuid.Parse(body.SenderUserID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid sender_user_id")
			return
		}
		senderUserID = &uid
	}

	var aiReplyDraftID *uuid.UUID
	if body.AIReplyDraftID != "" {
		draftID, err := uuid.Parse(body.AIReplyDraftID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid ai_reply_draft_id")
			return
		}
		aiReplyDraftID = &draftID
	}

	msg, err := h.svc.SendMessage(r.Context(), accountID, convoID, body.SenderType, senderUserID, body.ContentType, body.Text, body.MediaURL, aiReplyDraftID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, msg)
}

func (h *Handler) GetReplyDraft(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	userID, _ := middleware.UserIDFromContext(r)
	role, _ := middleware.RoleFromContext(r)

	conversationID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid conversation ID")
		return
	}
	draft, err := h.svc.GetPendingReplyDraft(r.Context(), accountID, userID, conversationID, role)
	if err != nil {
		if err.Error() == "conversation not found" {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"draft": draft})
}

func (h *Handler) DismissReplyDraft(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	userID, _ := middleware.UserIDFromContext(r)
	role, _ := middleware.RoleFromContext(r)
	vars := mux.Vars(r)

	conversationID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid conversation ID")
		return
	}
	draftID, err := uuid.Parse(vars["draft_id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid reply draft ID")
		return
	}
	if err := h.svc.DismissReplyDraft(r.Context(), accountID, userID, conversationID, draftID, role); err != nil {
		if err.Error() == "conversation not found" || err.Error() == "reply draft not found" {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "dismissed"})
}

func (h *Handler) CloseConversation(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}
	userID, _ := middleware.UserIDFromContext(r)
	role, _ := middleware.RoleFromContext(r)

	vars := mux.Vars(r)
	convoID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid conversation ID")
		return
	}

	if err := h.svc.CloseConversation(r.Context(), accountID, userID, convoID, role); err != nil {
		if err.Error() == "conversation not found" {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "closed"})
}

func (h *Handler) ListChannels(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}

	channels, err := h.svc.ListChannels(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if channels == nil {
		channels = []*types.Channel{}
	}

	writeJSON(w, http.StatusOK, channels)
}

func (h *Handler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}

	var body struct {
		Type              string          `json:"type"`
		BridgeIdentity    string          `json:"bridge_identity"`
		BridgeCredentials json.RawMessage `json:"bridge_credentials"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if body.Type == "" {
		writeError(w, http.StatusBadRequest, "missing channel type")
		return
	}

	var bridgeIdentity *string
	if body.BridgeIdentity != "" {
		bridgeIdentity = &body.BridgeIdentity
	}

	ch, err := h.svc.CreateChannel(r.Context(), accountID, body.Type, bridgeIdentity, []byte(body.BridgeCredentials))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, ch)
}

func (h *Handler) GetChannel(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}

	vars := mux.Vars(r)
	chID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel ID")
		return
	}

	status, err := h.svc.GetChannelStatus(r.Context(), accountID, chID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) DisconnectChannel(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}

	vars := mux.Vars(r)
	chID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel ID")
		return
	}

	if err := h.svc.DisconnectChannel(r.Context(), accountID, chID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

func (h *Handler) ListBridgeConnections(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}
	connections, err := h.svc.ListBridgeConnections(r.Context(), accountID, r.URL.Query().Get("refresh") == "true")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if connections == nil {
		connections = []*types.BridgeConnection{}
	}
	writeJSON(w, http.StatusOK, connections)
}

func (h *Handler) StartBridgeConnection(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}
	var body struct {
		Platform string `json:"platform"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	connection, err := h.svc.StartBridgeConnection(r.Context(), accountID, body.Platform)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, connection)
}

func (h *Handler) GetBridgeConnection(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}
	channelID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel ID")
		return
	}
	connection, err := h.svc.GetBridgeConnection(r.Context(), accountID, channelID, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, connection)
}

func (h *Handler) GetBridgeQRCode(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}
	channelID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel ID")
		return
	}
	image, contentType, err := h.svc.BridgeQRCode(r.Context(), accountID, channelID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(image)
}

func (h *Handler) SubmitBridgeSession(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}
	channelID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel ID")
		return
	}
	var body struct {
		Session string `json:"session"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	connection, err := h.svc.SubmitBridgeSecret(r.Context(), accountID, channelID, body.Session)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, connection)
}

func (h *Handler) SubmitBridgeCode(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}
	channelID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel ID")
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	connection, err := h.svc.SubmitBridgeCode(r.Context(), accountID, channelID, body.Code)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, connection)
}

// Helpers
// SimulateInbound accepts a mock inbound event payload and publishes it to the
// Redis inbound stream, simulating a message from a 3rd-party platform.
func (h *Handler) SimulateInbound(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}

	var body struct {
		ChannelID         string `json:"channel_id"`
		SenderExternalID  string `json:"sender_external_id"`
		SenderDisplayName string `json:"sender_display_name"`
		SenderAvatarURL   string `json:"sender_avatar_url"`
		ContentType       string `json:"content_type"`
		Text              string `json:"text"`
		MediaURL          string `json:"media_url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if body.ChannelID == "" {
		writeError(w, http.StatusBadRequest, "channel_id is required")
		return
	}
	if body.SenderExternalID == "" {
		writeError(w, http.StatusBadRequest, "sender_external_id is required")
		return
	}
	if body.ContentType == "" {
		body.ContentType = "text"
	}

	if err := h.svc.SimulateInbound(r.Context(), accountID, body.ChannelID, body.SenderExternalID, body.SenderDisplayName, body.SenderAvatarURL, body.ContentType, body.Text, body.MediaURL); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "published"})
}

// ListChannelsForSimulator returns the list of channels for the current account
// so the dev widget can target a specific channel when simulating inbound messages.
func (h *Handler) ListChannelsForSimulator(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}

	channels, err := h.svc.ListChannels(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if channels == nil {
		channels = []*types.Channel{}
	}

	writeJSON(w, http.StatusOK, channels)
}

// HandlePlatformWebhook processes native incoming webhooks from platforms (Telegram, WhatsApp, Instagram, Messenger).
func (h *Handler) HandlePlatformWebhook(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	platform := strings.ToLower(vars["platform"])
	channelIDStr := vars["channel_id"]
	if channelIDStr == "" {
		channelIDStr = r.URL.Query().Get("channel_id")
	}
	if channelIDStr == "" {
		channelIDStr = r.Header.Get("X-Channel-ID")
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	// If channelID is not provided directly, try resolving from context if authenticated
	if channelIDStr == "" {
		if accountID, ok := middleware.AccountIDFromContext(r); ok {
			chs, err := h.svc.ListChannels(r.Context(), accountID)
			if err == nil {
				for _, ch := range chs {
					if ch.Type == platform || ch.Type == "matrix_"+platform {
						channelIDStr = ch.ID.String()
						break
					}
				}
			}
		}
	}

	if channelIDStr == "" {
		writeError(w, http.StatusBadRequest, "missing channel_id or could not resolve channel for platform")
		return
	}

	var events []types.InboundEvent
	switch platform {
	case "telegram":
		events, err = webhooks.ParseTelegramUpdate(channelIDStr, bodyBytes)
	case "whatsapp":
		events, err = webhooks.ParseWhatsAppWebhook(channelIDStr, bodyBytes)
	case "instagram", "messenger":
		events, err = webhooks.ParseMetaWebhook(channelIDStr, bodyBytes)
	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported platform: %s", platform))
		return
	}

	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse %s payload: %v", platform, err))
		return
	}

	for _, event := range events {
		if err := h.svc.PublishInbound(r.Context(), event); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to process inbound event: %v", err))
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"processed_events": len(events),
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (h *Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
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
	userRole, ok := middleware.RoleFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing role")
		return
	}

	filter := r.URL.Query().Get("filter")
	if filter == "" {
		filter = "all"
	}
	if filter != "all" && filter != "mine" && filter != "unassigned" {
		writeError(w, http.StatusBadRequest, "invalid filter")
		return
	}

	leadState := r.URL.Query().Get("state")
	conversations, err := h.svc.ListConversations(r.Context(), accountID, userID, userRole, filter, leadState)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if conversations == nil {
		conversations = []*types.ConversationListItem{}
	}

	writeJSON(w, http.StatusOK, conversations)
}

func (h *Handler) GetConversation(w http.ResponseWriter, r *http.Request) {
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
	userRole, ok := middleware.RoleFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing role")
		return
	}

	vars := mux.Vars(r)
	convoID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid conversation ID")
		return
	}

	convo, err := h.svc.GetConversation(r.Context(), accountID, userID, convoID, userRole)
	if err != nil {
		if err.Error() == "conversation not found" {
			writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, convo)
}

func (h *Handler) GetConversationMessages(w http.ResponseWriter, r *http.Request) {
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
	userRole, ok := middleware.RoleFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing role")
		return
	}

	vars := mux.Vars(r)
	convoID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid conversation ID")
		return
	}

	beforeCursor := r.URL.Query().Get("before")
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	messages, nextCursor, err := h.svc.GetConversationMessages(r.Context(), accountID, userID, convoID, userRole, beforeCursor, limit)
	if err != nil {
		if err.Error() == "conversation not found" {
			writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if messages == nil {
		messages = []*types.Message{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"messages":    messages,
		"next_cursor": nextCursor,
	})
}

func (h *Handler) AssignConversation(w http.ResponseWriter, r *http.Request) {
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

	vars := mux.Vars(r)
	convoID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid conversation ID")
		return
	}

	var body struct {
		UserIDs []string `json:"user_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	assignedUserIDs := []uuid.UUID{}
	for _, idStr := range body.UserIDs {
		u, err := uuid.Parse(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user ID: "+idStr)
			return
		}
		assignedUserIDs = append(assignedUserIDs, u)
	}

	if err := h.svc.AssignConversation(r.Context(), accountID, convoID, assignedUserIDs, userID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
}

func (h *Handler) ReadConversation(w http.ResponseWriter, r *http.Request) {
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
	userRole, ok := middleware.RoleFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing role")
		return
	}

	vars := mux.Vars(r)
	convoID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid conversation ID")
		return
	}

	// Verify visibility first
	_, err = h.svc.GetConversation(r.Context(), accountID, userID, convoID, userRole)
	if err != nil {
		if err.Error() == "conversation not found" {
			writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := h.svc.ReadConversation(r.Context(), accountID, userID, convoID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "read"})
}

func (h *Handler) CreateLead(w http.ResponseWriter, r *http.Request) {
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
	userRole, ok := middleware.RoleFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing role")
		return
	}

	vars := mux.Vars(r)
	convoID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid conversation ID")
		return
	}

	lead, err := h.svc.CreateLead(r.Context(), accountID, userID, convoID, userRole)
	if err != nil {
		if err.Error() == "conversation not found" {
			writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, lead)
}

func (h *Handler) UpdateLeadState(w http.ResponseWriter, r *http.Request) {
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
	userRole, ok := middleware.RoleFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing role")
		return
	}

	vars := mux.Vars(r)
	leadID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid lead ID")
		return
	}

	var body struct {
		StateKey string `json:"state_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	lead, err := h.svc.UpdateLeadState(r.Context(), accountID, userID, leadID, userRole, body.StateKey)
	if err != nil {
		if err.Error() == "lead not found" {
			writeError(w, http.StatusNotFound, "lead not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, lead)
}

func (h *Handler) UpdateLeadTags(w http.ResponseWriter, r *http.Request) {
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
	userRole, ok := middleware.RoleFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing role")
		return
	}

	vars := mux.Vars(r)
	leadID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid lead ID")
		return
	}

	var body struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	lead, err := h.svc.UpdateLeadTags(r.Context(), accountID, userID, leadID, userRole, body.Tags)
	if err != nil {
		if err.Error() == "lead not found" {
			writeError(w, http.StatusNotFound, "lead not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, lead)
}

func (h *Handler) CreateLeadNote(w http.ResponseWriter, r *http.Request) {
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
	userRole, ok := middleware.RoleFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing role")
		return
	}

	vars := mux.Vars(r)
	leadID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid lead ID")
		return
	}

	var body struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	note, err := h.svc.CreateLeadNote(r.Context(), accountID, userID, leadID, userRole, body.Body)
	if err != nil {
		if err.Error() == "lead not found" {
			writeError(w, http.StatusNotFound, "lead not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, note)
}

func (h *Handler) ListLeadNotes(w http.ResponseWriter, r *http.Request) {
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
	userRole, ok := middleware.RoleFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing role")
		return
	}

	vars := mux.Vars(r)
	leadID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid lead ID")
		return
	}

	notes, err := h.svc.ListLeadNotes(r.Context(), accountID, userID, leadID, userRole)
	if err != nil {
		if err.Error() == "lead not found" {
			writeError(w, http.StatusNotFound, "lead not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, notes)
}

func (h *Handler) ListLeadHistory(w http.ResponseWriter, r *http.Request) {
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
	userRole, ok := middleware.RoleFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing role")
		return
	}

	vars := mux.Vars(r)
	leadID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid lead ID")
		return
	}

	history, err := h.svc.ListLeadHistory(r.Context(), accountID, userID, leadID, userRole)
	if err != nil {
		if err.Error() == "lead not found" {
			writeError(w, http.StatusNotFound, "lead not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, history)
}
