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

// TestPostgresBounceRestart verifies that restarting PostgreSQL during active
// service writes allows automatic reconnection and zero data loss.
//
// DESTRUCTIVE: restarts the shared dev stack's Postgres. Opt in with
// WHATFUNNEL_DESTRUCTIVE_TESTS=1 (`make test-destructive`).
func TestPostgresBounceRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping postgres bounce test in short mode")
	}
	requireDestructive(t)
	skipIfServicesDown(t)

	ctx := context.Background()
	pool := testPool(t)

	// Register cleanup before any fallible step. It opens its own connection
	// because the pool used below is dead after the restart.
	adminEmail := uniqueEmail("pg-bounce-admin")
	t.Cleanup(func() { cleanupAccountByEmail(t, adminEmail) })

	// 1. Initial write before bounce
	t.Log("Step 1: Performing writes before PostgreSQL bounce...")
	adminClient := newClient()
	regResp, body := post(t, adminClient, gatewayURL+"/auth/signup", map[string]string{
		"account_name": "PG Bounce Co",
		"email":        adminEmail,
		"password":     "AdminPassword123!",
	})
	require.Equal(t, 201, regResp.StatusCode)
	accountID := uuid.MustParse(body["account_id"].(string))

	channelID := createTestProviderChannel(t, pool, accountID, "PG Bounce WA")

	ps, err := pubsub.NewClient("localhost:6379")
	require.NoError(t, err)
	defer ps.Close()

	publishTestProviderMessage(t, ps, channelID, "+15551112222", "user-1", "User 1", "Message pre-bounce", "bounce_msg_1", messaging.DirectionInbound)

	require.Eventually(t, func() bool {
		var count int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE account_id = $1 AND provider_message_id = 'bounce_msg_1'`, accountID).Scan(&count)
		return count == 1
	}, 10*time.Second, 200*time.Millisecond, "Pre-bounce message must be persisted")

	// 2. Restart PostgreSQL container
	t.Log("Step 2: Restarting PostgreSQL container...")
	out, err := composeCmd(t, "restart", "postgres")
	require.NoError(t, err, "restart postgres failed: %s", string(out))

	// 3. Wait for PostgreSQL to become ready
	require.Eventually(t, func() bool {
		_, err := composeCmd(t, "exec", "-T", "postgres", "pg_isready", "-U", "whatfunnel", "-d", "whatfunnel")
		return err == nil
	}, 30*time.Second, 500*time.Millisecond, "Postgres must become ready after bounce")

	// 4. Post-bounce write must be ingested by the services (their pools must reconnect).
	t.Log("Step 4: Reconnecting and performing post-bounce write...")
	postPool := testPool(t)
	publishTestProviderMessage(t, ps, channelID, "+15553334444", "user-2", "User 2", "Message post-bounce", "bounce_msg_2", messaging.DirectionInbound)

	countMessages := func() int {
		var count int
		_ = postPool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE account_id = $1`, accountID).Scan(&count)
		return count
	}

	// 5. Verify both pre-bounce and post-bounce writes exist exactly once
	require.Eventually(t, func() bool { return countMessages() == 2 },
		45*time.Second, 300*time.Millisecond, "Both messages must exist after postgres bounce")
	assert.Never(t, func() bool { return countMessages() != 2 },
		2*time.Second, 200*time.Millisecond, "no message may be lost or duplicated across the bounce")
}
