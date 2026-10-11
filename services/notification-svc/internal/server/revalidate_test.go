package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
)

type mutableSession struct {
	mu        sync.Mutex
	userID    uuid.UUID
	accountID uuid.UUID
	role      string
	valid     bool
	lookups   int
}

func (m *mutableSession) GetSession(*http.Request) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lookups++
	if !m.valid {
		return nil, errors.New("no session")
	}
	return map[string]string{
		"user_id": m.userID.String(), "account_id": m.accountID.String(), "role": m.role,
	}, nil
}

func TestRevalidateClients_ClosesRevokedAndUpdatesRole(t *testing.T) {
	hub := newTestHub()
	sess := &mutableSession{userID: uuid.New(), accountID: uuid.New(), role: "agent", valid: true}
	srv := NewServer(hub, sess, hub.logger, nil, true)

	client := newTestClient()
	client.UserID, client.AccountID, client.Role = sess.userID, sess.accountID, "agent"
	client.sessionReq = httptest.NewRequest(http.MethodGet, "/ws", nil)
	if err := hub.RegisterClient(client); err != nil {
		t.Fatal(err)
	}

	// Role change is picked up without closing the socket.
	sess.mu.Lock()
	sess.role = "manager"
	sess.mu.Unlock()
	srv.RevalidateClients()
	if client.Role != "manager" {
		t.Fatalf("role = %q, want manager", client.Role)
	}
	if len(hub.snapshotClients()) != 1 {
		t.Fatal("client should remain connected")
	}

	// Revoked session closes the socket.
	sess.mu.Lock()
	sess.valid = false
	sess.mu.Unlock()
	srv.RevalidateClients()
	if len(hub.snapshotClients()) != 0 {
		t.Fatal("revoked client should be unregistered")
	}
	if _, ok := <-client.Send; ok {
		t.Fatal("send channel should be closed")
	}
}

func TestRevalidateClients_LoadsEachSessionOnce(t *testing.T) {
	hub := newTestHub()
	sess := &mutableSession{userID: uuid.New(), accountID: uuid.New(), role: "agent", valid: true}
	srv := NewServer(hub, sess, hub.logger, nil, true)

	client := newTestClient()
	client.UserID, client.AccountID, client.Role = sess.userID, sess.accountID, "agent"
	client.sessionReq = httptest.NewRequest(http.MethodGet, "/ws", nil)
	if err := hub.RegisterClient(client); err != nil {
		t.Fatal(err)
	}

	srv.RevalidateClients()
	if sess.lookups != 1 {
		t.Fatalf("session lookups = %d, want 1 per client", sess.lookups)
	}
}
