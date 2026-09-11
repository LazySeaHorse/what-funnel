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

	"github.com/whatfunnel/whatfunnel/adapters/telegram-botapi/internal/botapi"
	"github.com/whatfunnel/whatfunnel/adapters/telegram-botapi/internal/commandstream"
	adapterconfig "github.com/whatfunnel/whatfunnel/adapters/telegram-botapi/internal/config"
	"github.com/whatfunnel/whatfunnel/adapters/telegram-botapi/internal/control"
	"github.com/whatfunnel/whatfunnel/adapters/telegram-botapi/internal/eventstream"
	"github.com/whatfunnel/whatfunnel/adapters/telegram-botapi/internal/session"
	wfcrypto "github.com/whatfunnel/whatfunnel/packages/go-common/crypto"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"golang.org/x/sync/errgroup"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("telegram adapter stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	config, err := adapterconfig.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	streamClient, err := pubsub.NewClient(config.RedisURL)
	if err != nil {
		return fmt.Errorf("connect redis: %w", err)
	}
	defer streamClient.Close()
	cipher, err := wfcrypto.NewCipherFromHex(config.EncryptionKey)
	if err != nil {
		return fmt.Errorf("configure credential encryption: %w", err)
	}
	api, err := botapi.New(config.BotAPIBaseURL, nil)
	if err != nil {
		return fmt.Errorf("configure telegram api: %w", err)
	}
	manager, err := session.NewManager(ctx, config.DatabasePath, cipher, api, eventstream.NewPublisher(streamClient), logger)
	if err != nil {
		return fmt.Errorf("initialize telegram sessions: %w", err)
	}
	defer manager.Close()
	mediaSource, err := session.NewHTTPMediaSource(config.ConversationInternalURL, config.SharedSecret)
	if err != nil {
		return fmt.Errorf("configure outbound media: %w", err)
	}
	manager.SetMediaSource(mediaSource)
	handler, err := control.NewHandler(manager, config.SharedSecret)
	if err != nil {
		return fmt.Errorf("initialize control api: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/", handler.Routes())
	server := &http.Server{Addr: ":" + config.Port, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 70 * time.Second, IdleTimeout: 60 * time.Second}
	consumer := commandstream.NewConsumer(streamClient, manager, config.ConsumerName)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		if err := consumer.Run(groupCtx); err != nil && groupCtx.Err() == nil {
			return err
		}
		return nil
	})
	group.Go(func() error {
		logger.Info("telegram adapter listening", "port", config.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve control api: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		<-groupCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	})
	return group.Wait()
}
