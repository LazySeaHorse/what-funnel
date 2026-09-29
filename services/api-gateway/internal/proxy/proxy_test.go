package proxy_test

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/whatfunnel/whatfunnel/services/api-gateway/internal/proxy"
)

func TestHTTPProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Echo", "echoed")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "upstream body")
	}))
	defer upstream.Close()

	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream url: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := proxy.HTTP(u, logger)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-Custom-Echo") != "echoed" {
		t.Fatalf("expected X-Custom-Echo header, got %q", rec.Header().Get("X-Custom-Echo"))
	}
	if rec.Body.String() != "upstream body" {
		t.Fatalf("expected 'upstream body', got %q", rec.Body.String())
	}
}

func TestHTTPProxy_StripsInternalHeaders(t *testing.T) {
	var capturedHeader http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream url: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := proxy.HTTP(u, logger)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Internal-Token", "secret-token")
	req.Header.Set("X-Account-ID", "00000000-0000-0000-0000-000000000001")
	req.Header.Set("X-User-ID", "00000000-0000-0000-0000-000000000002")
	req.Header.Set("X-User-Role", "admin")
	req.Header.Set("X-Internal-Custom", "injected")
	req.Header.Set("X-Forwarded-Host", "spoofed-attacker.com")
	req.Header.Set("X-Legit-Header", "allowed")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if capturedHeader.Get("X-Internal-Token") != "" {
		t.Errorf("expected X-Internal-Token to be stripped, got %q", capturedHeader.Get("X-Internal-Token"))
	}
	if capturedHeader.Get("X-Account-ID") != "" {
		t.Errorf("expected X-Account-ID to be stripped, got %q", capturedHeader.Get("X-Account-ID"))
	}
	if capturedHeader.Get("X-User-ID") != "" {
		t.Errorf("expected X-User-ID to be stripped, got %q", capturedHeader.Get("X-User-ID"))
	}
	if capturedHeader.Get("X-User-Role") != "" {
		t.Errorf("expected X-User-Role to be stripped, got %q", capturedHeader.Get("X-User-Role"))
	}
	if capturedHeader.Get("X-Internal-Custom") != "" {
		t.Errorf("expected X-Internal-Custom to be stripped, got %q", capturedHeader.Get("X-Internal-Custom"))
	}
	if capturedHeader.Get("X-Forwarded-Host") == "spoofed-attacker.com" {
		t.Errorf("expected client X-Forwarded-Host to be stripped, got %q", capturedHeader.Get("X-Forwarded-Host"))
	}
	if capturedHeader.Get("X-Legit-Header") != "allowed" {
		t.Errorf("expected X-Legit-Header to be preserved, got %q", capturedHeader.Get("X-Legit-Header"))
	}
}

func TestWebSocketProxy_NonUpgrade(t *testing.T) {
	u, _ := url.Parse("http://127.0.0.1:8084")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := proxy.WebSocket(u, logger)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-upgrade request, got %d", rec.Code)
	}
}

func TestWebSocketProxy_StripsForwardedHost(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen error: %v", err)
	}
	defer ln.Close()

	headerChan := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 2048)
		n, _ := conn.Read(buf)
		headerChan <- string(buf[:n])
		_, _ = conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"))
	}()

	u, _ := url.Parse("http://" + ln.Addr().String())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := proxy.WebSocket(u, logger)

	ts := httptest.NewServer(handler)
	defer ts.Close()

	conn, err := net.Dial("tcp", ts.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer conn.Close()

	reqStr := "GET /ws HTTP/1.1\r\n" +
		"Host: client-requested.com\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"X-Forwarded-Host: evil-attacker.com\r\n" +
		"\r\n"
	_, _ = conn.Write([]byte(reqStr))

	select {
	case received := <-headerChan:
		if strings.Contains(received, "evil-attacker.com") {
			t.Fatalf("expected evil-attacker.com to be stripped, got:\n%s", received)
		}
		if !strings.Contains(received, "X-Forwarded-Host: client-requested.com") {
			t.Fatalf("expected X-Forwarded-Host: client-requested.com, got:\n%s", received)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for upstream request")
	}
}

func TestWebSocketProxy_FirefoxConnectionHeader(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj := w.(http.Hijacker)
		conn, rw, err := hj.Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\nhello-from-upstream")
		_ = rw.Flush()
		buf := make([]byte, 4)
		_, _ = io.ReadFull(rw, buf)
		_, _ = conn.Write(buf)
	}))
	defer upstream.Close()

	u, _ := url.Parse(upstream.URL)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ts := httptest.NewServer(proxy.WebSocket(u, logger))
	defer ts.Close()

	conn, err := net.Dial("tcp", ts.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	_, _ = conn.Write([]byte("GET /ws HTTP/1.1\r\nHost: example.com\r\nUpgrade: websocket\r\n" +
		"Connection: keep-alive, Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n"))
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101, got %d", resp.StatusCode)
	}
	// Bytes the upstream sent right after the handshake must not be lost.
	first := make([]byte, len("hello-from-upstream"))
	if _, err := io.ReadFull(reader, first); err != nil || string(first) != "hello-from-upstream" {
		t.Fatalf("expected buffered upstream bytes, got %q err=%v", first, err)
	}
	_, _ = conn.Write([]byte("ping"))
	echo := make([]byte, 4)
	if _, err := io.ReadFull(reader, echo); err != nil || string(echo) != "ping" {
		t.Fatalf("expected echo, got %q err=%v", echo, err)
	}
}

func TestHTTPProxy_ForwardedForOmitsPortAndRedirectsPassThrough(t *testing.T) {
	var forwardedFor, realIP string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwardedFor = r.Header.Get("X-Forwarded-For")
		realIP = r.Header.Get("X-Real-IP")
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	defer upstream.Close()

	u, _ := url.Parse(upstream.URL)
	handler := proxy.HTTP(u, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "203.0.113.9:54321"
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/elsewhere" {
		t.Fatalf("expected redirect passthrough, got %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if forwardedFor != "203.0.113.9" || realIP != "203.0.113.9" {
		t.Fatalf("expected port-less client ip, got XFF=%q X-Real-IP=%q", forwardedFor, realIP)
	}
}
