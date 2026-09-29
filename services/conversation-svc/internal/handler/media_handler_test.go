package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/handler"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

func TestHandler_GetMedia_SecurityHeaders(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testPool(t)
	accountID, userID := setupTestTenant(t, pool, "media-sec-test")

	// Create a channel
	var channelID uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO channels (account_id, type, provider, status)
		VALUES ($1, 'whatsapp', 'whatsapp', 'connected') RETURNING id
	`, accountID).Scan(&channelID)
	require.NoError(t, err)

	svc := service.New(pool, nil)
	cacheDir := filepath.Join(t.TempDir(), "media-cache")
	require.NoError(t, svc.ConfigureMediaCache(cacheDir))

	h := handler.New(svc, &mockSessionStore{})

	createMedia := func(filename, mimeType string, data []byte) uuid.UUID {
		mediaID := uuid.New()
		storageKey := mediaID.String()
		filePath := filepath.Join(cacheDir, storageKey)
		require.NoError(t, os.WriteFile(filePath, data, 0o600))

		_, err := pool.Exec(context.Background(), `
			INSERT INTO media_objects (
				id, account_id, channel_id, filename, mime_type, size_bytes,
				storage_key, expires_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, mediaID, accountID, channelID, filename, mimeType, int64(len(data)), storageKey, time.Now().Add(24*time.Hour))
		require.NoError(t, err)
		return mediaID
	}

	tests := []struct {
		name                 string
		filename             string
		mimeType             string
		data                 []byte
		wantDispositionType  string // "attachment" or "inline"
		wantCSP              string
		wantXContentTypeOpts string
	}{
		{
			name:                 "SVG image must be attachment (prevent Stored XSS)",
			filename:             "exploit.svg",
			mimeType:             "image/svg+xml",
			data:                 []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
			wantDispositionType:  "attachment",
			wantCSP:              "default-src 'none'; sandbox",
			wantXContentTypeOpts: "nosniff",
		},
		{
			name:                 "HTML document must be attachment (prevent Stored XSS)",
			filename:             "phish.html",
			mimeType:             "text/html",
			data:                 []byte(`<html><body><script>alert(document.cookie)</script></body></html>`),
			wantDispositionType:  "attachment",
			wantCSP:              "default-src 'none'; sandbox",
			wantXContentTypeOpts: "nosniff",
		},
		{
			name:                 "XML document must be attachment",
			filename:             "data.xml",
			mimeType:             "text/xml",
			data:                 []byte(`<?xml version="1.0"?><root></root>`),
			wantDispositionType:  "attachment",
			wantCSP:              "default-src 'none'; sandbox",
			wantXContentTypeOpts: "nosniff",
		},
		{
			name:                 "PDF document must be attachment",
			filename:             "doc.pdf",
			mimeType:             "application/pdf",
			data:                 []byte(`%PDF-1.4 ...`),
			wantDispositionType:  "attachment",
			wantCSP:              "default-src 'none'; sandbox",
			wantXContentTypeOpts: "nosniff",
		},
		{
			name:                 "Unknown binary must be attachment",
			filename:             "payload.bin",
			mimeType:             "application/octet-stream",
			data:                 []byte{0x00, 0x01, 0x02},
			wantDispositionType:  "attachment",
			wantCSP:              "default-src 'none'; sandbox",
			wantXContentTypeOpts: "nosniff",
		},
		{
			name:                 "PNG raster image can be inline",
			filename:             "photo.png",
			mimeType:             "image/png",
			data:                 []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR..."),
			wantDispositionType:  "inline",
			wantCSP:              "default-src 'none'; sandbox",
			wantXContentTypeOpts: "nosniff",
		},
		{
			name:                 "JPEG raster image can be inline",
			filename:             "photo.jpg",
			mimeType:             "image/jpeg",
			data:                 []byte("\xff\xd8\xff..."),
			wantDispositionType:  "inline",
			wantCSP:              "default-src 'none'; sandbox",
			wantXContentTypeOpts: "nosniff",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mediaID := createMedia(tc.filename, tc.mimeType, tc.data)

			req := httptest.NewRequest(http.MethodGet, "/media/"+mediaID.String(), nil)
			req = req.WithContext(context.WithValue(req.Context(), types.ContextKeyAccountID, accountID))
			req = req.WithContext(context.WithValue(req.Context(), types.ContextKeyUserID, userID))
			req = req.WithContext(context.WithValue(req.Context(), types.ContextKeyUserRole, types.RoleManager))
			req = mux.SetURLVars(req, map[string]string{"id": mediaID.String()})

			rr := httptest.NewRecorder()
			h.GetMedia(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, tc.wantCSP, rr.Header().Get("Content-Security-Policy"))
			assert.Equal(t, tc.wantXContentTypeOpts, rr.Header().Get("X-Content-Type-Options"))

			disposition := rr.Header().Get("Content-Disposition")
			assert.NotEmpty(t, disposition)
			assert.Contains(t, disposition, tc.wantDispositionType)
			assert.Contains(t, disposition, tc.filename)
		})
	}
}
