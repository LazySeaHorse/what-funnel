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
	"golang.org/x/sync/errgroup"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	if err := run(logger); err != nil {
		logger.Error("notification-svc stopped with an error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg := config.MustLoad()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 1. Connect to Database
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()
	logger.Info("connected to database")

	// 2. Connect to Redis
	psClient, err := pubsub.NewClient(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("connect to redis: %w", err)
	}
	defer psClient.Close()
	logger.Info("connected to redis")

	// 3. Initialize Session Store & Hub
	sess := session.New(pool, cfg.SessionSecret, cfg.CookieSecure)
	hub := server.NewHub(logger)
	defer hub.Close()

	// 4. Initialize and Start Event Consumer
	consumerName := fmt.Sprintf("notification-svc-%s", getHostname())
	c := consumer.NewConsumer(pool, psClient, hub, logger)

	// 5. Initialize Server & Routes
	srvHandler := server.NewServer(hub, sess, logger, cfg.AllowedOrigins, cfg.IsProduction())
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

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return c.Run(groupCtx, consumerName)
	})
	group.Go(func() error {
		logger.Info("notification-svc listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		<-groupCtx.Done()
		logger.Info("shutting down notification-svc...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
		return nil
	})

	if err := group.Wait(); err != nil {
		return err
	}
	logger.Info("notification-svc stopped")
	return nil
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
