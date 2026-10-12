package handler

import (
	"net/http"

	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// SimulateInbound accepts a mock inbound event payload and publishes it to the
// Redis inbound stream, simulating a message from a 3rd-party platform.
func (h *Handler) SimulateInbound(w http.ResponseWriter, r *http.Request) {
	accountID, _, _ := callerFrom(r)

	var body struct {
		ChannelID         string `json:"channel_id"`
		SenderExternalID  string `json:"sender_external_id"`
		SenderDisplayName string `json:"sender_display_name"`
		SenderAvatarURL   string `json:"sender_avatar_url"`
		ContentType       string `json:"content_type"`
		Text              string `json:"text"`
		MediaURL          string `json:"media_url"`
	}

	if !decodeJSON(w, r, &body) {
		return
	}

	if body.ChannelID == "" {
		writeError(w, http.StatusBadRequest, "channel_id is required")
		return
	}
	if body.SenderExternalID == "" {
		writeError(w, http.StatusBadRequest, "sender_external_id is required")
		return
	}
	if body.ContentType == "" {
		body.ContentType = "text"
	}

	if err := h.svc.SimulateInbound(r.Context(), accountID, body.ChannelID, body.SenderExternalID, body.SenderDisplayName, body.SenderAvatarURL, body.ContentType, body.Text, body.MediaURL); err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "published"})
}

// EnsureChannelForSimulator creates an isolated development channel. It does
// not create provider credentials or a live adapter connection.
func (h *Handler) EnsureChannelForSimulator(w http.ResponseWriter, r *http.Request) {
	accountID, _, _ := callerFrom(r)

	var body struct {
		Provider string `json:"provider"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	channel, err := h.svc.EnsureSimulatorChannel(r.Context(), accountID, body.Provider)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, channel)
}

// ListChannelsForSimulator returns the list of channels for the current account
// so the dev widget can target a specific channel when simulating inbound messages.
func (h *Handler) ListChannelsForSimulator(w http.ResponseWriter, r *http.Request) {
	accountID, _, _ := callerFrom(r)

	channels, err := h.svc.ListChannels(r.Context(), accountID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	if channels == nil {
		channels = []*types.Channel{}
	}

	writeJSON(w, http.StatusOK, channels)
}
