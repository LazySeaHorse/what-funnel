package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

func (h *Handler) ListChannels(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}

	channels, err := h.svc.ListChannels(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if channels == nil {
		channels = []*types.Channel{}
	}

	writeJSON(w, http.StatusOK, channels)
}

func (h *Handler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}

	var body struct {
		Type              string          `json:"type"`
		BridgeIdentity    string          `json:"bridge_identity"`
		BridgeCredentials json.RawMessage `json:"bridge_credentials"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if body.Type == "" {
		writeError(w, http.StatusBadRequest, "missing channel type")
		return
	}

	var bridgeIdentity *string
	if body.BridgeIdentity != "" {
		bridgeIdentity = &body.BridgeIdentity
	}

	ch, err := h.svc.CreateChannel(r.Context(), accountID, body.Type, bridgeIdentity, []byte(body.BridgeCredentials))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, ch)
}

func (h *Handler) GetChannel(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}

	vars := mux.Vars(r)
	chID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel ID")
		return
	}

	status, err := h.svc.GetChannelStatus(r.Context(), accountID, chID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing account")
		return
	}
	actorID, ok := middleware.UserIDFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user")
		return
	}

	channelID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel ID")
		return
	}

	if err := h.svc.DeleteChannel(r.Context(), accountID, actorID, channelID); err != nil {
		if errors.Is(err, service.ErrChannelNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
