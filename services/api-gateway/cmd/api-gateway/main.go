// api-gateway is the single entry point for all client traffic.
// It reverse-proxies requests to identity-svc and workspace-svc.
// In v1, service-to-service calls are plain HTTP (no service mesh).
//
// Routing table:
//   /auth/*           → identity-svc
//   /workspace/*      → workspace-svc
//   /onboarding/*     → workspace-svc
//   /users/*          → workspace-svc
//   /channels/*       → conversation-svc
//   /bridge-connections/* → conversation-svc
//   /conversations/*  → conversation-svc
//   /leads/*          → conversation-svc
//   /ws               → notification-svc (WebSocket)
//   /api/kb/*         → ai-kb-compiler (admin-only)
//   /healthz          → local health check
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
)


func main() {
	identitySvcURL := mustEnv("IDENTITY_SVC_URL")
	workspaceSvcURL := mustEnv("WORKSPACE_SVC_URL")
	conversationSvcURL := mustEnv("CONVERSATION_SVC_URL")
	notificationSvcURL := mustEnv("NOTIFICATION_SVC_URL")
	aiKBCompilerURL := envOrDefault("AI_KB_COMPILER_URL", "http://ai-kb-compiler:8085")
	port := envOrDefault("PORT", "8080")


	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	identityBase, err := url.Parse(identitySvcURL)
	if err != nil {
		logger.Error("invalid IDENTITY_SVC_URL", "error", err)
		os.Exit(1)
	}
	workspaceBase, err := url.Parse(workspaceSvcURL)
	if err != nil {
		logger.Error("invalid WORKSPACE_SVC_URL", "error", err)
		os.Exit(1)
	}
	conversationBase, err := url.Parse(conversationSvcURL)
	if err != nil {
		logger.Error("invalid CONVERSATION_SVC_URL", "error", err)
		os.Exit(1)
	}
	notificationBase, err := url.Parse(notificationSvcURL)
	if err != nil {
		logger.Error("invalid NOTIFICATION_SVC_URL", "error", err)
		os.Exit(1)
	}
	aiKBCompilerBase, err := url.Parse(aiKBCompilerURL)
	if err != nil {
		logger.Error("invalid AI_KB_COMPILER_URL", "error", err)
		os.Exit(1)
	}


	handler := newRouter(aiKBCompilerBase, identityBase, workspaceBase, conversationBase, notificationBase, logger)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      loggingMiddleware(logger)(handler),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  90 * time.Second,
	}

	go func() {
		logger.Info("api-gateway listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
	logger.Info("api-gateway stopped")
}

// newRouter builds and returns the gateway mux. Extracted for testability.
func newRouter(
	aiKBCompilerBase, identityBase, workspaceBase, conversationBase, notificationBase *url.URL,
	logger *slog.Logger,
) http.Handler {
	r := mux.NewRouter()
	r.Use(middleware.CSRFProtection())

	// Health check (local, not proxied)
	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok","service":"api-gateway"}`)
	}).Methods(http.MethodGet)

	// Proxy /auth/* → identity-svc
	r.PathPrefix("/auth/").Handler(proxy(identityBase, logger))

	// Proxy /workspace/* → workspace-svc
	r.PathPrefix("/workspace/").Handler(proxy(workspaceBase, logger))
	r.PathPrefix("/users").Handler(proxy(workspaceBase, logger))

	// Proxy /onboarding/* → workspace-svc
	r.PathPrefix("/onboarding").Handler(proxy(workspaceBase, logger))

	// Proxy /channels/* → conversation-svc
	r.PathPrefix("/channels").Handler(proxy(conversationBase, logger))

	// Proxy guided bridge connection setup → conversation-svc
	r.PathPrefix("/bridge-connections").Handler(proxy(conversationBase, logger))

	// Proxy /conversations/* → conversation-svc
	r.PathPrefix("/conversations").Handler(proxy(conversationBase, logger))

	// Proxy /leads/* → conversation-svc
	r.PathPrefix("/leads").Handler(proxy(conversationBase, logger))

	// Proxy /internal/conversations/* → conversation-svc
	r.PathPrefix("/internal/conversations").Handler(proxy(conversationBase, logger))

	// Proxy /simulate-inbound and /simulate/* → conversation-svc (dev test simulation)
	r.PathPrefix("/simulate").Handler(proxy(conversationBase, logger))
	r.Handle("/simulate-inbound", proxy(conversationBase, logger))

	// Proxy /webhooks/* → conversation-svc (native platform webhooks)
	r.PathPrefix("/webhooks").Handler(proxy(conversationBase, logger))

	// Proxy /ws → notification-svc (WebSocket)
	r.Handle("/ws", wsProxy(notificationBase, logger))

	// Proxy /api/kb/* → ai-kb-compiler (admin-only)
	r.PathPrefix("/api/kb/").Handler(kbProxy(aiKBCompilerBase, identityBase, logger))

	// Catch-all 404
	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
	})

	return r
}

