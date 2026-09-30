package main

import (
	"context"
	"errors"
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
	"github.com/whatfunnel/whatfunnel/packages/go-common/metrics"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/services/identity-svc/internal/handler"
	"github.com/whatfunnel/whatfunnel/services/identity-svc/internal/service"
	"github.com/whatfunnel/whatfunnel/services/identity-svc/internal/session"
	"golang.org/x/sync/errgroup"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	if err := run(logger); err != nil {
		logger.Error("identity-svc stopped with an error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg := config.MustLoad()
	if err := middleware.ValidateInternalServiceToken(); err != nil {
		return fmt.Errorf("internal service auth: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()
	logger.Info("connected to database")

	sess := session.New(pool, cfg.SessionSecret, cfg.CookieSecure)
	svc, err := service.New(pool, sess)
	if err != nil {
		return fmt.Errorf("init service: %w", err)
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
	r.Handle("/metrics", metrics.Handler()).Methods(http.MethodGet)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	janitor := session.NewJanitor(sess, cfg.SessionPurgeInterval, logger)

	group, groupCtx := errgroup.WithContext(ctx)

	// Background worker: periodic expired session cleanup
	group.Go(func() error {
		logger.Info("starting session cleanup janitor", "interval", cfg.SessionPurgeInterval.String())
		if err := janitor.Run(groupCtx); err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("session janitor: %w", err)
		}
		return nil
	})

	// Background worker: HTTP API server
	group.Go(func() error {
		logger.Info("identity-svc listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	})

	// Graceful shutdown listener
	group.Go(func() error {
		<-groupCtx.Done()
		logger.Info("shutting down identity-svc...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
		return nil
	})

	if err := group.Wait(); err != nil {
		return err
	}
	logger.Info("identity-svc stopped")
	return nil
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
