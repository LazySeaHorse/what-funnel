package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
)

// Provider command stream the outbox dispatcher publishes WhatsApp commands to.
const commandStream = "adapter.commands.whatsapp"

// TestOutboxClaimSIGKILLRecovery SIGKILLs the conversation service container, leaves a
// stale claim (held by the "killed" worker) on a pending outbox command, restarts the
// service and confirms the command is reclaimed and published to the provider command
// stream exactly once.
//
// DESTRUCTIVE: kills a container of the shared dev stack. Opt in with
// WHATFUNNEL_DESTRUCTIVE_TESTS=1 (`make test-destructive`).
func TestOutboxClaimSIGKILLRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping outbox failover test in short mode")
	}
	requireDestructive(t)
	skipIfServicesDown(t)

	pool := testPool(t)
	ctx := context.Background()

	accountID := uuid.New()
	channelID := uuid.New()
	contactID := uuid.New()
	convoID := uuid.New()
	messageID := uuid.New()

	// Register cleanup and service recovery before anything can fail: the container
	// must never be left dead, and the seeded account (cascades) must not leak.
	t.Cleanup(func() { _, _ = composeCmd(t, "start", "conversation-svc") })
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, accountID) })

	// 1. Setup account, channel, contact, and conversation
	_, err := pool.Exec(ctx, `
		INSERT INTO accounts (id, name, settings)
		VALUES ($1, 'SIGKILL Failover Test Co', '{}')
		ON CONFLICT (id) DO NOTHING;
	`, accountID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO channels (id, account_id, type, provider, label, status)
		VALUES ($1, $2, 'whatsapp', 'whatsapp', 'Failover WA', 'connected')
		ON CONFLICT (id) DO NOTHING;
	`, channelID, accountID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO contacts (id, account_id, channel_id, external_identity, display_name)
		VALUES ($1, $2, $3, '+15559876543', 'Test User')
		ON CONFLICT (id) DO NOTHING;
	`, contactID, accountID, channelID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO conversations (id, account_id, channel_id, contact_id, status, external_thread_id)
		VALUES ($1, $2, $3, $4, 'open', '+15559876543')
		ON CONFLICT (id) DO NOTHING;
	`, convoID, accountID, channelID, contactID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO messages (id, account_id, conversation_id, direction, sender_type, content_type, content, delivery_status)
		VALUES ($1, $2, $3, 'outbound', 'human', 'text', '{"text":"SIGKILL failover test payload"}'::jsonb, 'queued')
		ON CONFLICT (id) DO NOTHING;
	`, messageID, accountID, convoID)
	require.NoError(t, err)

	// 2. Stop the service FIRST so nothing can claim the row before we stage the crash state.
	t.Log("Step 2: SIGKILLing conversation-svc...")
	out, err := composeCmd(t, "kill", "-s", "SIGKILL", "conversation-svc")
	require.NoError(t, err, "SIGKILL of conversation-svc must succeed: %s", string(out))
	require.Eventually(t, func() bool {
		resp, err := http.Get("http://localhost:8083/healthz")
		if err == nil {
			resp.Body.Close()
			return false
		}
		return true
	}, 15*time.Second, 200*time.Millisecond, "conversation-svc must be down after SIGKILL")

	// 3. Insert outbox message
	cmdID := "cmd:sigkill:" + uuid.NewString()
	cmd := messaging.Command{
		SchemaVersion: messaging.SchemaVersion,
		ID:            cmdID,
		Kind:          messaging.CommandSendMessage,
		Provider:      messaging.ProviderWhatsApp,
		ChannelID:     channelID.String(),
		CreatedAt:     time.Now().UTC(),
		MessageID:     messageID.String(),
		Message: &messaging.Message{
			ExternalThreadID: "+15559876543",
			Direction:        messaging.DirectionOutbound,
			Sender:           messaging.Sender{ExternalID: "agent-kill"},
			ContentType:      messaging.ContentText,
			Text:             "SIGKILL failover test payload",
			ProviderTimestamp: time.Now().UTC(),
		},
	}
	cmdBytes, err := json.Marshal(cmd)
	require.NoError(t, err)

	var outboxID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO message_outbox (account_id, channel_id, message_id, provider, command)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id;
	`, accountID, channelID, messageID, "whatsapp", cmdBytes).Scan(&outboxID)
	require.NoError(t, err)

	// 4. Stage the crash state: the row was claimed by a worker that died before publishing.
	_, err = pool.Exec(ctx, `
		UPDATE message_outbox
		SET claimed_at = NOW() - INTERVAL '6 minutes', claimed_by = 'killed-worker'
		WHERE id = $1;
	`, outboxID)
	require.NoError(t, err)

	ps, err := pubsub.NewClient("localhost:6379")
	require.NoError(t, err)
	defer ps.Close()
	published := func() int {
		entries, err := ps.RawClient().XRange(ctx, commandStream, "-", "+").Result()
		if err != nil {
			return -1
		}
		n := 0
		for _, entry := range entries {
			if payload, ok := entry.Values["payload"].(string); ok && strings.Contains(payload, cmdID) {
				n++
			}
		}
		return n
	}
	require.Equal(t, 0, published(), "command must not be published before the service restarts")

	// 5. Restart the service; its dispatcher must reclaim the stale row.
	t.Log("Step 5: Restarting conversation-svc...")
	out, err = composeCmd(t, "start", "conversation-svc")
	require.NoError(t, err, "restart conversation-svc failed: %s", string(out))

	// 6. The row is reclaimed by a new worker and marked dispatched...
	require.Eventually(t, func() bool {
		var dispatchedAt *time.Time
		_ = pool.QueryRow(ctx, `SELECT dispatched_at FROM message_outbox WHERE id = $1`, outboxID).Scan(&dispatchedAt)
		return dispatchedAt != nil
	}, 45*time.Second, 500*time.Millisecond, "Outbox command must be dispatched after restart")

	var claimedBy *string
	require.NoError(t, pool.QueryRow(ctx, `SELECT claimed_by FROM message_outbox WHERE id = $1`, outboxID).Scan(&claimedBy))
	require.NotNil(t, claimedBy)
	assert.NotEqual(t, "killed-worker", *claimedBy, "the stale claim must have been taken over by the restarted service")

	// 7. ...and the downstream side effect happened exactly once: one entry on the
	// provider command stream (a duplicate would be a duplicate WhatsApp send).
	require.Eventually(t, func() bool { return published() == 1 },
		10*time.Second, 200*time.Millisecond, "command must be published to %s", commandStream)
	assert.Never(t, func() bool { return published() != 1 },
		3*time.Second, 200*time.Millisecond, "command must be published exactly once, never re-dispatched")
}
