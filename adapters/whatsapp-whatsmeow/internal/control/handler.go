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

	"github.com/whatfunnel/whatfunnel/adapters/whatsapp-whatsmeow/internal/session"
)

const maxRequestBytes = 1024 * 1024

type Controller interface {
	Create(ctx context.Context, channelID string) (session.Snapshot, error)
	Snapshot(channelID string) (session.Snapshot, error)
	Logout(ctx context.Context, channelID string) error
	Download(ctx context.Context, channelID, providerRef string) (session.MediaFile, error)
}

type Handler struct {
	controller Controller
	secret     []byte
}

func NewHandler(controller Controller, secret string) (*Handler, error) {
	if controller == nil || strings.TrimSpace(secret) == "" {
		return nil, errors.New("whatsapp control: invalid handler configuration")
	}
	return &Handler{controller: controller, secret: []byte(secret)}, nil
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/connections", h.create)
	mux.HandleFunc("GET /v1/connections/{channelID}", h.get)
	mux.HandleFunc("DELETE /v1/connections/{channelID}", h.delete)
	mux.HandleFunc("GET /v1/media/{channelID}/{providerRef}", h.download)
	return h.authenticate(mux)
}

func (h *Handler) download(w http.ResponseWriter, request *http.Request) {
	media, err := h.controller.Download(
		request.Context(),
		request.PathValue("channelID"),
		request.PathValue("providerRef"),
	)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			writeError(w, http.StatusNotFound, "WhatsApp media not found.")
			return
		}
		writeError(w, http.StatusBadGateway, "Could not download WhatsApp media.")
		return
	}
	if media.MIMEType != "" {
		w.Header().Set("Content-Type", media.MIMEType)
	}
	if media.Filename != "" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": media.Filename}))
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(media.Data)
}

func (h *Handler) create(w http.ResponseWriter, request *http.Request) {
	defer request.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, maxRequestBytes))
	decoder.DisallowUnknownFields()
	var body struct {
		ChannelID string `json:"channel_id"`
	}
	if err := decoder.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body.")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "Invalid request body.")
		return
	}

	snapshot, err := h.controller.Create(request.Context(), body.ChannelID)
	if err != nil {
		if errors.Is(err, session.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, "This WhatsApp connection already exists.")
			return
		}
		writeError(w, http.StatusBadGateway, "Could not start WhatsApp pairing.")
		return
	}
	writeJSON(w, http.StatusAccepted, snapshot)
}

func (h *Handler) get(w http.ResponseWriter, request *http.Request) {
	snapshot, err := h.controller.Snapshot(request.PathValue("channelID"))
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			writeError(w, http.StatusNotFound, "WhatsApp connection not found.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not read WhatsApp connection.")
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (h *Handler) delete(w http.ResponseWriter, request *http.Request) {
	err := h.controller.Logout(request.Context(), request.PathValue("channelID"))
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			writeError(w, http.StatusNotFound, "WhatsApp connection not found.")
			return
		}
		writeError(w, http.StatusBadGateway, "Could not unlink WhatsApp.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
