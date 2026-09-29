package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
)

// TestRedisPauseResumeFailover pauses Redis mid-traffic, resumes it,
// and ensures the consumers reconnect and events are processed without drops
// or duplicates.
//
// DESTRUCTIVE: pauses the shared dev stack's Redis. Opt in with
// WHATFUNNEL_DESTRUCTIVE_TESTS=1 (`make test-destructive`).
func TestRedisPauseResumeFailover(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping redis failover test in short mode")
	}
	requireDestructive(t)
	skipIfServicesDown(t)

	pool := testPool(t)
	ctx := context.Background()

	// Register recovery/cleanup BEFORE anything that can fail so Redis is never
	// left paused and no account leaks. Unpausing a running container is a no-op error.
	adminEmail := uniqueEmail("redis-pause-mgr")
	t.Cleanup(func() { cleanupAccountByEmail(t, adminEmail) })
	t.Cleanup(func() { _, _ = composeCmd(t, "unpause", "redis") })

	adminClient := newClient()
	regResp, body := post(t, adminClient, gatewayURL+"/auth/signup", map[string]string{
		"account_name": "Redis Pause Co",
		"email":        adminEmail,
		"password":     "AdminPassword123!",
	})
	require.Equal(t, 201, regResp.StatusCode)
	accountID := uuid.MustParse(body["account_id"].(string))

	channelID := createTestProviderChannel(t, pool, accountID, "Redis Pause Channel")

	ps, err := pubsub.NewClient("localhost:6379")
	require.NoError(t, err)
	defer ps.Close()

	countMessages := func() int {
		var count int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE account_id = $1`, accountID).Scan(&count)
		return count
	}

	// 1. Send pre-pause message
	t.Log("Step 1: Sending message before pausing Redis...")
	publishTestProviderMessage(t, ps, channelID, "+15551111111", "customer1", "Customer 1", "Hello pre-pause", "msg_pre_1", messaging.DirectionInbound)
	require.Eventually(t, func() bool { return countMessages() == 1 }, 10*time.Second, 200*time.Millisecond, "Pre-pause message must be processed")

	// 2. Pause Redis and prove it is actually unreachable (the outage is real).
	t.Log("Step 2: Pausing Redis container...")
	out, err := composeCmd(t, "pause", "redis")
	require.NoError(t, err, "pause redis: %s", string(out))
	require.Eventually(t, func() bool { return !redisReachable("localhost:6379") },
		10*time.Second, 200*time.Millisecond, "Redis must become unreachable while paused")

	// 3. Resume Redis and wait until it answers again.
	t.Log("Step 3: Resuming Redis container...")
	out, err = composeCmd(t, "unpause", "redis")
	require.NoError(t, err, "unpause redis: %s", string(out))
	require.Eventually(t, func() bool { return redisReachable("localhost:6379") },
		15*time.Second, 200*time.Millisecond, "Redis must answer again after unpause")

	psPost, err := pubsub.NewClient("localhost:6379")
	require.NoError(t, err)
	defer psPost.Close()

	// 4. Send post-resume message; the consumers must have reconnected to ingest it.
	t.Log("Step 4: Sending message after resuming Redis...")
	publishTestProviderMessage(t, psPost, channelID, "+15552222222", "customer2", "Customer 2", "Hello post-resume", "msg_post_2", messaging.DirectionInbound)

	// 5. Both messages persisted exactly once (no drops, no duplicates).
	require.Eventually(t, func() bool { return countMessages() == 2 },
		30*time.Second, 300*time.Millisecond, "Both messages must be ingested after redis unpause")
	assert.Never(t, func() bool { return countMessages() != 2 },
		2*time.Second, 200*time.Millisecond, "message count must stay at exactly 2 (no redelivery duplicates)")

	for _, providerID := range []string{"msg_pre_1", "msg_post_2"} {
		var n int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT count(*) FROM messages WHERE account_id = $1 AND provider_message_id = $2`, accountID, providerID).Scan(&n))
		assert.Equal(t, 1, n, "provider message %s must be stored exactly once", providerID)
	}
}
