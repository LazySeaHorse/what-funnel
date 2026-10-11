// api-gateway is the single entry point for all client traffic.
// It reverse-proxies requests to identity-svc and workspace-svc.
// In v1, service-to-service calls are plain HTTP (no service mesh).
//
// Routing table:
//
//	/auth/*           → identity-svc
//	/workspace/*      → workspace-svc
//	/onboarding/*     → workspace-svc
//	/users/*          → workspace-svc
//	/channels/*       → conversation-svc
//	/channel-connections/* → conversation-svc
//	/conversations/*  → conversation-svc
//	/leads/*          → conversation-svc
//	/ws               → notification-svc (WebSocket)
//	/api/kb/*         → ai-kb-compiler (manager-only)
//	/healthz          → local health check
//
// Prometheus metrics are served on METRICS_ADDR (default :9090), not the public listener.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/config"
	"github.com/whatfunnel/whatfunnel/packages/go-common/metrics"
	commonmw "github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	gwmiddleware "github.com/whatfunnel/whatfunnel/services/api-gateway/internal/middleware"
	"github.com/whatfunnel/whatfunnel/services/api-gateway/internal/proxy"
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

	mustParseURL := func(name, raw string) *url.URL {
		u, err := url.Parse(raw)
		if err != nil {
			logger.Error("invalid "+name, "error", err)
			os.Exit(1)
		}
		return u
	}
	identityBase := mustParseURL("IDENTITY_SVC_URL", identitySvcURL)
	workspaceBase := mustParseURL("WORKSPACE_SVC_URL", workspaceSvcURL)
	conversationBase := mustParseURL("CONVERSATION_SVC_URL", conversationSvcURL)
	notificationBase := mustParseURL("NOTIFICATION_SVC_URL", notificationSvcURL)
	aiKBCompilerBase := mustParseURL("AI_KB_COMPILER_URL", aiKBCompilerURL)

	handler := newRouter(aiKBCompilerBase, identityBase, workspaceBase, conversationBase, notificationBase, logger)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           gwmiddleware.Logging(logger)(handler),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// No WriteTimeout: it would cut off long downloads, streams and WebSocket tunnels.
		// The upstream transport bounds time-to-first-byte instead.
		IdleTimeout: 90 * time.Second,
	}

	// Metrics are served on a separate internal listener that is not published by compose.
	metricsAddr := envOrDefault("METRICS_ADDR", ":9090")
	metricsSrv := &http.Server{
		Addr:              metricsAddr,
		Handler:           newMetricsHandler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		logger.Info("api-gateway metrics listening", "addr", metricsAddr)
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics server error", "error", err)
		}
	}()

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
	_ = metricsSrv.Shutdown(ctx)
	logger.Info("api-gateway stopped")
}

// newMetricsHandler serves Prometheus metrics for the internal metrics listener.
func newMetricsHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.Handler())
	return mux
}

// newRouter builds and returns the gateway mux. Extracted for testability.
func newRouter(
	aiKBCompilerBase, identityBase, workspaceBase, conversationBase, notificationBase *url.URL,
	logger *slog.Logger,
) http.Handler {
	r := mux.NewRouter()
	r.Use(commonmw.CSRFProtection())

	// Health check (local, not proxied)
	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok","service":"api-gateway"}`)
	}).Methods(http.MethodGet)

	identity := proxy.HTTP(identityBase, logger)
	workspace := proxy.HTTP(workspaceBase, logger)
	conversation := proxy.HTTP(conversationBase, logger)

	// Proxy /auth/* → identity-svc
	r.PathPrefix("/auth/").Handler(identity)

	// Proxy /workspace/*, /users, /onboarding → workspace-svc
	r.PathPrefix("/workspace/").Handler(workspace)
	r.PathPrefix("/users").Handler(workspace)
	r.PathPrefix("/onboarding").Handler(workspace)

	// Proxy /channels, /channel-connections, /conversations, /media, /leads → conversation-svc
	r.PathPrefix("/channels").Handler(conversation)
	r.PathPrefix("/channel-connections").Handler(conversation)
	r.PathPrefix("/conversations").Handler(conversation)
	r.PathPrefix("/media").Handler(conversation)
	r.PathPrefix("/leads").Handler(conversation)

	// Internal inter-service endpoints and simulation test harnesses are not mounted
	// on the public gateway by default to prevent unauthorized external access.
	// They are mounted only when ENABLE_SIMULATION_ROUTES=true is explicitly set.
	if config.SimulationRoutesEnabled() {
		r.PathPrefix("/internal/conversations").Handler(conversation)
		r.PathPrefix("/simulate").Handler(conversation)
		r.Handle("/simulate-inbound", conversation)
	}

	// Proxy /ws → notification-svc (WebSocket)
	// Note: In production, Nginx proxies /ws directly to notification-svc:8084 to eliminate
	// double-proxy overhead. This route is retained for dev/test harness backwards compatibility.
	r.Handle("/ws", proxy.WebSocket(notificationBase, logger))

	// Proxy /api/kb/* → ai-kb-compiler (manager-only)
	r.PathPrefix("/api/kb/").Handler(proxy.KB(aiKBCompilerBase, identityBase, logger))

	// Catch-all 404
	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
	})

	return r
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
