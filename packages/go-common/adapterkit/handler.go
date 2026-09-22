package adapterkit

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

const maxRequestBytes = 1024 * 1024

// Controller defines the contract an adapter session manager must satisfy to be controlled over HTTP.
type Controller interface {
	Create(ctx context.Context, channelID, credential string) (Snapshot, error)
	Retry(ctx context.Context, channelID, credential string) (Snapshot, error)
	Snapshot(channelID string) (Snapshot, error)
	Logout(ctx context.Context, channelID string) error
	Download(ctx context.Context, channelID, providerRef string) (MediaFile, error)
}

// HandlerConfig configures the adapter HTTP control plane.
type HandlerConfig struct {
	ProviderName string // Human-readable provider name, e.g. "Telegram", "WhatsApp"
	SharedSecret string
}

// Handler serves HTTP control plane endpoints for an adapter.
type Handler struct {
	controller   Controller
	providerName string
	secret       []byte
}

// NewHandler creates a new adapter control HTTP handler.
func NewHandler(controller Controller, cfg HandlerConfig) (*Handler, error) {
	if controller == nil || strings.TrimSpace(cfg.SharedSecret) == "" {
		return nil, errors.New("adapter control: invalid handler configuration")
	}
	name := strings.TrimSpace(cfg.ProviderName)
	if name == "" {
		name = "Adapter"
	}
	return &Handler{
		controller:   controller,
		providerName: name,
		secret:       []byte(cfg.SharedSecret),
	}, nil
}

// Routes registers control and media routes and wraps them with authentication middleware.
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
		Credential string `json:"credential,omitempty"`
	}
	if !decodeBody(w, request, &body) {
		return
	}
	snapshot, err := h.controller.Create(request.Context(), body.ChannelID, body.Credential)
	if err != nil {
		if errors.Is(err, ErrAlreadyExists) {
			writeError(w, http.StatusConflict, fmt.Sprintf("This %s connection already exists.", h.providerName))
			return
		}
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Could not connect %s.", h.providerName))
		return
	}
	writeJSON(w, http.StatusAccepted, snapshot)
}

func (h *Handler) retry(w http.ResponseWriter, request *http.Request) {
	var body struct {
		Credential string `json:"credential,omitempty"`
	}
	if !decodeBody(w, request, &body) {
		return
	}
	snapshot, err := h.controller.Retry(request.Context(), request.PathValue("channelID"), body.Credential)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, fmt.Sprintf("%s connection not found.", h.providerName))
			return
		}
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Could not restart %s pairing.", h.providerName))
		return
	}
	writeJSON(w, http.StatusAccepted, snapshot)
}

func (h *Handler) get(w http.ResponseWriter, request *http.Request) {
	snapshot, err := h.controller.Snapshot(request.PathValue("channelID"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, fmt.Sprintf("%s connection not found.", h.providerName))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Could not read %s connection.", h.providerName))
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (h *Handler) delete(w http.ResponseWriter, request *http.Request) {
	if err := h.controller.Logout(request.Context(), request.PathValue("channelID")); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, fmt.Sprintf("%s connection not found.", h.providerName))
			return
		}
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Could not disconnect %s.", h.providerName))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) download(w http.ResponseWriter, request *http.Request) {
	media, err := h.controller.Download(
		request.Context(),
		request.PathValue("channelID"),
		request.PathValue("providerRef"),
	)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrMediaNotFound) {
			writeError(w, http.StatusNotFound, fmt.Sprintf("%s media not found.", h.providerName))
			return
		}
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Could not download %s media.", h.providerName))
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
	if request.Body == nil {
		return true
	}
	defer request.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, maxRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
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
