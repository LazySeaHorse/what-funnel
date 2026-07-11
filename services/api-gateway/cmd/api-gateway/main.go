// api-gateway is the single entry point for all client traffic.
// It reverse-proxies requests to identity-svc and workspace-svc.
// In v1, service-to-service calls are plain HTTP (no service mesh).
//
// Routing table:
//   /auth/*        → identity-svc
//   /workspace/*   → workspace-svc
//   /healthz       → local health check
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
)

func main() {
	identitySvcURL := mustEnv("IDENTITY_SVC_URL")
	workspaceSvcURL := mustEnv("WORKSPACE_SVC_URL")
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

	r := mux.NewRouter()

	// Health check (local, not proxied)
	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok","service":"api-gateway"}`)
	}).Methods(http.MethodGet)

	// Proxy /auth/* → identity-svc
	r.PathPrefix("/auth/").Handler(proxy(identityBase, logger))

	// Proxy /workspace/* → workspace-svc
	r.PathPrefix("/workspace/").Handler(proxy(workspaceBase, logger))

	// Catch-all 404
	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
	})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      loggingMiddleware(logger)(r),
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
