package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/whatfunnel/whatfunnel/packages/go-common/config"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/handler"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/service"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/session"
)

func main() {
	cfg := config.MustLoad()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	logger.Info("connected to database")

	sess := session.New(pool, cfg.SessionSecret)
	svc, err := service.New(pool, cfg.EncryptionKey)
	if err != nil {
		logger.Error("failed to init service", "error", err)
		os.Exit(1)
	}

	r := mux.NewRouter()
	r.Use(loggingMiddleware(logger))

	h := handler.New(svc, sess)
	h.RegisterRoutes(r)

	// Health check
	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	}).Methods(http.MethodGet)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("workspace-svc listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
	logger.Info("workspace-svc stopped")
}

func loggingMiddleware(logger *slog.Logger) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"duration", time.Since(start).String(),
			)
		})
	}
}
