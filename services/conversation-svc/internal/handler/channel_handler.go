package handler

import (
	"net/http"

	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
)

func (h *Handler) ListChannels(w http.ResponseWriter, request *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(request)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Missing account.")
		return
	}
	channels, err := h.svc.ListChannels(request.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not list channels.")
		return
	}
	writeJSON(w, http.StatusOK, channels)
}
