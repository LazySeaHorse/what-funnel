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
	matrixadapter "github.com/whatfunnel/whatfunnel/adapters/matrix-mautrix"
	"github.com/whatfunnel/whatfunnel/packages/go-common/config"
	"github.com/whatfunnel/whatfunnel/packages/go-common/crypto"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
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

	// 3. Initialize Crypto Cipher
	cipher, err := crypto.NewCipherFromHex(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("initialize cipher: %w", err)
	}

	// 4. Initialize Service and Session Store
	svc := service.New(pool, cipher, psClient)
	svc.ConfigureBridgeConnections(service.BridgeConnectionConfig{
		Provisioning: matrixadapter.ProvisioningConfig{
			HomeserverURL:            cfg.MatrixHomeserverURL,
			ServerName:               cfg.MatrixServerName,
			RegistrationSharedSecret: cfg.MatrixRegistrationSharedSecret,
		},
		BridgeIdentities: map[string]string{
			"whatsapp":  cfg.MatrixWhatsAppBridgeIdentity,
			"telegram":  cfg.MatrixTelegramBridgeIdentity,
			"instagram": cfg.MatrixInstagramBridgeIdentity,
			"messenger": cfg.MatrixMessengerBridgeIdentity,
		},
	})
	sess := session.New(pool, cfg.SessionSecret, cfg.CookieSecure)

	// 5. Initialize and Register Matrix Adapter
	matrixAdapter := matrixadapter.New()
	svc.RegisterAdapter("matrix_whatsapp", matrixAdapter)
	svc.RegisterAdapter("matrix_instagram", matrixAdapter)
	svc.RegisterAdapter("matrix_messenger", matrixAdapter)
	svc.RegisterAdapter("matrix_telegram", matrixAdapter)

	// Load existing channels from DB and configure adapters
	if err := svc.InitAdapters(ctx); err != nil {
		logger.Error("failed to initialize adapter configurations from database", "error", err)
	}

	// 6. Initialize HTTP API Routes
	r := mux.NewRouter()
	r.Use(loggingMiddleware(logger))

	h := handler.New(svc, sess)
	h.RegisterRoutes(r)

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
		logger.Info("starting matrix adapter")
		err := matrixAdapter.Start(groupCtx, func(event types.InboundEvent) {
			logger.Info("received event from matrix adapter, publishing to redis", "channel_id", event.ChannelID, "event_id", event.Message.ExternalMessageID)
			if _, err := psClient.Publish(groupCtx, "messages.inbound", event); err != nil && groupCtx.Err() == nil {
				logger.Error("failed to publish inbound event to Redis", "error", err)
			}
		}, func(event types.ExternalOutboundEvent) {
			logger.Info("received external outbound event from matrix adapter, publishing to redis", "channel_id", event.ChannelID, "event_id", event.ExternalMessageID)
			if _, err := psClient.Publish(groupCtx, "messages.external_outbound", event); err != nil && groupCtx.Err() == nil {
				logger.Error("failed to publish external outbound event to Redis", "error", err)
			}
		})
		if err != nil {
			return fmt.Errorf("run matrix adapter: %w", err)
		}
		if groupCtx.Err() == nil {
			return errors.New("matrix adapter stopped unexpectedly")
		}
		return nil
	})
	group.Go(func() error {
		logger.Info("starting Redis stream consumer for messages.inbound")
		err := psClient.Consume(groupCtx, "messages.inbound", "conversation-svc-group", "conversation-svc-consumer", func(ctx context.Context, id string, payload []byte) error {
			var event types.InboundEvent
			if err := json.Unmarshal(payload, &event); err != nil {
				logger.Error("failed to unmarshal inbound event", "error", err)
				return nil // Return nil so malformed messages get acknowledged and cleared
			}

			logger.Info("processing message from Redis stream", "event_id", event.Message.ExternalMessageID)
			if err := svc.IngestInbound(ctx, event); err != nil {
				logger.Error("failed to ingest inbound message", "error", err)
				return err // Keep in stream pending queue
			}
			return nil
		})
		return consumerResult(groupCtx, "messages.inbound", err)
	})
	group.Go(func() error {
		logger.Info("starting Redis stream consumer for messages.external_outbound")
		err := psClient.Consume(groupCtx, "messages.external_outbound", "conversation-svc-group", "conversation-svc-external-outbound-consumer", func(ctx context.Context, id string, payload []byte) error {
			var event types.ExternalOutboundEvent
			if err := json.Unmarshal(payload, &event); err != nil {
				logger.Error("failed to unmarshal external outbound event", "error", err)
				return nil
			}

			logger.Info("processing external outbound message from Redis stream", "event_id", event.ExternalMessageID)
			if err := svc.IngestExternalOutbound(ctx, event); err != nil {
				logger.Error("failed to ingest external outbound message", "error", err)
				return err
			}
			return nil
		})
		return consumerResult(groupCtx, "messages.external_outbound", err)
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
