package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

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

func (h *Handler) SetConversationAIAutoReply(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	userID, _ := middleware.UserIDFromContext(r)
	role, _ := middleware.RoleFromContext(r)
	conversationID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid conversation ID")
		return
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.svc.SetConversationAIAutoReply(r.Context(), accountID, userID, conversationID, role, body.Enabled); err != nil {
		if err.Error() == "conversation not found" {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ai_auto_reply_enabled": body.Enabled})
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
