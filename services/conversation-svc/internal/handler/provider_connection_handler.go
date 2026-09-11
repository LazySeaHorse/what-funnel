package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
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
		if connection != nil {
			// The durable connection record contains a safe, user-facing failure
			// detail and lets the UI retry or unlink it. Adapter internals stay out
			// of the HTTP response.
			writeJSON(w, http.StatusCreated, connection)
			return
		}
		writeError(w, http.StatusBadRequest, "Could not create channel connection.")
		return
	}
	writeJSON(w, http.StatusCreated, connection)
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
			writeJSON(w, http.StatusOK, connection)
			return
		}
		writeError(w, http.StatusBadGateway, "Could not reconnect channel connection.")
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