// proxy returns an http.Handler that forwards requests to the given upstream base URL.
// Headers (including Cookie for session) are forwarded; Host is rewritten to the upstream.
func proxy(upstream *url.URL, logger *slog.Logger) http.Handler {
	client := &http.Client{
		Timeout: 25 * time.Second,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := *upstream
		target.Path = strings.TrimRight(target.Path, "/") + r.URL.Path
		target.RawQuery = r.URL.RawQuery

		req, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), r.Body)
		if err != nil {
			logger.Error("proxy: create request", "error", err)
			http.Error(w, "gateway error", http.StatusBadGateway)
			return
		}

		// Forward all headers (including Cookie for session authentication)
		for key, vals := range r.Header {
			for _, v := range vals {
				req.Header.Add(key, v)
			}
		}
		req.Header.Set("X-Forwarded-For", r.RemoteAddr)
		req.Header.Set("X-Real-IP", r.RemoteAddr)

		resp, err := client.Do(req)
		if err != nil {
			logger.Error("proxy: upstream error", "target", target.String(), "error", err)
			http.Error(w, "gateway error: upstream unavailable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Forward response headers (including Set-Cookie for sessions)
		for key, vals := range resp.Header {
			for _, v := range vals {
				w.Header().Add(key, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body) //nolint:errcheck
	})
}

func loggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"duration", time.Since(start).String(),
				"remote", r.RemoteAddr,
			)
		})
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required env var %q is not set", key))
	}
	return v
}

func envOrDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

// wsProxy dials the upstream server and copies bytes bidirectionally to support WebSocket proxying.
func wsProxy(upstreamURL *url.URL, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Connection"), "upgrade") ||
			!strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			http.Error(w, "websocket proxy: not an upgrade request", http.StatusBadRequest)
			return
		}

		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
			return
		}
		clientConn, _, err := hj.Hijack()
		if err != nil {
			logger.Error("websocket proxy: hijack failed", "error", err)
			return
		}
		defer clientConn.Close()

		upstreamAddr := upstreamURL.Host
		if !strings.Contains(upstreamAddr, ":") {
			if upstreamURL.Scheme == "https" || upstreamURL.Scheme == "wss" {
				upstreamAddr += ":443"
			} else {
				upstreamAddr += ":80"
			}
		}
		upstreamConn, err := net.Dial("tcp", upstreamAddr)
		if err != nil {
			logger.Error("websocket proxy: dial upstream failed", "addr", upstreamAddr, "error", err)
			_, _ = clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
			return
		}
		defer upstreamConn.Close()

		path := r.URL.Path
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		reqStr := fmt.Sprintf("%s %s HTTP/1.1\r\n", r.Method, path)
		reqStr += fmt.Sprintf("Host: %s\r\n", upstreamURL.Host)
		for k, vals := range r.Header {
			if strings.EqualFold(k, "Host") {
				continue
			}
			for _, v := range vals {
				reqStr += fmt.Sprintf("%s: %s\r\n", k, v)
			}
		}
		reqStr += "\r\n"

		_, err = upstreamConn.Write([]byte(reqStr))
		if err != nil {
			logger.Error("websocket proxy: write upstream failed", "error", err)
			return
		}

		errChan := make(chan error, 2)
		cp := func(dst io.Writer, src io.Reader) {
			_, err := io.Copy(dst, src)
			errChan <- err
		}
		go cp(clientConn, upstreamConn)
		go cp(upstreamConn, clientConn)

		<-errChan
	})
}

// kbProxy validates the session cookie with identity-svc, checks if the role is admin,
// injects X-Account-ID and X-User-ID headers, and proxies the request to the KB compiler service.
func kbProxy(kbBase, identityBase *url.URL, logger *slog.Logger) http.Handler {
	client := &http.Client{
		Timeout: 25 * time.Second,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Validate session against identity-svc/auth/me
		authMeURL := fmt.Sprintf("%s://%s/auth/me", identityBase.Scheme, identityBase.Host)
		authReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, authMeURL, nil)
		if err != nil {
			logger.Error("kbProxy: create auth request", "error", err)
			http.Error(w, "gateway error", http.StatusBadGateway)
			return
		}

		// Forward Cookie header
		if cookie := r.Header.Get("Cookie"); cookie != "" {
			authReq.Header.Set("Cookie", cookie)
		}

		authResp, err := client.Do(authReq)
		if err != nil {
			logger.Error("kbProxy: call identity-svc failed", "error", err)
			http.Error(w, "identity service unavailable", http.StatusBadGateway)
			return
		}
		defer authResp.Body.Close()

		if authResp.StatusCode != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"unauthenticated"}`)
			return
		}

		// Parse user details
		var authMe struct {
			UserID    string `json:"user_id"`
			AccountID string `json:"account_id"`
			Role      string `json:"role"`
		}
		if err := json.NewDecoder(authResp.Body).Decode(&authMe); err != nil {
			logger.Error("kbProxy: decode auth response", "error", err)
			http.Error(w, "invalid auth response", http.StatusInternalServerError)
			return
		}

		// 2. Enforce manager role
		if authMe.Role != "manager" && authMe.Role != "admin" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":"forbidden: insufficient role"}`)
			return
		}

		// 3. Forward request to kb-compiler (rewriting /api/kb/ to /internal/kb/)
		targetPath := strings.Replace(r.URL.Path, "/api/kb/", "/internal/kb/", 1)
		target := *kbBase
		target.Path = strings.TrimRight(target.Path, "/") + targetPath
		target.RawQuery = r.URL.RawQuery

		req, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), r.Body)
		if err != nil {
			logger.Error("kbProxy: create forwarding request", "error", err)
			http.Error(w, "gateway error", http.StatusBadGateway)
			return
		}

		// Forward headers
		for key, vals := range r.Header {
			for _, v := range vals {
				req.Header.Add(key, v)
			}
		}

		// Inject trusted tenant and user headers
		req.Header.Set("X-Account-ID", authMe.AccountID)
		req.Header.Set("X-User-ID", authMe.UserID)

		resp, err := client.Do(req)
		if err != nil {
			logger.Error("kbProxy: upstream error", "target", target.String(), "error", err)
			http.Error(w, "gateway error: upstream unavailable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		for key, vals := range resp.Header {
			for _, v := range vals {
				w.Header().Add(key, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
}
