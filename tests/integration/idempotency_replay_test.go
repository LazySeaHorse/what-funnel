package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
)

// TestIdempotencyReplay tests delivering identical payloads twice to verify
// the duplicate is ignored and only one entry is written to the database.
func TestIdempotencyReplay(t *testing.T) {
	skipIfServicesDown(t)
	pool := testPool(t)
	ctx := context.Background()

	// 1. Setup isolated test user and conversation
	client := newClient()
	email := uniqueEmail("idempotent")
	password := "SecurePass123!"

	regResp, _ := post(t, client, gatewayURL+"/auth/signup", map[string]any{
		"account_name": "Idempotency Test Account",
		"email":        email,
		"password":     password,
	})
	require.Equal(t, http.StatusCreated, regResp.StatusCode)

	loginResp, _ := post(t, client, gatewayURL+"/auth/login", map[string]any{
		"email":    email,
		"password": password,
	})
	require.Equal(t, http.StatusOK, loginResp.StatusCode)

	var accountID uuid.UUID
	err := pool.QueryRow(ctx, "SELECT account_id FROM users WHERE email = $1", email).Scan(&accountID)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM accounts WHERE id = $1", accountID)
	})

	// Create test channel
	var channelID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, provider, label, status)
		VALUES ($1, 'whatsapp', 'whatsapp', 'WhatsApp Idem', 'connected')
		RETURNING id;
	`, accountID).Scan(&channelID)
	require.NoError(t, err)

	// Create contact and conversation
	var contactID, convoID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
		VALUES ($1, $2, $3, 'Idempotent Customer')
		RETURNING id;
	`, accountID, channelID, "user_"+uuid.NewString()).Scan(&contactID)
	require.NoError(t, err)

	err = pool.QueryRow(ctx, `
		INSERT INTO conversations (account_id, contact_id, channel_id, status)
		VALUES ($1, $2, $3, 'open')
		RETURNING id;
	`, accountID, contactID, channelID).Scan(&convoID)
	require.NoError(t, err)

	// Test Case 1: Outbound message API with Idempotency-Key
	t.Run("HTTP Send Message with duplicate idempotency key writes only once", func(t *testing.T) {
		idempotencyKey := "idem_key_" + uuid.NewString()
		payload := map[string]any{
			"content_type":    "text",
			"text":            "First attempt message",
			"idempotency_key": idempotencyKey,
		}

		sendURL := fmt.Sprintf("%s/conversations/%s/send", gatewayURL, convoID)

		// First request
		resp1, body1 := post(t, client, sendURL, payload)
		require.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp1.StatusCode, "First send should succeed: %v", body1)
		msgID1, ok := body1["id"].(string)
		require.True(t, ok && msgID1 != "", "First response should return message id")

		// Replay second request with same idempotency key and same conversation
		resp2, body2 := post(t, client, sendURL, payload)
		require.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp2.StatusCode, "Second send should succeed: %v", body2)
		msgID2, ok := body2["id"].(string)
		require.True(t, ok && msgID2 != "", "Second response should return message id")

		// Both should return the same message ID
		assert.Equal(t, msgID1, msgID2, "Replay must return original message ID")

		// Verify database has exactly 1 record for this idempotency key
		var count int
		err = pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM messages
			WHERE account_id = $1 AND idempotency_key = $2
		`, accountID, idempotencyKey).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "Exactly 1 record must exist in DB for this idempotency key")
	})

	// Test Case 2: Adapter provider event replay with duplicate provider_message_id
	t.Run("PubSub adapter event replay with identical provider_message_id writes only once", func(t *testing.T) {
		ps, err := pubsub.NewClient("localhost:6379")
		if err != nil {
			t.Skipf("skipping pubsub test: redis not reachable: %v", err)
		}
		defer ps.Close()

		providerMsgID := "prov_dup_" + uuid.NewString()
		threadID := "thread_" + uuid.NewString()

		publishTestProviderMessage(
			t, ps, channelID.String(), threadID,
			"sender_"+uuid.NewString(), "Sender Name",
			"Inbound duplicated text", providerMsgID,
			messaging.DirectionInbound,
		)

		// Wait briefly for conversation-svc to ingest
		require.Eventually(t, func() bool {
			var cnt int
			_ = pool.QueryRow(ctx, `
				SELECT COUNT(*) FROM messages
				WHERE account_id = $1 AND provider_message_id = $2
			`, accountID, providerMsgID).Scan(&cnt)
			return cnt == 1
		}, 5*time.Second, 200*time.Millisecond, "First event should be ingested")

		// Replay the exact same event
		publishTestProviderMessage(
			t, ps, channelID.String(), threadID,
			"sender_"+uuid.NewString(), "Sender Name",
			"Inbound duplicated text", providerMsgID,
			messaging.DirectionInbound,
		)

		time.Sleep(1 * time.Second)

		// Verify database STILL has exactly 1 record for this provider_message_id
		var finalCount int
		err = pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM messages
			WHERE account_id = $1 AND provider_message_id = $2
		`, accountID, providerMsgID).Scan(&finalCount)
		require.NoError(t, err)
		assert.Equal(t, 1, finalCount, "Replayed event must not create duplicate message record")
	})
}
