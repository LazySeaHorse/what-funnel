package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fakeadapter "github.com/whatfunnel/whatfunnel/adapters/fake"
	"github.com/whatfunnel/whatfunnel/packages/go-common/crypto"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/handler"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

type mockSessionStore struct {
	userID    uuid.UUID
	accountID uuid.UUID
	role      string
	loggedIn  bool
}

func (m *mockSessionStore) GetUserID(r *http.Request) (uuid.UUID, bool) {
	return m.userID, m.loggedIn
}

func (m *mockSessionStore) GetAccountID(r *http.Request) (uuid.UUID, bool) {
	return m.accountID, m.loggedIn
}

func (m *mockSessionStore) GetRole(r *http.Request) (string, bool) {
	return m.role, m.loggedIn
}

func getTestDSN() string {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
	}
	return dsn
}

func getTestRedis() string {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}
	return redisURL
}

func TestChannelIngestionE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Database Connection
	pool, err := db.Connect(ctx, getTestDSN())
	require.NoError(t, err)
	defer pool.Close()

	// 2. Redis Connection
	ps, err := pubsub.NewClient(getTestRedis())
	require.NoError(t, err)
	defer ps.Close()

	// Clear test stream
	ps.RawClient().Del(ctx, "messages.inbound")
	ps.RawClient().Del(ctx, "conversation.updated")

	// 3. Setup Test Account and User
	var accountID uuid.UUID
	err = pool.QueryRow(ctx, `INSERT INTO accounts (name) VALUES ('Ingestion E2E Tenant') RETURNING id`).Scan(&accountID)
	require.NoError(t, err)

	var userID uuid.UUID
	userEmail := fmt.Sprintf("e2e-member-%s@example.com", uuid.New())
	err = pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, $2, 'pwd', 'agent') RETURNING id`, accountID, userEmail).Scan(&userID)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM messages WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM conversations WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM contacts WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM channels WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	// 4. Initialize Ingestion Service & Fake Adapter
	cipher, err := crypto.NewCipherFromHex("test-key-exactly-32-bytes-padded")
	require.NoError(t, err)

	svc := service.New(pool, cipher, ps)
	fakeAdapter := fakeadapter.New()
	svc.RegisterAdapter("matrix_whatsapp", fakeAdapter)

	// Create Channel
	ch, err := svc.CreateChannel(ctx, accountID, "matrix_whatsapp", nil, nil)
	require.NoError(t, err)

	// Start Adapter
	go func() {
		_ = fakeAdapter.Start(ctx, func(event types.InboundEvent) {
			_, _ = ps.Publish(ctx, "messages.inbound", event)
		}, func(event types.ExternalOutboundEvent) {
			_, _ = ps.Publish(ctx, "messages.external_outbound", event)
		})
	}()

	// Start Stream Consumer
	go func() {
		_ = ps.Consume(ctx, "messages.inbound", "e2e-group", "e2e-consumer", func(ctx context.Context, id string, payload []byte) error {
			var event types.InboundEvent
			if err := json.Unmarshal(payload, &event); err != nil {
				return nil
			}
			return svc.IngestInbound(ctx, event)
		})
	}()

	// Give a moment for the goroutines to start up and register callback/consumer group
	time.Sleep(100 * time.Millisecond)

	// 5. Simulate Inbound WhatsApp-shaped Event
	inboundEvent := types.InboundEvent{
		ChannelID:        ch.ID.String(),
		ExternalThreadID: "whatsapp-thread-999",
		Contact: types.ContactRef{
			ExternalIdentity: "999@s.whatsapp.net",
			DisplayName:      "E2E Contact",
		},
		Message: types.NormalizedMessage{
			ContentType:       "text",
			Text:              "Hello WhatFunnel Ingestion E2E",
			ExternalMessageID: "external-msg-999",
		},
		Timestamp: time.Now(),
	}

	fakeAdapter.SimulateInbound(inboundEvent)

	// Wait for consumer to process message
	time.Sleep(500 * time.Millisecond)

	// Verify Contact persisted
	var contactID uuid.UUID
	err = pool.QueryRow(ctx, `SELECT id FROM contacts WHERE channel_id = $1 AND external_identity = '999@s.whatsapp.net'`, ch.ID).Scan(&contactID)
	require.NoError(t, err, "contact must be persisted")

	// Verify Conversation persisted
	var convoID uuid.UUID
	err = pool.QueryRow(ctx, `SELECT id FROM conversations WHERE contact_id = $1`, contactID).Scan(&convoID)
	require.NoError(t, err, "conversation must be persisted")

	// Verify Message persisted
	var msgCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id = $1`, convoID).Scan(&msgCount)
	require.NoError(t, err)
	assert.Equal(t, 1, msgCount, "inbound message must be persisted")

	// 6. Test Outbound Send via HTTP API
	sessStore := &mockSessionStore{
		userID:    userID,
		accountID: accountID,
		role:      "agent",
		loggedIn:  true,
	}

	h := handler.New(svc, sessStore)
	r := mux.NewRouter()
	h.RegisterRoutes(r)

	sendBody := map[string]any{
		"content_type":   "text",
		"text":           "Replying back to contact",
		"sender_type":    "human",
		"sender_user_id": userID.String(),
	}
	sendJSON, _ := json.Marshal(sendBody)

	req, _ := http.NewRequest(http.MethodPost, "/internal/conversations/"+convoID.String()+"/send", bytes.NewBuffer(sendJSON))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	// Confirm message persisted in database as outbound
	var dbDirection, dbSenderType string
	err = pool.QueryRow(ctx, `SELECT direction, sender_type FROM messages WHERE conversation_id = $1 AND direction = 'outbound'`, convoID).
		Scan(&dbDirection, &dbSenderType)
	require.NoError(t, err)
	assert.Equal(t, "outbound", dbDirection)
	assert.Equal(t, "human", dbSenderType)

	// Confirm it reaches fake adapter
	sentMsgs := fakeAdapter.GetSentMessages()
	require.Len(t, sentMsgs, 1)
	assert.Equal(t, ch.ID.String(), sentMsgs[0].ChannelID)
	assert.Equal(t, "999@s.whatsapp.net", sentMsgs[0].ExternalThreadID)
	assert.Equal(t, "Replying back to contact", sentMsgs[0].Message.Text)
}
