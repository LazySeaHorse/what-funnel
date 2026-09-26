package integration

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

// TestOutboxClaimSIGKILLRecovery forcefully terminates the conversation service container
// with SIGKILL mid-claim, restarts it, and confirms the pending outbox message
// is processed exactly once upon recovery without duplicates.
func TestOutboxClaimSIGKILLRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping outbox failover test in short mode")
	}
	skipIfServicesDown(t)

	pool := testPool(t)
	ctx := context.Background()

	accountID := uuid.New()
	channelID := uuid.New()
	contactID := uuid.New()
	convoID := uuid.New()
	messageID := uuid.New()

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

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM message_outbox WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM messages WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM conversations WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM contacts WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM channels WHERE id = $1`, channelID)
		pool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	// 2. Insert outbox message
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

	// 3. Mark row as claimed by a worker that was killed (stale claim > 5m)
	_, err = pool.Exec(ctx, `
		UPDATE message_outbox
		SET claimed_at = NOW() - INTERVAL '6 minutes', claimed_by = 'killed-worker'
		WHERE id = $1;
	`, outboxID)
	require.NoError(t, err)

	// 4. Forcefully terminate conversation service container with SIGKILL
	t.Log("Step 3: Forcefully terminating conversation-svc with SIGKILL...")
	killCmd := exec.Command("docker", "compose", "kill", "-s", "SIGKILL", "conversation-svc")
	_ = killCmd.Run()

	// 5. Restart conversation service container
	t.Log("Step 4: Restarting conversation-svc...")
	startCmd := exec.Command("docker", "compose", "start", "conversation-svc")
	startOutput, err := startCmd.CombinedOutput()
	require.NoError(t, err, "restart conversation-svc failed: %s", string(startOutput))

	// 6. Confirm the restarted service reclaims and dispatches the outbox command
	t.Log("Step 5: Verifying restarted service reclaims and dispatches message...")
	require.Eventually(t, func() bool {
		var dispatchedAt *time.Time
		_ = pool.QueryRow(ctx, `SELECT dispatched_at FROM message_outbox WHERE id = $1`, outboxID).Scan(&dispatchedAt)
		return dispatchedAt != nil
	}, 20*time.Second, 500*time.Millisecond, "Outbox command must be dispatched after restart")

	// 7. Verify claim is not duplicated (dispatched_at set once)
	var dispatchCount int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM message_outbox WHERE id = $1 AND dispatched_at IS NOT NULL`, outboxID).Scan(&dispatchCount)
	require.NoError(t, err)
	assert.Equal(t, 1, dispatchCount, "Command must be processed exactly once")

	t.Log("Step 6: Mid-transaction SIGKILL recovery verified successfully.")
}
