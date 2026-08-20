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

	// 3. Initialize Crypto Cipher
	cipher, err := crypto.NewCipherFromHex(cfg.EncryptionKey)
	if err != nil {
		logger.Error("failed to initialize cipher", "error", err)
		os.Exit(1)
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
	sess := session.New(pool, cfg.SessionSecret)

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

	// Start Matrix Adapter background process
	go func() {
		logger.Info("starting matrix adapter")
		err := matrixAdapter.Start(ctx, func(event types.InboundEvent) {
			logger.Info("received event from matrix adapter, publishing to redis", "channel_id", event.ChannelID, "event_id", event.Message.ExternalMessageID)
			if _, err := psClient.Publish(ctx, "messages.inbound", event); err != nil {
				logger.Error("failed to publish inbound event to Redis", "error", err)
			}
		}, func(event types.ExternalOutboundEvent) {
			logger.Info("received external outbound event from matrix adapter, publishing to redis", "channel_id", event.ChannelID, "event_id", event.ExternalMessageID)
			if _, err := psClient.Publish(ctx, "messages.external_outbound", event); err != nil {
				logger.Error("failed to publish external outbound event to Redis", "error", err)
			}
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("matrix adapter error", "error", err)
		}
	}()

	// 6. Start Redis Streams Inbound Ingestion Consumer
	go func() {
		logger.Info("starting Redis stream consumer for messages.inbound")
		err := psClient.Consume(ctx, "messages.inbound", "conversation-svc-group", "conversation-svc-consumer", func(ctx context.Context, id string, payload []byte) error {
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
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("redis stream consumer error", "error", err)
		}
	}()

	// Start Redis Streams External Outbound Ingestion Consumer
	go func() {
		logger.Info("starting Redis stream consumer for messages.external_outbound")
		err := psClient.Consume(ctx, "messages.external_outbound", "conversation-svc-group", "conversation-svc-external-outbound-consumer", func(ctx context.Context, id string, payload []byte) error {
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
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("redis stream external outbound consumer error", "error", err)
		}
	}()

	// 7. Initialize HTTP API Routes
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

	go func() {
		logger.Info("conversation-svc listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
	logger.Info("conversation-svc stopped")
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
