package main

import (
	"context"
	"encoding/json"
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
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/adapterclient"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/handler"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/session"
	"golang.org/x/sync/errgroup"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	if err := run(logger); err != nil {
		logger.Error("conversation-svc stopped with an error", "error", err)
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

	// 3. Initialize Service and Session Store
	svc := service.New(pool, psClient)
	sess := session.New(pool, cfg.SessionSecret, cfg.CookieSecure)

	// 5. Register the internal WhatsApp control plane. The adapter owns all
	// WhatsApp session material; this service only stores domain state.
	whatsAppControl, err := adapterclient.New(cfg.WhatsAppAdapterURL, cfg.AdapterSharedSecret)
	if err != nil {
		return fmt.Errorf("configure whatsapp adapter: %w", err)
	}
	svc.RegisterAdapterControl(messaging.ProviderWhatsApp, whatsAppControl)
	svc.RegisterProviderMediaFetcher(messaging.ProviderWhatsApp, whatsAppControl)
	telegramControl, err := adapterclient.New(cfg.TelegramAdapterURL, cfg.AdapterSharedSecret)
	if err != nil {
		return fmt.Errorf("configure telegram adapter: %w", err)
	}
	svc.RegisterAdapterControl(messaging.ProviderTelegram, telegramControl)
	svc.RegisterProviderMediaFetcher(messaging.ProviderTelegram, telegramControl)
	if err := svc.ConfigureMediaCache(cfg.MediaCachePath); err != nil {
		return fmt.Errorf("configure media cache: %w", err)
	}

	// 6. Initialize HTTP API Routes
	r := mux.NewRouter()
	r.Use(loggingMiddleware(logger))

	h := handler.New(svc, sess)
	h.RegisterRoutes(r)
	r.Handle("/internal/media/{id}", handler.NewInternalMediaHandler(svc, cfg.AdapterSharedSecret)).Methods(http.MethodGet)

	// Health Check Endpoint
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

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		logger.Info("starting provider event consumer")
		err := psClient.Consume(groupCtx, "adapter.events", "conversation-svc-provider-events", "conversation-svc-provider-events-1", func(ctx context.Context, _ string, payload []byte) error {
			var event messaging.Event
			if err := json.Unmarshal(payload, &event); err != nil {
				logger.Error("discarding malformed provider event", "error", err)
				return nil
			}
			if err := event.Validate(); err != nil {
				logger.Error("discarding invalid provider event", "event_id", event.ID, "error", err)
				return nil
			}
			if err := svc.IngestProviderEvent(ctx, event); err != nil {
				logger.Error("failed to ingest provider event", "event_id", event.ID, "error", err)
				return err
			}
			return nil
		})
		return consumerResult(groupCtx, "adapter.events", err)
	})
	group.Go(func() error {
		logger.Info("starting provider command outbox")
		err := svc.DispatchOutbox(groupCtx)
		if groupCtx.Err() != nil && errors.Is(err, groupCtx.Err()) {
			return nil
		}
		return fmt.Errorf("dispatch provider commands: %w", err)
	})
	group.Go(func() error {
		logger.Info("starting media cache cleanup")
		err := svc.RunMediaCleanup(groupCtx)
		if groupCtx.Err() != nil && errors.Is(err, groupCtx.Err()) {
			return nil
		}
		return fmt.Errorf("clean media cache: %w", err)
	})
	group.Go(func() error {
		logger.Info("conversation-svc listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		<-groupCtx.Done()
		logger.Info("shutting down conversation-svc...")
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
	logger.Info("conversation-svc stopped")
	return nil
}

func consumerResult(ctx context.Context, stream string, err error) error {
	if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("consume stream %s: %w", stream, err)
	}
	return fmt.Errorf("consume stream %s stopped unexpectedly", stream)
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
