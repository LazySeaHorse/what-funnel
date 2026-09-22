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
//	/api/kb/*         → ai-kb-compiler (admin-only)
//	/healthz          → local health check
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
		Handler:      gwmiddleware.Logging(logger)(handler),
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
	r.Use(commonmw.CSRFProtection())

	// Health check (local, not proxied)
	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok","service":"api-gateway"}`)
	}).Methods(http.MethodGet)

	// Proxy /auth/* → identity-svc
	r.PathPrefix("/auth/").Handler(proxy.HTTP(identityBase, logger))

	// Proxy /workspace/* → workspace-svc
	r.PathPrefix("/workspace/").Handler(proxy.HTTP(workspaceBase, logger))
	r.PathPrefix("/users").Handler(proxy.HTTP(workspaceBase, logger))

	// Proxy /onboarding/* → workspace-svc
	r.PathPrefix("/onboarding").Handler(proxy.HTTP(workspaceBase, logger))

	// Proxy /channels/* → conversation-svc
	r.PathPrefix("/channels").Handler(proxy.HTTP(conversationBase, logger))
	r.PathPrefix("/channel-connections").Handler(proxy.HTTP(conversationBase, logger))

	// Proxy /conversations/* → conversation-svc
	r.PathPrefix("/conversations").Handler(proxy.HTTP(conversationBase, logger))
	r.PathPrefix("/media").Handler(proxy.HTTP(conversationBase, logger))

	// Proxy /leads/* → conversation-svc
	r.PathPrefix("/leads").Handler(proxy.HTTP(conversationBase, logger))

	// Proxy /internal/conversations/* → conversation-svc
	r.PathPrefix("/internal/conversations").Handler(proxy.HTTP(conversationBase, logger))

	// Proxy /simulate-inbound and /simulate/* → conversation-svc (dev test simulation)
	r.PathPrefix("/simulate").Handler(proxy.HTTP(conversationBase, logger))
	r.Handle("/simulate-inbound", proxy.HTTP(conversationBase, logger))

	// Proxy /ws → notification-svc (WebSocket)
	// Note: In production, Nginx proxies /ws directly to notification-svc:8084 to eliminate
	// double-proxy overhead. This route is retained for dev/test harness backwards compatibility.
	r.Handle("/ws", proxy.WebSocket(notificationBase, logger))

	// Proxy /api/kb/* → ai-kb-compiler (admin-only)
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
