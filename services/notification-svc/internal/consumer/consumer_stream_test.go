package consumer_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/notification-svc/internal/consumer"
	"github.com/whatfunnel/whatfunnel/services/notification-svc/internal/server"
)

func TestConsumer_AllStreams_StartAndDispatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testPool(t)
	accountID, adminID := setupTestTenant(t, pool, "ws-all-streams")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create Member User
	var memberID uuid.UUID
	err := pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, 'm@example.com', 'h', 'member') RETURNING id`, accountID).Scan(&memberID)
	require.NoError(t, err)

	// Create channel, contact, conversation assigned to member
	var channelID, contactID, convoID uuid.UUID
	err = pool.QueryRow(ctx, `INSERT INTO channels (account_id, type, status) VALUES ($1, 'matrix_whatsapp', 'connected') RETURNING id`, accountID).Scan(&channelID)
	require.NoError(t, err)

	err = pool.QueryRow(ctx, `INSERT INTO contacts (account_id, channel_id, external_identity) VALUES ($1, $2, 'c-stream') RETURNING id`, accountID, channelID).Scan(&contactID)
	require.NoError(t, err)

	err = pool.QueryRow(ctx, `
		INSERT INTO conversations (account_id, contact_id, channel_id, assigned_user_ids)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, accountID, contactID, channelID, []uuid.UUID{memberID}).Scan(&convoID)
	require.NoError(t, err)

	// Redis connection
	redisAddr := os.Getenv("REDIS_URL")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	ps, err := pubsub.NewClient(redisAddr)
	require.NoError(t, err)
	defer ps.Close()

	// Clean up redis streams before test
	ps.RawClient().Del(ctx, "lead.state_changed", "channel.status_changed")

	logger := slog.Default()
	hub := server.NewHub(logger)
	go hub.Run(ctx)

	// Register Admin and Member clients
	adminClient := &server.Client{
		UserID:    adminID,
		AccountID: accountID,
		Role:      types.RoleAdmin,
		Send:      make(chan []byte, 20),
	}
	memberClient := &server.Client{
		UserID:    memberID,
		AccountID: accountID,
		Role:      types.RoleMember,
		Send:      make(chan []byte, 20),
	}
	hub.RegisterClient(adminClient)
	hub.RegisterClient(memberClient)

	c := consumer.NewConsumer(pool, ps, hub, logger)
	// Start the table-driven consumer loop
	c.Start(ctx, "test-stream-consumer-"+uuid.New().String())

	// Give a moment for consumers to subscribe
	time.Sleep(100 * time.Millisecond)

	// 1. Publish lead.state_changed
	leadID := uuid.New()
	leadPayload := map[string]any{
		"type":            "lead.state_changed",
		"conversation_id": convoID,
		"lead_id":         leadID,
		"from_state":      "new",
		"to_state":        "contacted",
	}
	_, err = ps.Publish(ctx, "lead.state_changed", leadPayload)
	require.NoError(t, err)

	// Member (assigned) should receive lead.state_changed
	select {
	case data := <-memberClient.Send:
		var event map[string]any
		err := json.Unmarshal(data, &event)
		require.NoError(t, err)
		assert.Equal(t, "lead.state_changed", event["type"])
		assert.Equal(t, convoID.String(), event["conversation_id"])
		assert.Equal(t, "contacted", event["to_state"])
	case <-time.After(2 * time.Second):
		t.Fatal("Member client did not receive lead.state_changed event")
	}

	// Admin should also receive lead.state_changed
	select {
	case data := <-adminClient.Send:
		var event map[string]any
		err := json.Unmarshal(data, &event)
		require.NoError(t, err)
		assert.Equal(t, "lead.state_changed", event["type"])
	case <-time.After(2 * time.Second):
		t.Fatal("Admin client did not receive lead.state_changed event")
	}

	// 2. Publish channel.status_changed
	channelPayload := map[string]any{
		"account_id": accountID,
		"channel_id": channelID,
		"status":     "connected",
		"detail":     "All systems go",
	}
	_, err = ps.Publish(ctx, "channel.status_changed", channelPayload)
	require.NoError(t, err)

	select {
	case data := <-adminClient.Send:
		var event map[string]any
		err := json.Unmarshal(data, &event)
		require.NoError(t, err)
		assert.Equal(t, "channel.status_changed", event["type"])
		assert.Equal(t, channelID.String(), event["channel_id"])
		assert.Equal(t, "connected", event["status"])
	case <-time.After(2 * time.Second):
		t.Fatal("Admin client did not receive channel.status_changed event")
	}
}
