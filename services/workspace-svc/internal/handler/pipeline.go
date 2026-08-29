package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/service"
)

func (h *Handler) ListPipelines(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	pipelines, err := h.svc.ListPipelines(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pipelines)
}

func (h *Handler) UpdatePipeline(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	actorID, _ := middleware.UserIDFromContext(r)

	vars := mux.Vars(r)
	pipelineID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid pipeline id")
		return
	}

	var body struct {
		Name   string                `json:"name"`
		States []types.PipelineState `json:"states"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if err := h.svc.UpdatePipeline(r.Context(), accountID, actorID, pipelineID, service.UpdatePipelineRequest{
		Name:   body.Name,
		States: body.States,
	}); err != nil {
		if inUseErr, ok := err.(*service.ErrPipelineInUse); ok {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":    "in_use",
				"message":  inUseErr.Error(),
				"lead_ids": inUseErr.LeadIDs,
			})
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
