package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/adapterclient"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
	"rsc.io/qr"
)

func (h *Handler) ListProviderConnections(w http.ResponseWriter, request *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(request)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Missing account.")
		return
	}
	connections, err := h.svc.ListProviderConnections(request.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not list channel connections.")
		return
	}
	writeJSON(w, http.StatusOK, connections)
}

func (h *Handler) GetProviderConnectionQR(w http.ResponseWriter, request *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(request)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Missing account.")
		return
	}
	channelID, err := uuid.Parse(mux.Vars(request)["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid channel ID.")
		return
	}
	connection, err := h.svc.GetProviderConnection(request.Context(), accountID, channelID, true)
	if err != nil || connection.QRData == "" {
		writeError(w, http.StatusNotFound, "Pairing code is not available.")
		return
	}
	code, err := qr.Encode(connection.QRData, qr.M)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not render pairing code.")
		return
	}
	code.Scale = 6
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(code.PNG())
}

func (h *Handler) StartProviderConnection(w http.ResponseWriter, request *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(request)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Missing account.")
		return
	}
	defer request.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 1024*1024))
	decoder.DisallowUnknownFields()
	var body struct {
		Provider   messaging.Provider `json:"provider"`
		Label      string             `json:"label"`
		Credential string             `json:"credential"`
	}
	if err := decoder.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body.")
		return
	}
	connection, err := h.svc.StartProviderConnection(request.Context(), accountID, body.Provider, body.Label, body.Credential)
	if err != nil {
		if errors.Is(err, service.ErrProviderConnectionLabelExists) {
			writeError(w, http.StatusConflict, "A channel connection with that label already exists.")
			return
		}
		if connection != nil {
			writeConnectionFailure(w, request, connection, err)
			return
		}
		writeServiceError(w, request, err)
		return
	}
	writeJSON(w, http.StatusCreated, connection)
}

// writeConnectionFailure reports an adapter failure. An adapter that rejects
// the input itself (for example an invalid bot token) yields a 422 carrying
// the adapter's user-safe message; every other failure is a 502. The durable
// connection record (which carries a safe, user-facing failure detail and lets
// the UI retry or unlink it) is included; adapter internals stay out of the
// response and are logged instead.
func writeConnectionFailure(w http.ResponseWriter, request *http.Request, connection *types.ProviderConnection, cause error) {
	slog.WarnContext(request.Context(), "provider connection failed", "channel_id", connection.ChannelID, "provider", connection.Provider, "error", cause)
	status := http.StatusBadGateway
	message := connection.Detail
	if message == "" {
		message = "Could not connect this provider account."
	}
	var adapterErr *adapterclient.Error
	if errors.As(cause, &adapterErr) && adapterErr.IsInvalidInput() {
		status = http.StatusUnprocessableEntity
		if adapterErr.Message != "" {
			message = adapterErr.Message
		}
	}
	writeJSON(w, status, map[string]any{"error": message, "connection": connection})
}

func (h *Handler) RetryProviderConnection(w http.ResponseWriter, request *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(request)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Missing account.")
		return
	}
	channelID, err := uuid.Parse(mux.Vars(request)["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid channel ID.")
		return
	}
	defer request.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 1024*1024))
	decoder.DisallowUnknownFields()
	var body struct {
		Credential string `json:"credential"`
	}
	if err := decoder.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body.")
		return
	}
	connection, err := h.svc.RetryProviderConnection(request.Context(), accountID, channelID, body.Credential)
	if errors.Is(err, service.ErrChannelNotFound) {
		writeError(w, http.StatusNotFound, "Channel connection not found.")
		return
	}
	if err != nil {
		if connection != nil {
			writeConnectionFailure(w, request, connection, err)
			return
		}
		writeServiceError(w, request, err)
		return
	}
	writeJSON(w, http.StatusOK, connection)
}

func (h *Handler) GetProviderConnection(w http.ResponseWriter, request *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(request)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Missing account.")
		return
	}
	channelID, err := uuid.Parse(mux.Vars(request)["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid channel ID.")
		return
	}
	connection, err := h.svc.GetProviderConnection(request.Context(), accountID, channelID, true)
	if errors.Is(err, service.ErrChannelNotFound) {
		writeError(w, http.StatusNotFound, "Channel connection not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not read channel connection.")
		return
	}
	writeJSON(w, http.StatusOK, connection)
}

func (h *Handler) DeleteProviderConnection(w http.ResponseWriter, request *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(request)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Missing account.")
		return
	}
	actorID, ok := middleware.UserIDFromContext(request)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Missing user.")
		return
	}
	channelID, err := uuid.Parse(mux.Vars(request)["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid channel ID.")
		return
	}
	if err := h.svc.DeleteProviderConnection(request.Context(), accountID, actorID, channelID); err != nil {
		if errors.Is(err, service.ErrChannelNotFound) {
			writeError(w, http.StatusNotFound, "Channel connection not found.")
			return
		}
		writeError(w, http.StatusBadGateway, "Could not unlink channel connection.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
