package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
)

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
