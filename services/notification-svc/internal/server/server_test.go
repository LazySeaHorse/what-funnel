package server

import (
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestHub_RegisterClientAfterClose(t *testing.T) {
	hub := newTestHub()
	hub.Close()

	err := hub.RegisterClient(newTestClient())
	if !errors.Is(err, ErrHubClosed) {
		t.Fatalf("RegisterClient() error = %v, want %v", err, ErrHubClosed)
	}
}

func TestHub_UnregisterClientClosesSendOnce(t *testing.T) {
	hub := newTestHub()
	client := newTestClient()
	if err := hub.RegisterClient(client); err != nil {
		t.Fatalf("RegisterClient() error = %v", err)
	}

	hub.UnregisterClient(client)
	hub.UnregisterClient(client)

	if _, ok := <-client.Send; ok {
		t.Fatal("client send channel is open after unregister")
	}
}

func TestHub_CloseClosesRegisteredClients(t *testing.T) {
	hub := newTestHub()
	clients := []*Client{newTestClient(), newTestClient()}
	for _, client := range clients {
		if err := hub.RegisterClient(client); err != nil {
			t.Fatalf("RegisterClient() error = %v", err)
		}
	}

	hub.Close()
	hub.Close()

	for _, client := range clients {
		if _, ok := <-client.Send; ok {
			t.Fatal("client send channel is open after hub close")
		}
	}
}

func TestHub_BroadcastAndUnregisterConcurrently(t *testing.T) {
	hub := newTestHub()
	accountID := uuid.New()

	for range 100 {
		client := newTestClient()
		client.AccountID = accountID
		if err := hub.RegisterClient(client); err != nil {
			t.Fatalf("RegisterClient() error = %v", err)
		}

		var wg sync.WaitGroup
		wg.Go(func() {
			hub.BroadcastToAccount(accountID, map[string]string{"type": "test"}, nil)
		})
		wg.Go(func() {
			hub.UnregisterClient(client)
		})
		wg.Wait()
	}
}

func newTestHub() *Hub {
	return NewHub(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func newTestClient() *Client {
	return &Client{Send: make(chan []byte, 1)}
}
