package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
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

	var body struct {
		Config string `json:"config"` // raw JSON string — will be encrypted before storage
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	config, err := mergeAIProviderConfig(body.Config, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, "AI provider config must be valid JSON")
		return
	}
	if !aiProviderConfigHasKey(config) {
		existing, err := h.svc.GetAIProviderConfig(r.Context(), accountID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		config, err = mergeAIProviderConfig(body.Config, existing)
		if err != nil {
			writeError(w, http.StatusBadRequest, "AI provider config must be valid JSON")
			return
		}
		if !aiProviderConfigHasKey(config) {
			writeError(w, http.StatusBadRequest, "AI provider API key is required")
			return
		}
	}
	if err := h.svc.UpdateAIProviderConfig(r.Context(), accountID, actorID, config); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) TestAIConfig(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)

	var body struct {
		Config          string `json:"config"`
		APIKey          string `json:"api_key"`
		BaseURL         string `json:"base_url"`
		CompletionModel string `json:"completion_model"`
		EmbeddingModel  string `json:"embedding_model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	configJSON := body.Config
	if configJSON == "" && (body.APIKey != "" || body.BaseURL != "" || body.CompletionModel != "" || body.EmbeddingModel != "") {
		b, _ := json.Marshal(map[string]string{
			"api_key":          body.APIKey,
			"base_url":         body.BaseURL,
			"completion_model": body.CompletionModel,
			"embedding_model":  body.EmbeddingModel,
		})
		configJSON = string(b)
	}

	config, err := mergeAIProviderConfig(configJSON, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, "AI provider config must be valid JSON")
		return
	}

	if !aiProviderConfigHasKey(config) {
		existing, err := h.svc.GetAIProviderConfig(r.Context(), accountID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		config, err = mergeAIProviderConfig(configJSON, existing)
		if err != nil {
			writeError(w, http.StatusBadRequest, "AI provider config must be valid JSON")
			return
		}
		if !aiProviderConfigHasKey(config) {
			writeError(w, http.StatusBadRequest, "AI provider API key is required")
			return
		}
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
	config, err := h.svc.GetAIProviderConfig(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publicAIProviderConfig(config))
}

func mergeAIProviderConfig(nextJSON, existingJSON string) (string, error) {
	var next map[string]any
	if err := json.Unmarshal([]byte(nextJSON), &next); err != nil {
		return "", err
	}
	mergedConfig := make(map[string]any)
	if strings.TrimSpace(existingJSON) != "" {
		if err := json.Unmarshal([]byte(existingJSON), &mergedConfig); err != nil {
			return "", err
		}
	}
	for key, value := range next {
		if key == "api_key" {
			if apiKey, _ := value.(string); strings.TrimSpace(apiKey) == "" {
				continue
			}
		}
		mergedConfig[key] = value
	}
	merged, err := json.Marshal(mergedConfig)
	return string(merged), err
}

func aiProviderConfigHasKey(configJSON string) bool {
	var config struct {
		APIKey string `json:"api_key"`
	}
	return json.Unmarshal([]byte(configJSON), &config) == nil && strings.TrimSpace(config.APIKey) != ""
}

func publicAIProviderConfig(configJSON string) map[string]any {
	response := map[string]any{"configured": false}
	if configJSON == "" {
		return response
	}
	var config struct {
		APIKey          string `json:"api_key"`
		BaseURL         string `json:"base_url"`
		CompletionModel string `json:"completion_model"`
		EmbeddingModel  string `json:"embedding_model"`
	}
	if json.Unmarshal([]byte(configJSON), &config) != nil {
		return response
	}
	response["configured"] = strings.TrimSpace(config.APIKey) != ""
	if response["configured"] != true {
		return response
	}
	response["base_url"] = config.BaseURL
	response["completion_model"] = config.CompletionModel
	response["embedding_model"] = config.EmbeddingModel
	return response
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
