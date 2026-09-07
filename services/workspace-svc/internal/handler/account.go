package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/service"
)

func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "no account in session")
		return
	}
	account, err := h.svc.GetAccount(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, account)
}

func (h *Handler) UpdateAccountName(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "workspace name is required")
		return
	}
	if err := h.svc.UpdateAccountName(r.Context(), accountID, actorID, name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteAccount permanently removes the current admin's account and all tenant data.
// The database foreign keys cascade through tenant-owned records.
func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)

	var body struct {
		Confirmation string `json:"confirmation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	account, err := h.svc.GetAccount(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if body.Confirmation != account.Name {
		writeError(w, http.StatusBadRequest, "workspace name confirmation does not match")
		return
	}
	if err := h.svc.DeleteAccount(r.Context(), accountID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	var settings map[string]any
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.svc.UpdateAccountSettings(r.Context(), accountID, actorID, settings); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) PatchSettings(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	var settings map[string]any
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(settings) == 0 {
		writeError(w, http.StatusBadRequest, "at least one setting is required")
		return
	}
	if err := h.svc.MergeAccountSettings(r.Context(), accountID, actorID, settings); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) UpdateAIConfig(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	var config service.AIProviderConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.svc.UpdateAIProviderConfig(r.Context(), accountID, actorID, config); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrAIProviderNotConfigured) || strings.Contains(err.Error(), "is required") {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) TestAIConfig(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)

	var config service.AIProviderConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if strings.TrimSpace(config.APIKey) == "" {
		existing, err := h.svc.GetAIProviderConfig(r.Context(), accountID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if existing == nil {
			writeError(w, http.StatusBadRequest, "ai provider api key is required")
			return
		}
		config.APIKey = existing.APIKey
	}

	if err := h.svc.TestAIProviderConfig(r.Context(), config); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "AI provider connection verified successfully",
	})
}

func (h *Handler) GetAIConfigStatus(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	status, err := h.svc.GetAIProviderStatus(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) UpdateProductMode(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	var body struct {
		ProductMode string `json:"product_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.ProductMode != "full_workspace" && body.ProductMode != "chatbot_only" {
		writeError(w, http.StatusBadRequest, "invalid product mode")
		return
	}
	if err := h.svc.UpdateProductMode(r.Context(), accountID, actorID, body.ProductMode); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) SetAccountSlug(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	var req struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if err := h.svc.SetAccountSlug(r.Context(), accountID, actorID, req.Slug); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"slug": req.Slug, "status": "updated"})
}

func (h *Handler) GetAccountSlug(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	slug, err := h.svc.GetAccountSlug(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"slug": slug})
}
