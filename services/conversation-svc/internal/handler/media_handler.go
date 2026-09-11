package handler

import (
	"crypto/subtle"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

func (h *Handler) UploadConversationMedia(w http.ResponseWriter, request *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(request)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Missing account.")
		return
	}
	conversationID, err := uuid.Parse(mux.Vars(request)["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid conversation ID.")
		return
	}
	request.Body = http.MaxBytesReader(w, request.Body, messaging.MaxMediaBytes+1024*1024)
	if err := request.ParseMultipartForm(1024 * 1024); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid media upload.")
		return
	}
	if request.MultipartForm != nil {
		defer request.MultipartForm.RemoveAll()
	}
	file, header, err := request.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "A media file is required.")
		return
	}
	defer file.Close()
	media, err := h.svc.SaveOutboundMedia(
		request.Context(), accountID, conversationID,
		header.Filename, header.Header.Get("Content-Type"), file,
	)
	if errors.Is(err, messaging.ErrMediaTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "Media must be 20 MiB or smaller.")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "Could not store media.")
		return
	}
	writeJSON(w, http.StatusCreated, media)
}

func (h *Handler) GetMedia(w http.ResponseWriter, request *http.Request) {
	accountID, ok := middleware.AccountIDFromContext(request)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Missing account.")
		return
	}
	h.serveMedia(w, request, &accountID)
}

func (h *Handler) serveMedia(w http.ResponseWriter, request *http.Request, accountID *uuid.UUID) {
	mediaID, err := uuid.Parse(mux.Vars(request)["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid media ID.")
		return
	}
	content, err := h.svc.OpenMedia(request.Context(), accountID, mediaID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Media is unavailable. Open the original platform to view it.")
		return
	}
	defer content.Reader.Close()
	w.Header().Set("Content-Type", content.MIMEType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=300")
	if content.Filename != "" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": content.Filename}))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, content.Reader)
}

func NewInternalMediaHandler(svc *service.Service, secret string) http.Handler {
	expected := []byte(strings.TrimSpace(secret))
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		provided := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		if len(expected) == 0 || len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), expected) != 1 {
			writeError(w, http.StatusUnauthorized, "Unauthorized.")
			return
		}
		mediaID, err := uuid.Parse(mux.Vars(request)["id"])
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid media ID.")
			return
		}
		content, err := svc.OpenMedia(request.Context(), nil, mediaID)
		if err != nil {
			writeError(w, http.StatusNotFound, "Media unavailable.")
			return
		}
		defer content.Reader.Close()
		w.Header().Set("Content-Type", content.MIMEType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if content.Filename != "" {
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": content.Filename}))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, content.Reader)
	})
}
