package handler

import (
	"net/http"
)

func (h *Handler) CreateLead(w http.ResponseWriter, r *http.Request) {
	accountID, userID, userRole := callerFrom(r)

	convoID, ok := pathUUID(w, r, "id", "invalid conversation ID")
	if !ok {
		return
	}

	lead, err := h.svc.CreateLead(r.Context(), accountID, userID, convoID, userRole)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, lead)
}

func (h *Handler) UpdateLeadState(w http.ResponseWriter, r *http.Request) {
	accountID, userID, userRole := callerFrom(r)

	leadID, ok := pathUUID(w, r, "id", "invalid lead ID")
	if !ok {
		return
	}

	var body struct {
		StateKey string `json:"state_key"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	lead, err := h.svc.UpdateLeadState(r.Context(), accountID, userID, leadID, userRole, body.StateKey)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, lead)
}

func (h *Handler) UpdateLeadTags(w http.ResponseWriter, r *http.Request) {
	accountID, userID, userRole := callerFrom(r)

	leadID, ok := pathUUID(w, r, "id", "invalid lead ID")
	if !ok {
		return
	}

	var body struct {
		Tags []string `json:"tags"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	lead, err := h.svc.UpdateLeadTags(r.Context(), accountID, userID, leadID, userRole, body.Tags)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, lead)
}

func (h *Handler) CreateLeadNote(w http.ResponseWriter, r *http.Request) {
	accountID, userID, userRole := callerFrom(r)

	leadID, ok := pathUUID(w, r, "id", "invalid lead ID")
	if !ok {
		return
	}

	var body struct {
		Body string `json:"body"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	note, err := h.svc.CreateLeadNote(r.Context(), accountID, userID, leadID, userRole, body.Body)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, note)
}

func (h *Handler) ListLeadNotes(w http.ResponseWriter, r *http.Request) {
	accountID, userID, userRole := callerFrom(r)

	leadID, ok := pathUUID(w, r, "id", "invalid lead ID")
	if !ok {
		return
	}

	notes, err := h.svc.ListLeadNotes(r.Context(), accountID, userID, leadID, userRole)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, notes)
}

func (h *Handler) ListLeadHistory(w http.ResponseWriter, r *http.Request) {
	accountID, userID, userRole := callerFrom(r)

	leadID, ok := pathUUID(w, r, "id", "invalid lead ID")
	if !ok {
		return
	}

	history, err := h.svc.ListLeadHistory(r.Context(), accountID, userID, leadID, userRole)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, history)
}
