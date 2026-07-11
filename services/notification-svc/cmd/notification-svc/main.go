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
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"github.com/whatfunnel/whatfunnel/services/notification-svc/internal/consumer"
	"github.com/whatfunnel/whatfunnel/services/notification-svc/internal/server"
	"github.com/whatfunnel/whatfunnel/services/notification-svc/internal/session"
)

func main() {
	cfg := config.MustLoad()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Connect to Database
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	logger.Info("connected to database")

	// 2. Connect to Redis
	psClient, err := pubsub.NewClient(cfg.RedisURL)
	if err != nil {
		logger.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer psClient.Close()
	logger.Info("connected to redis")

	// 3. Initialize Session Store & Hub
	sess := session.New(pool, cfg.SessionSecret)
	hub := server.NewHub(logger)
	go hub.Run(ctx)

	// 4. Initialize and Start Event Consumer
	consumerName := fmt.Sprintf("notification-svc-%s", getHostname())
	c := consumer.NewConsumer(pool, psClient, hub, logger)
	c.Start(ctx, consumerName)

	// 5. Initialize Server & Routes
	srvHandler := server.NewServer(hub, sess, logger)
	r := mux.NewRouter()
	r.Use(loggingMiddleware(logger))

	r.HandleFunc("/ws", srvHandler.HandleWS)

	// Health Check
	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok","service":"notification-svc"}`)
	}).Methods(http.MethodGet)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("notification-svc listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down notification-svc...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
	logger.Info("notification-svc stopped")
}

func loggingMiddleware(logger *slog.Logger) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			// Avoid logging verbose ping/pong or upgrade logs at Info level unless desired
			if r.URL.Path != "/ws" {
				logger.Info("request",
					"method", r.Method,
					"path", r.URL.Path,
					"duration", time.Since(start).String(),
				)
			}
		})
	}
}

func getHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}
