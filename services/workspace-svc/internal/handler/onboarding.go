package handler

import (
	"encoding/json"
	"net/http"

	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
)

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
