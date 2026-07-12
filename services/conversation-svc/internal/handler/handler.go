package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
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
	mw := middleware.NewSessionMiddleware(sess)
	return &Handler{svc: svc, sess: sess, mw: mw}
}

func (h *Handler) RegisterRoutes(r *mux.Router) {
	auth := h.mw.RequireAuthenticated
	admin := middleware.RequireAdmin

	// Outbound Send
	r.Handle("/internal/conversations/{id}/send", auth(http.HandlerFunc(h.SendMessage))).Methods(http.MethodPost)

	// Channel lifecycle
	r.Handle("/channels", auth(admin(http.HandlerFunc(h.ListChannels)))).Methods(http.MethodGet)
	r.Handle("/channels", auth(admin(http.HandlerFunc(h.CreateChannel)))).Methods(http.MethodPost)
	r.Handle("/channels/{id}", auth(admin(http.HandlerFunc(h.GetChannel)))).Methods(http.MethodGet)
	r.Handle("/channels/{id}/disconnect", auth(admin(http.HandlerFunc(h.DisconnectChannel)))).Methods(http.MethodPost)

	// Conversations listing & details
	r.Handle("/conversations", auth(http.HandlerFunc(h.ListConversations))).Methods(http.MethodGet)
	r.Handle("/conversations/{id}", auth(http.HandlerFunc(h.GetConversation))).Methods(http.MethodGet)
	r.Handle("/conversations/{id}/messages", auth(http.HandlerFunc(h.GetConversationMessages))).Methods(http.MethodGet)
	r.Handle("/conversations/{id}/assign", auth(admin(http.HandlerFunc(h.AssignConversation)))).Methods(http.MethodPatch)
	r.Handle("/conversations/{id}/read", auth(http.HandlerFunc(h.ReadConversation))).Methods(http.MethodPost)
	r.Handle("/conversations/{id}/close", auth(http.HandlerFunc(h.CloseConversation))).Methods(http.MethodPost)

	// Lead management
	r.Handle("/conversations/{id}/lead", auth(http.HandlerFunc(h.CreateLead))).Methods(http.MethodPost)
	r.Handle("/leads/{id}/state", auth(http.HandlerFunc(h.UpdateLeadState))).Methods(http.MethodPatch)
	r.Handle("/leads/{id}/tags", auth(http.HandlerFunc(h.UpdateLeadTags))).Methods(http.MethodPatch)
	r.Handle("/leads/{id}/notes", auth(http.HandlerFunc(h.CreateLeadNote))).Methods(http.MethodPost)
	r.Handle("/leads/{id}/notes", auth(http.HandlerFunc(h.ListLeadNotes))).Methods(http.MethodGet)
	r.Handle("/leads/{id}/history", auth(http.HandlerFunc(h.ListLeadHistory))).Methods(http.MethodGet)
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
		ContentType  string `json:"content_type"`
		Text         string `json:"text"`
		MediaURL     string `json:"media_url"`
		SenderType   string `json:"sender_type"`
		SenderUserID string `json:"sender_user_id"`
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

	msg, err := h.svc.SendMessage(r.Context(), accountID, convoID, body.SenderType, senderUserID, body.ContentType, body.Text, body.MediaURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, msg)
}

func (h *Handler) CloseConversation(w http.ResponseWriter, r *http.Request) {
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

	// Publish conversation.closed event to Redis
	event := map[string]any{
		"account_id":      accountID.String(),
		"conversation_id": convoID.String(),
		"closed_at":       time.Now().Format(time.RFC3339),
	}
	_, err = h.svc.PubSub().Publish(r.Context(), "conversation.closed", event)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to publish close event: "+err.Error())
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

// Helpers
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
