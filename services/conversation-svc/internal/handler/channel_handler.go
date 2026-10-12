package handler

import (
	"net/http"
)

func (h *Handler) ListChannels(w http.ResponseWriter, request *http.Request) {
	accountID, _, _ := callerFrom(request)
	channels, err := h.svc.ListChannels(request.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not list channels.")
		return
	}
	writeJSON(w, http.StatusOK, channels)
}
