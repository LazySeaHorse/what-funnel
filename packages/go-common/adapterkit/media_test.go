package adapterkit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

func TestNewHTTPMediaSourceRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		secret  string
	}{
		{name: "relative url", baseURL: "/media", secret: "secret"},
		{name: "unsupported scheme", baseURL: "file:///tmp/media", secret: "secret"},
		{name: "empty secret", baseURL: "http://conversation-svc:8080"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewHTTPMediaSource(test.baseURL, test.secret, time.Second); err == nil {
				t.Fatal("NewHTTPMediaSource() error = nil, want error")
			}
		})
	}
}

func TestHTTPMediaSourceFetch(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "denied", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/internal/media/ok":
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Content-Disposition", `attachment; filename="../pic.png"`)
			_, _ = w.Write([]byte("abc"))
		case "/internal/media/big":
			_, _ = w.Write(make([]byte, messaging.MaxMediaBytes+1))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	source, err := NewHTTPMediaSource(server.URL+"/", "secret", time.Second)
	if err != nil {
		t.Fatalf("NewHTTPMediaSource() error = %v", err)
	}
	file, err := source.Fetch(context.Background(), "ok")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if string(file.Data) != "abc" || file.MIMEType != "image/png" || file.Filename != "pic.png" {
		t.Fatalf("Fetch() = %+v", file)
	}
	if _, err := source.Fetch(context.Background(), "missing"); err == nil {
		t.Fatal("Fetch(missing) error = nil, want status error")
	}
	if _, err := source.Fetch(context.Background(), "big"); !errors.Is(err, messaging.ErrMediaTooLarge) {
		t.Fatalf("Fetch(big) error = %v, want ErrMediaTooLarge", err)
	}
}
