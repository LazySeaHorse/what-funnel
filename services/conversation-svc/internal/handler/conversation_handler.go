package handler

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

const (
	defaultMessagesLimit = 20
	maxMessagesLimit     = 100
)

func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	accountID, callerUserID, callerRole := callerFrom(r)

	convoID, ok := pathUUID(w, r, "id", "invalid conversation ID")
	if !ok {
		return
	}

	// Verify conversation visibility and access permissions (SEC-04)
	if err := h.svc.CanSeeConversation(r.Context(), accountID, callerUserID, convoID, callerRole); err != nil {
		writeServiceError(w, r, err)
		return
	}

	var body struct {
		ContentType      string `json:"content_type"`
		Text             string `json:"text"`
		MediaID          string `json:"media_id"`
		ReplyToMessageID string `json:"reply_to_message_id"`
		SenderType       string `json:"sender_type"`
		SenderUserID     string `json:"sender_user_id"`
		AIReplyDraftID   string `json:"ai_reply_draft_id"`
		GenerationEpoch  *int64 `json:"generation_epoch"`
		MessagePurpose   string `json:"message_purpose"`
		IdempotencyKey   string `json:"idempotency_key"`
	}

	if !decodeJSON(w, r, &body) {
		return
	}

	if body.ContentType == "" {
		body.ContentType = "text"
	}

	senderType := types.MessageSender(body.SenderType)
	if senderType == "" {
		senderType = types.MessageSenderHuman
	}

	var senderUserID *uuid.UUID
	if senderType == types.MessageSenderHuman {
		if body.SenderUserID != "" {
			uid, err := uuid.Parse(body.SenderUserID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid sender_user_id")
				return
			}
			if callerUserID != uuid.Nil && uid != callerUserID {
				writeError(w, http.StatusBadRequest, "cannot send message as another user")
				return
			}
		}
		if callerUserID != uuid.Nil {
			senderUserID = &callerUserID
		}
	} else if senderType == types.MessageSenderAI {
		if callerRole != types.RoleManager {
			writeError(w, http.StatusForbidden, "forbidden: cannot dispatch messages as AI")
			return
		}
	} else if senderType == types.MessageSenderContact {
		writeError(w, http.StatusBadRequest, "invalid sender_type for outbound message")
		return
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
	var replyToMessageID *uuid.UUID
	if body.ReplyToMessageID != "" {
		replyID, err := uuid.Parse(body.ReplyToMessageID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid reply_to_message_id")
			return
		}
		replyToMessageID = &replyID
	}

	msg, err := h.svc.SendMessage(r.Context(), service.SendMessageParams{
		AccountID:        accountID,
		ConversationID:   convoID,
		Sender:           senderType,
		SenderUserID:     senderUserID,
		ContentType:      body.ContentType,
		Text:             body.Text,
		MediaID:          body.MediaID,
		ReplyToMessageID: replyToMessageID,
		AIReplyDraftID:   aiReplyDraftID,
		GenerationEpoch:  body.GenerationEpoch,
		Purpose:          types.MessagePurpose(body.MessagePurpose),
		IdempotencyKey:   body.IdempotencyKey,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, msg)
}

func (h *Handler) EditProviderMessage(w http.ResponseWriter, r *http.Request) {
	accountID, userID, role := callerFrom(r)
	conversationID, messageID, ok := providerMessageIDs(w, r)
	if !ok {
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := h.svc.EditProviderMessage(r.Context(), accountID, userID, role, conversationID, messageID, body.Text); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}

func (h *Handler) DeleteProviderMessage(w http.ResponseWriter, r *http.Request) {
	accountID, userID, role := callerFrom(r)
	conversationID, messageID, ok := providerMessageIDs(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteProviderMessage(r.Context(), accountID, userID, role, conversationID, messageID); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}

func (h *Handler) ChangeProviderReaction(w http.ResponseWriter, r *http.Request) {
	accountID, userID, role := callerFrom(r)
	conversationID, messageID, ok := providerMessageIDs(w, r)
	if !ok {
		return
	}
	var body struct {
		Emoji   string `json:"emoji"`
		Removed bool   `json:"removed"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := h.svc.ChangeProviderReaction(r.Context(), accountID, userID, role, conversationID, messageID, body.Emoji, body.Removed); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}

func providerMessageIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	conversationID, ok := pathUUID(w, r, "id", "invalid conversation ID")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	messageID, ok := pathUUID(w, r, "message_id", "invalid message ID")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	return conversationID, messageID, true
}

func (h *Handler) GetReplyDraft(w http.ResponseWriter, r *http.Request) {
	accountID, userID, role := callerFrom(r)

	conversationID, ok := pathUUID(w, r, "id", "invalid conversation ID")
	if !ok {
		return
	}
	draft, err := h.svc.GetPendingReplyDraft(r.Context(), accountID, userID, conversationID, role)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"draft": draft})
}

func (h *Handler) DismissReplyDraft(w http.ResponseWriter, r *http.Request) {
	accountID, userID, role := callerFrom(r)

	conversationID, ok := pathUUID(w, r, "id", "invalid conversation ID")
	if !ok {
		return
	}
	draftID, ok := pathUUID(w, r, "draft_id", "invalid reply draft ID")
	if !ok {
		return
	}
	if err := h.svc.DismissReplyDraft(r.Context(), accountID, userID, conversationID, draftID, role); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "dismissed"})
}

// GetConversationSummary serves the stored summary (or {"summary": null}).
func (h *Handler) GetConversationSummary(w http.ResponseWriter, r *http.Request) {
	accountID, userID, role := callerFrom(r)

	conversationID, ok := pathUUID(w, r, "id", "invalid conversation ID")
	if !ok {
		return
	}
	summary, err := h.svc.GetConversationSummary(r.Context(), accountID, userID, conversationID, role)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": summary})
}

// RequestConversationSummary asks ai-answer-svc to generate a summary. It
// answers 202 {"status":"queued"} once the request is on the stream, or 200
// {"status":"up_to_date"} when the stored summary already covers every
// message. The current summary (if any) is always included; the new one
// arrives as a conversation.summary_updated websocket event.
func (h *Handler) RequestConversationSummary(w http.ResponseWriter, r *http.Request) {
	accountID, userID, role := callerFrom(r)

	conversationID, ok := pathUUID(w, r, "id", "invalid conversation ID")
	if !ok {
		return
	}
	status, summary, err := h.svc.RequestConversationSummary(r.Context(), accountID, userID, conversationID, role)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	code := http.StatusAccepted
	if status == types.SummaryStatusUpToDate {
		code = http.StatusOK
	}
	writeJSON(w, code, map[string]any{"status": status, "summary": summary})
}

func (h *Handler) CloseConversation(w http.ResponseWriter, r *http.Request) {
	accountID, userID, role := callerFrom(r)

	convoID, ok := pathUUID(w, r, "id", "invalid conversation ID")
	if !ok {
		return
	}

	if err := h.svc.CloseConversation(r.Context(), accountID, userID, convoID, role); err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "closed"})
}

func (h *Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
	accountID, userID, userRole := callerFrom(r)

	filter := r.URL.Query().Get("filter")
	if filter == "" {
		filter = "all"
	}
	if filter != "all" && filter != "mine" && filter != "unassigned" {
		writeError(w, http.StatusBadRequest, "invalid filter")
		return
	}

	leadState := r.URL.Query().Get("state")

	// Unparseable values become 0; the service applies the defaults and bounds.
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	conversations, err := h.svc.ListConversations(r.Context(), accountID, userID, userRole, filter, leadState, limit, offset)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	if conversations == nil {
		conversations = []*types.ConversationListItem{}
	}

	writeJSON(w, http.StatusOK, conversations)
}

func (h *Handler) GetConversation(w http.ResponseWriter, r *http.Request) {
	accountID, userID, userRole := callerFrom(r)

	convoID, ok := pathUUID(w, r, "id", "invalid conversation ID")
	if !ok {
		return
	}

	convo, err := h.svc.GetConversation(r.Context(), accountID, userID, convoID, userRole)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, convo)
}

func (h *Handler) GetConversationMessages(w http.ResponseWriter, r *http.Request) {
	accountID, userID, userRole := callerFrom(r)

	convoID, ok := pathUUID(w, r, "id", "invalid conversation ID")
	if !ok {
		return
	}

	beforeCursor := r.URL.Query().Get("before")
	limitStr := r.URL.Query().Get("limit")
	limit := defaultMessagesLimit
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > maxMessagesLimit {
		limit = maxMessagesLimit
	}

	messages, nextCursor, err := h.svc.GetConversationMessages(r.Context(), accountID, userID, convoID, userRole, beforeCursor, limit)
	if err != nil {
		writeServiceError(w, r, err)
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
	accountID, userID, _ := callerFrom(r)

	convoID, ok := pathUUID(w, r, "id", "invalid conversation ID")
	if !ok {
		return
	}

	var body struct {
		UserIDs []string `json:"user_ids"`
	}
	if !decodeJSON(w, r, &body) {
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
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
}

func (h *Handler) UpdateConversationAIControl(w http.ResponseWriter, r *http.Request) {
	accountID, userID, role := callerFrom(r)
	conversationID, ok := pathUUID(w, r, "id", "invalid conversation ID")
	if !ok {
		return
	}
	var body struct {
		Action        string `json:"action"`
		ReplyOverride string `json:"reply_override"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	state, err := h.svc.UpdateConversationAIControl(r.Context(), accountID, userID, conversationID, role, body.Action, body.ReplyOverride)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ai_control": state})
}

func (h *Handler) ReadConversation(w http.ResponseWriter, r *http.Request) {
	accountID, userID, userRole := callerFrom(r)

	convoID, ok := pathUUID(w, r, "id", "invalid conversation ID")
	if !ok {
		return
	}

	// Verify visibility first
	if err := h.svc.CanSeeConversation(r.Context(), accountID, userID, convoID, userRole); err != nil {
		writeServiceError(w, r, err)
		return
	}

	if err := h.svc.ReadConversation(r.Context(), accountID, userID, convoID); err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "read"})
}
