package control

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/whatfunnel/whatfunnel/adapters/telegram-botapi/internal/session"
)

const maxRequestBytes = 1024 * 1024

type Controller interface {
	Create(context.Context, string, string) (session.Snapshot, error)
	Retry(context.Context, string, string) (session.Snapshot, error)
	Snapshot(string) (session.Snapshot, error)
	Logout(context.Context, string) error
	Download(context.Context, string, string) (session.MediaFile, error)
}

type Handler struct {
	controller Controller
	secret     []byte
}

func NewHandler(controller Controller, secret string) (*Handler, error) {
	if controller == nil || strings.TrimSpace(secret) == "" {
		return nil, errors.New("telegram control: invalid handler configuration")
	}
	return &Handler{controller: controller, secret: []byte(secret)}, nil
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/connections", h.create)
	mux.HandleFunc("POST /v1/connections/{channelID}/retry", h.retry)
	mux.HandleFunc("GET /v1/connections/{channelID}", h.get)
	mux.HandleFunc("DELETE /v1/connections/{channelID}", h.delete)
	mux.HandleFunc("GET /v1/media/{channelID}/{providerRef}", h.download)
	return h.authenticate(mux)
}

func (h *Handler) create(w http.ResponseWriter, request *http.Request) {
	var body struct {
		ChannelID  string `json:"channel_id"`
		Credential string `json:"credential"`
	}
	if !decodeBody(w, request, &body) {
		return
	}
	snapshot, err := h.controller.Create(request.Context(), body.ChannelID, body.Credential)
	if err != nil {
		if errors.Is(err, session.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, "This Telegram bot is already connected.")
			return
		}
		writeError(w, http.StatusBadGateway, "Could not connect this Telegram bot. Check the token and try again.")
		return
	}
	writeJSON(w, http.StatusAccepted, snapshot)
}

func (h *Handler) retry(w http.ResponseWriter, request *http.Request) {
	var body struct {
		Credential string `json:"credential"`
	}
	if !decodeBody(w, request, &body) {
		return
	}
	snapshot, err := h.controller.Retry(request.Context(), request.PathValue("channelID"), body.Credential)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Telegram connection not found.")
			return
		}
		writeError(w, http.StatusBadGateway, "Could not reconnect this Telegram bot.")
		return
	}
	writeJSON(w, http.StatusAccepted, snapshot)
}

func (h *Handler) get(w http.ResponseWriter, request *http.Request) {
	snapshot, err := h.controller.Snapshot(request.PathValue("channelID"))
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Telegram connection not found.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not read Telegram connection.")
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (h *Handler) delete(w http.ResponseWriter, request *http.Request) {
	if err := h.controller.Logout(request.Context(), request.PathValue("channelID")); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Telegram connection not found.")
			return
		}
		writeError(w, http.StatusBadGateway, "Could not disconnect Telegram.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) download(w http.ResponseWriter, request *http.Request) {
	media, err := h.controller.Download(request.Context(), request.PathValue("channelID"), request.PathValue("providerRef"))
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Telegram media not found.")
			return
		}
		writeError(w, http.StatusBadGateway, "Could not download Telegram media.")
		return
	}
	if media.MIMEType != "" {
		w.Header().Set("Content-Type", media.MIMEType)
	}
	if media.Filename != "" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": media.Filename}))
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(media.Data)
}

func decodeBody(w http.ResponseWriter, request *http.Request, target any) bool {
	defer request.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, maxRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body.")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "Invalid request body.")
		return false
	}
	return true
}

func (h *Handler) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		provided := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		if len(provided) != len(h.secret) || subtle.ConstantTimeCompare([]byte(provided), h.secret) != 1 {
			writeError(w, http.StatusUnauthorized, "Unauthorized.")
			return
		}
		next.ServeHTTP(w, request)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
