package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/packages/go-common/webhooks"
)

// HandlePlatformWebhookVerification handles the Meta GET verification handshake (hub.challenge).
func (h *Handler) HandlePlatformWebhookVerification(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	channelIDStr := vars["channel_id"]
	if channelIDStr == "" {
		channelIDStr = r.URL.Query().Get("channel_id")
	}

	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode == "" || challenge == "" {
		writeError(w, http.StatusBadRequest, "missing hub.mode or hub.challenge")
		return
	}

	expectedToken := ""
	if channelIDStr != "" {
		if chUUID, err := uuid.Parse(channelIDStr); err == nil {
			if creds, _, err := h.svc.GetChannelWebhookCredentials(r.Context(), chUUID); err == nil && creds != nil {
				expectedToken = creds.VerifyToken
				if expectedToken == "" {
					expectedToken = creds.WebhookSecret
				}
			}
		}
	}

	if expectedToken != "" {
		verifiedChallenge, err := webhooks.VerifyMetaChallenge(mode, token, expectedToken, challenge)
		if err != nil {
			writeError(w, http.StatusForbidden, "verification token mismatch")
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(verifiedChallenge))
		return
	}

	// If no verify token configured on channel yet but valid handshake format
	if mode == "subscribe" && challenge != "" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(challenge))
		return
	}

	writeError(w, http.StatusForbidden, "invalid verification request")
}

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
	isAuthenticated := false
	if accountID, ok := middleware.AccountIDFromContext(r); ok {
		isAuthenticated = true
		if channelIDStr == "" {
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

	chUUID, parseErr := uuid.Parse(channelIDStr)

	// Webhook signature / token verification
	if parseErr == nil {
		creds, _, err := h.svc.GetChannelWebhookCredentials(r.Context(), chUUID)
		if err == nil && creds != nil {
			switch platform {
			case "telegram":
				secret := creds.SecretToken
				if secret == "" {
					secret = creds.WebhookSecret
				}
				sig := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
				if secret != "" {
					if err := webhooks.VerifyTelegramSecret(sig, secret); err != nil {
						writeError(w, http.StatusUnauthorized, "invalid telegram secret token")
						return
					}
				} else if sig != "" {
					writeError(w, http.StatusUnauthorized, "missing channel secret for verification")
					return
				}
			case "whatsapp", "instagram", "messenger":
				appSecret := creds.AppSecret
				if appSecret == "" {
					appSecret = creds.WebhookSecret
				}
				sig := r.Header.Get("X-Hub-Signature-256")
				if appSecret != "" {
					if err := webhooks.VerifyMetaSignature(bodyBytes, sig, appSecret); err != nil {
						writeError(w, http.StatusUnauthorized, "invalid webhook signature")
						return
					}
				} else if sig != "" {
					writeError(w, http.StatusUnauthorized, "missing channel secret for signature verification")
					return
				}
			}
		} else if !isAuthenticated && (r.Header.Get("X-Hub-Signature-256") != "" || r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != "") {
			writeError(w, http.StatusUnauthorized, "channel not found or unconfigured")
			return
		}
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
