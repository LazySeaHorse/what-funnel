package consumer_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/notification-svc/internal/consumer"
	"github.com/whatfunnel/whatfunnel/services/notification-svc/internal/server"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Skipf("skipping integration test: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func setupTestTenant(t *testing.T, pool *pgxpool.Pool, name string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	var accountID uuid.UUID
	err := pool.QueryRow(ctx, `INSERT INTO accounts (name) VALUES ($1) RETURNING id`, name).Scan(&accountID)
	require.NoError(t, err)

	var userID uuid.UUID
	err = pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin') RETURNING id`, accountID, name+"@example.com").Scan(&userID)
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

	return accountID, userID
}

func TestConsumer_PrivacyFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testPool(t)
	accountID, _ := setupTestTenant(t, pool, "ws-privacy")
	ctx := context.Background()

	// Create Member A and Member B users in the DB
	var memberAID, memberBID uuid.UUID
	err := pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, 'a@example.com', 'hash', 'member') RETURNING id`, accountID).Scan(&memberAID)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, 'b@example.com', 'hash', 'member') RETURNING id`, accountID).Scan(&memberBID)
	require.NoError(t, err)

	// Create channel, contact, and conversation assigned to Member A
	var channelID uuid.UUID
	err = pool.QueryRow(ctx, `INSERT INTO channels (account_id, type, status) VALUES ($1, 'matrix_whatsapp', 'connected') RETURNING id`, accountID).Scan(&channelID)
	require.NoError(t, err)

	var contactID uuid.UUID
	err = pool.QueryRow(ctx, `INSERT INTO contacts (account_id, channel_id, external_identity) VALUES ($1, $2, 'c1') RETURNING id`, accountID, channelID).Scan(&contactID)
	require.NoError(t, err)

	var convoID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO conversations (account_id, contact_id, channel_id, assigned_user_ids)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, accountID, contactID, channelID, []uuid.UUID{memberAID}).Scan(&convoID)
	require.NoError(t, err)

	// Insert message
	var msgID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content)
		VALUES ($1, $2, 'inbound', 'contact', 'text', '{"text": "private message"}') RETURNING id
	`, accountID, convoID).Scan(&msgID)
	require.NoError(t, err)

	// Update conversation last_message_at
	_, err = pool.Exec(ctx, `UPDATE conversations SET last_message_at = NOW() WHERE id = $1`, convoID)
	require.NoError(t, err)

	// Turn off unassigned_conversations_visible_to_members setting just in case, though this is assigned to A
	_, err = pool.Exec(ctx, `UPDATE accounts SET settings = '{"unassigned_conversations_visible_to_members": false}' WHERE id = $1`, accountID)
	require.NoError(t, err)

	// Initialize PubSub client
	redisAddr := os.Getenv("REDIS_URL")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	ps, err := pubsub.NewClient(redisAddr)
	require.NoError(t, err)
	defer ps.Close()

	// Setup Hub and logger
	logger := slog.Default()
	hub := server.NewHub(logger)
	go hub.Run(ctx)

	// Register two mock websocket clients: clientA (Member A) and clientB (Member B)
	clientA := &server.Client{
		UserID:    memberAID,
		AccountID: accountID,
		Role:      types.RoleMember,
		Send:      make(chan []byte, 10),
	}
	clientB := &server.Client{
		UserID:    memberBID,
		AccountID: accountID,
		Role:      types.RoleMember,
		Send:      make(chan []byte, 10),
	}

	hub.RegisterClient(clientA)
	hub.RegisterClient(clientB)

	// Create Consumer and invoke handler manually to test synchronously
	c := consumer.NewConsumer(pool, ps, hub, logger)

	payloadBytes, _ := json.Marshal(map[string]any{
		"account_id":      accountID,
		"conversation_id": convoID,
		"message_id":      msgID,
	})

	// Invoke handleConversationUpdated
	err = c.HandleConversationUpdatedForTest(ctx, "test-id", payloadBytes)
	require.NoError(t, err)

	// Assertions: Client A (assigned) should receive the message; Client B (not assigned) should not!
	select {
	case data := <-clientA.Send:
		var event server.WSMessageEvent
		err := json.Unmarshal(data, &event)
		require.NoError(t, err)
		assert.Equal(t, "message.received", event.Type)
		assert.Equal(t, convoID.String(), event.ConversationID)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Client A did not receive the websocket event")
	}

	select {
	case data := <-clientB.Send:
		t.Fatalf("Privacy leak! Client B received private event: %s", string(data))
	case <-time.After(500 * time.Millisecond):
		// Success! Client B did not receive the event.
	}
}
