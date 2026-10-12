package handler

import (
	"crypto/subtle"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

// mediaTransferTimeout bounds one media upload or download. The server-wide
// read/write timeouts (15s) are sized for JSON and would cut off a 20 MiB
// transfer on a slow link.
const mediaTransferTimeout = 5 * time.Minute

// extendMediaDeadlines lifts the server's per-request read and write
// deadlines for this transfer. Writers that cannot adjust deadlines (for
// example test recorders) keep the server defaults.
func extendMediaDeadlines(w http.ResponseWriter, request *http.Request) {
	controller := http.NewResponseController(w)
	deadline := time.Now().Add(mediaTransferTimeout)
	if err := controller.SetReadDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		slog.WarnContext(request.Context(), "extend media read deadline failed", "error", err)
	}
	if err := controller.SetWriteDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		slog.WarnContext(request.Context(), "extend media write deadline failed", "error", err)
	}
}

func (h *Handler) UploadConversationMedia(w http.ResponseWriter, request *http.Request) {
	viewer := mediaViewerFromRequest(request)
	extendMediaDeadlines(w, request)
	conversationID, ok := pathUUID(w, request, "id", "Invalid conversation ID.")
	if !ok {
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
		request.Context(), viewer, conversationID,
		header.Filename, header.Header.Get("Content-Type"), file,
	)
	if errors.Is(err, messaging.ErrMediaTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "Media must be 20 MiB or smaller.")
		return
	}
	if err != nil {
		if errors.Is(err, service.ErrNotFound) || errors.Is(err, service.ErrForbidden) || errors.Is(err, service.ErrValidation) {
			writeServiceError(w, request, err)
			return
		}
		slog.ErrorContext(request.Context(), "store outbound media failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not store media.")
		return
	}
	writeJSON(w, http.StatusCreated, media)
}

// mediaViewerFromRequest builds the authorisation subject for media access.
func mediaViewerFromRequest(request *http.Request) service.MediaViewer {
	accountID, userID, role := callerFrom(request)
	return service.MediaViewer{AccountID: accountID, UserID: userID, Role: role}
}

func (h *Handler) GetMedia(w http.ResponseWriter, request *http.Request) {
	viewer := mediaViewerFromRequest(request)
	h.serveMedia(w, request, &viewer)
}

func (h *Handler) serveMedia(w http.ResponseWriter, request *http.Request, viewer *service.MediaViewer) {
	mediaID, ok := pathUUID(w, request, "id", "Invalid media ID.")
	if !ok {
		return
	}
	extendMediaDeadlines(w, request)
	content, err := h.svc.OpenMedia(request.Context(), viewer, mediaID)
	if err != nil {
		writeMediaOpenError(w, request, err, "Media is unavailable. Open the original platform to view it.")
		return
	}
	defer content.Reader.Close()
	w.Header().Set("Cache-Control", "private, max-age=300")
	writeMediaBody(w, content, service.MediaContentDisposition)
}

// writeMediaBody streams opened media with the hardening headers every media
// response carries. disposition builds the Content-Disposition value from the
// (defaulted) MIME type and the filename.
func writeMediaBody(w http.ResponseWriter, content service.MediaContent, disposition func(mimeType, filename string) string) {
	mimeType := strings.TrimSpace(content.MIMEType)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Content-Disposition", disposition(mimeType, content.Filename))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, content.Reader)
}

// attachmentDisposition always forces a download, whatever the MIME type.
func attachmentDisposition(_, filename string) string {
	if filename == "" {
		return "attachment"
	}
	return mime.FormatMediaType("attachment", map[string]string{"filename": filename})
}

// writeMediaOpenError reports media that cannot be served as 404 with a
// user-facing hint and everything else (database, store, provider failures) as
// a logged 500 so operational problems are not disguised as missing files.
func writeMediaOpenError(w http.ResponseWriter, request *http.Request, err error, notFoundMessage string) {
	if errors.Is(err, service.ErrNotFound) {
		writeError(w, http.StatusNotFound, notFoundMessage)
		return
	}
	if errors.Is(err, messaging.ErrMediaTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "Media must be 20 MiB or smaller.")
		return
	}
	slog.ErrorContext(request.Context(), "open media failed", "error", err)
	writeError(w, http.StatusInternalServerError, "Could not read media.")
}

func NewInternalMediaHandler(svc *service.Service, secret string) http.Handler {
	expected := []byte(strings.TrimSpace(secret))
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		provided := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		if len(expected) == 0 || len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), expected) != 1 {
			writeError(w, http.StatusUnauthorized, "Unauthorized.")
			return
		}
		mediaID, ok := pathUUID(w, request, "id", "Invalid media ID.")
		if !ok {
			return
		}
		extendMediaDeadlines(w, request)
		content, err := svc.OpenMedia(request.Context(), nil, mediaID)
		if err != nil {
			writeMediaOpenError(w, request, err, "Media unavailable.")
			return
		}
		defer content.Reader.Close()
		writeMediaBody(w, content, attachmentDisposition)
	})
}
