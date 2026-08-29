package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/packages/go-common/webhooks"
)

// HandlePlatformWebhook processes native incoming webhooks from platforms (Telegram, WhatsApp, Instagram, Messenger).
func (h *Handler) HandlePlatformWebhook(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	platform := strings.ToLower(vars["platform"])
	channelIDStr := vars["channel_id"]
	if channelIDStr == "" {
		channelIDStr = r.URL.Query().Get("channel_id")
	}
	if channelIDStr == "" {
		channelIDStr = r.Header.Get("X-Channel-ID")
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	// If channelID is not provided directly, try resolving from context if authenticated
	if channelIDStr == "" {
		if accountID, ok := middleware.AccountIDFromContext(r); ok {
			chs, err := h.svc.ListChannels(r.Context(), accountID)
			if err == nil {
				for _, ch := range chs {
					if ch.Type == platform || ch.Type == "matrix_"+platform {
						channelIDStr = ch.ID.String()
						break
					}
				}
			}
		}
	}

	if channelIDStr == "" {
		writeError(w, http.StatusBadRequest, "missing channel_id or could not resolve channel for platform")
		return
	}

	var events []types.InboundEvent
	switch platform {
	case "telegram":
		events, err = webhooks.ParseTelegramUpdate(channelIDStr, bodyBytes)
	case "whatsapp":
		events, err = webhooks.ParseWhatsAppWebhook(channelIDStr, bodyBytes)
	case "instagram", "messenger":
		events, err = webhooks.ParseMetaWebhook(channelIDStr, bodyBytes)
	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported platform: %s", platform))
		return
	}

	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse %s payload: %v", platform, err))
		return
	}

	for _, event := range events {
		if err := h.svc.PublishInbound(r.Context(), event); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to process inbound event: %v", err))
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"processed_events": len(events),
	})
}
