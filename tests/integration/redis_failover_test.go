package integration

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
)

// TestRedisPauseResumeFailover pauses Redis mid-traffic, resumes it,
// and ensures connection recovers and events are processed without drops.
func TestRedisPauseResumeFailover(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping redis failover test in short mode")
	}
	skipIfServicesDown(t)

	pool := testPool(t)
	ctx := context.Background()

	adminEmail := uniqueEmail("redis-pause-mgr")
	adminClient := newClient()

	regResp, body := post(t, adminClient, gatewayURL+"/auth/signup", map[string]string{
		"account_name": "Redis Pause Co",
		"email":        adminEmail,
		"password":     "AdminPassword123!",
	})
	require.Equal(t, 201, regResp.StatusCode)
	accountIDStr := body["account_id"].(string)
	accountID := uuid.MustParse(accountIDStr)

	t.Cleanup(func() {
		// Ensure redis is unpaused in case test failed while paused
		_ = exec.Command("docker", "compose", "unpause", "redis").Run()
		pool.Exec(ctx, `DELETE FROM sessions WHERE data::text LIKE '%'||$1||'%'`, accountIDStr)
		pool.Exec(ctx, `DELETE FROM messages WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM conversations WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM channels WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM users WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	channelID := createTestProviderChannel(t, pool, accountID, "Redis Pause Channel")

	ps, err := pubsub.NewClient("localhost:6379")
	require.NoError(t, err)
	defer ps.Close()

	// 1. Send pre-pause message
	t.Log("Step 1: Sending message before pausing Redis...")
	publishTestProviderMessage(t, ps, channelID, "+15551111111", "customer1", "Customer 1", "Hello pre-pause", "msg_pre_1", messaging.DirectionInbound)

	require.Eventually(t, func() bool {
		var count int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE account_id = $1 AND provider_message_id = 'msg_pre_1'`, accountID).Scan(&count)
		return count == 1
	}, 10*time.Second, 200*time.Millisecond, "Pre-pause message must be processed")

	// 2. Pause Redis container
	t.Log("Step 2: Pausing Redis container...")
	pauseCmd := exec.Command("docker", "compose", "pause", "redis")
	out, err := pauseCmd.CombinedOutput()
	require.NoError(t, err, "pause redis: %s", string(out))

	// Wait 1 second while paused
	time.Sleep(1 * time.Second)

	// 3. Resume Redis container
	t.Log("Step 3: Resuming Redis container...")
	unpauseCmd := exec.Command("docker", "compose", "unpause", "redis")
	out, err = unpauseCmd.CombinedOutput()
	require.NoError(t, err, "unpause redis: %s", string(out))

	// Reconnect pubsub client if needed
	psPost, err := pubsub.NewClient("localhost:6379")
	require.NoError(t, err)
	defer psPost.Close()

	// 4. Send post-resume message
	t.Log("Step 4: Sending message after resuming Redis...")
	publishTestProviderMessage(t, psPost, channelID, "+15552222222", "customer2", "Customer 2", "Hello post-resume", "msg_post_2", messaging.DirectionInbound)

	// 5. Verify both messages are in the database (no dropped events)
	t.Log("Step 5: Verifying all messages persisted without drops...")
	require.Eventually(t, func() bool {
		var count int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE account_id = $1`, accountID).Scan(&count)
		return count >= 2
	}, 15*time.Second, 300*time.Millisecond, "Both messages must be ingested after redis unpause")

	var totalCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE account_id = $1`, accountID).Scan(&totalCount)
	require.NoError(t, err)
	assert.Equal(t, 2, totalCount, "Exact message count preserved with zero drops")

	t.Log("Step 6: Redis pause/resume failover verified successfully.")
}
