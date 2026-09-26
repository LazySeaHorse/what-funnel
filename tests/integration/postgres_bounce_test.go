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

// TestPostgresBounceRestart verifies that restarting PostgreSQL during active
// service writes allows automatic reconnection and zero data loss.
func TestPostgresBounceRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping postgres bounce test in short mode")
	}
	skipIfServicesDown(t)

	ctx := context.Background()
	pool := testPool(t)

	adminEmail := uniqueEmail("pg-bounce-admin")
	adminClient := newClient()

	// 1. Initial write before bounce
	t.Log("Step 1: Performing writes before PostgreSQL bounce...")
	regResp, body := post(t, adminClient, gatewayURL+"/auth/signup", map[string]string{
		"account_name": "PG Bounce Co",
		"email":        adminEmail,
		"password":     "AdminPassword123!",
	})
	require.Equal(t, 201, regResp.StatusCode)
	accountIDStr := body["account_id"].(string)
	accountID := uuid.MustParse(accountIDStr)

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
	t.Log("Step 2: Restarting PostgreSQL container (docker compose restart postgres)...")
	restartCmd := exec.Command("docker", "compose", "restart", "postgres")
	out, err := restartCmd.CombinedOutput()
	require.NoError(t, err, "restart postgres failed: %s", string(out))

	// 3. Wait for PostgreSQL to become ready
	t.Log("Step 3: Waiting for PostgreSQL pg_isready...")
	require.Eventually(t, func() bool {
		checkCmd := exec.Command("docker", "compose", "exec", "-T", "postgres", "pg_isready", "-U", "whatfunnel", "-d", "whatfunnel")
		return checkCmd.Run() == nil
	}, 20*time.Second, 500*time.Millisecond, "Postgres must become ready after bounce")

	// 4. Reconnect test pool and perform post-bounce write
	t.Log("Step 4: Reconnecting and performing post-bounce write...")
	postPool := testPool(t)
	defer postPool.Close()

	t.Cleanup(func() {
		postPool.Exec(ctx, `DELETE FROM sessions WHERE data::text LIKE '%'||$1||'%'`, accountIDStr)
		postPool.Exec(ctx, `DELETE FROM messages WHERE account_id = $1`, accountID)
		postPool.Exec(ctx, `DELETE FROM conversations WHERE account_id = $1`, accountID)
		postPool.Exec(ctx, `DELETE FROM channels WHERE account_id = $1`, accountID)
		postPool.Exec(ctx, `DELETE FROM users WHERE account_id = $1`, accountID)
		postPool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	publishTestProviderMessage(t, ps, channelID, "+15553334444", "user-2", "User 2", "Message post-bounce", "bounce_msg_2", messaging.DirectionInbound)

	// 5. Verify both pre-bounce and post-bounce writes exist
	t.Log("Step 5: Verifying data integrity post-restart...")
	require.Eventually(t, func() bool {
		var count int
		_ = postPool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE account_id = $1`, accountID).Scan(&count)
		return count >= 2
	}, 15*time.Second, 300*time.Millisecond, "Both messages must exist after postgres bounce")

	var total int
	err = postPool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE account_id = $1`, accountID).Scan(&total)
	require.NoError(t, err)
	assert.Equal(t, 2, total, "Zero data loss across postgres bounce")

	t.Log("Step 6: PostgreSQL bounce failover test passed!")
}
