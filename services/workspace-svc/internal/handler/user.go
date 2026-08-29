package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/service"
)

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

func (h *Handler) GetMyReplyMode(w http.ResponseWriter, r *http.Request) {
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

	preferences, err := h.svc.GetUserReplyMode(r.Context(), accountID, userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, preferences)
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
