package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRoundTripEntity tests creating and retrieving primary entities
// (contacts, conversations, messages) to verify exact data shape and field equality.
func TestRoundTripEntity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testPool(t)
	ctx := context.Background()

	// 1. Setup isolated Account and Channel
	var accountID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO accounts (name, settings)
		VALUES ('Round Trip Account', '{"timezone":"UTC","notifications":true}'::jsonb)
		RETURNING id;
	`).Scan(&accountID)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM accounts WHERE id = $1", accountID)
	})

	var channelID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, provider, label, status)
		VALUES ($1, 'webchat', 'webchat', 'Test WebChat', 'connected')
		RETURNING id;
	`, accountID).Scan(&channelID)
	require.NoError(t, err)

	// 2. Round-trip Contact test
	t.Run("Contact round-trip data shape and field equality", func(t *testing.T) {
		extIdentity := "usr_" + uuid.NewString()
		displayName := "Alice Roundtrip"
		avatarURL := "https://example.com/avatar.png"

		var insertedID uuid.UUID
		var createdAt time.Time
		err := pool.QueryRow(ctx, `
			INSERT INTO contacts (account_id, channel_id, external_identity, display_name, avatar_url)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, created_at;
		`, accountID, channelID, extIdentity, displayName, avatarURL).Scan(&insertedID, &createdAt)
		require.NoError(t, err)
		assert.False(t, insertedID == uuid.Nil)
		assert.False(t, createdAt.IsZero())

		// Retrieve and assert exact field equality
		var retrievedAccountID, retrievedChannelID uuid.UUID
		var retrievedIdentity, retrievedDisplayName, retrievedAvatar string
		var retrievedCreatedAt time.Time
		err = pool.QueryRow(ctx, `
			SELECT account_id, channel_id, external_identity, display_name, avatar_url, created_at
			FROM contacts
			WHERE id = $1
		`, insertedID).Scan(
			&retrievedAccountID,
			&retrievedChannelID,
			&retrievedIdentity,
			&retrievedDisplayName,
			&retrievedAvatar,
			&retrievedCreatedAt,
		)
		require.NoError(t, err)
		assert.Equal(t, accountID, retrievedAccountID)
		assert.Equal(t, channelID, retrievedChannelID)
		assert.Equal(t, extIdentity, retrievedIdentity)
		assert.Equal(t, displayName, retrievedDisplayName)
		assert.Equal(t, avatarURL, retrievedAvatar)
		assert.WithinDuration(t, createdAt, retrievedCreatedAt, time.Second)

		// Update and verify
		updatedName := "Alice Wonder"
		_, err = pool.Exec(ctx, "UPDATE contacts SET display_name = $1 WHERE id = $2", updatedName, insertedID)
		require.NoError(t, err)

		var verifyName string
		err = pool.QueryRow(ctx, "SELECT display_name FROM contacts WHERE id = $1", insertedID).Scan(&verifyName)
		require.NoError(t, err)
		assert.Equal(t, updatedName, verifyName)
	})

	// 3. Round-trip Conversation test
	var contactID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
		VALUES ($1, $2, $3, 'Bob Convo')
		RETURNING id;
	`, accountID, channelID, "convo_user_"+uuid.NewString()).Scan(&contactID)
	require.NoError(t, err)

	var convoID uuid.UUID
	t.Run("Conversation round-trip data shape and field equality", func(t *testing.T) {
		extThreadID := "thread_" + uuid.NewString()
		status := "open"

		var createdAt time.Time
		err := pool.QueryRow(ctx, `
			INSERT INTO conversations (account_id, contact_id, channel_id, status, external_thread_id)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, created_at;
		`, accountID, contactID, channelID, status, extThreadID).Scan(&convoID, &createdAt)
		require.NoError(t, err)
		assert.False(t, convoID == uuid.Nil)
		assert.False(t, createdAt.IsZero())

		// Retrieve and assert exact field equality
		var rAccountID, rContactID, rChannelID uuid.UUID
		var rStatus, rThreadID string
		var rCreatedAt time.Time
		var rAssignedUsers []uuid.UUID
		err = pool.QueryRow(ctx, `
			SELECT account_id, contact_id, channel_id, status, external_thread_id, assigned_user_ids, created_at
			FROM conversations
			WHERE id = $1
		`, convoID).Scan(
			&rAccountID,
			&rContactID,
			&rChannelID,
			&rStatus,
			&rThreadID,
			&rAssignedUsers,
			&rCreatedAt,
		)
		require.NoError(t, err)
		assert.Equal(t, accountID, rAccountID)
		assert.Equal(t, contactID, rContactID)
		assert.Equal(t, channelID, rChannelID)
		assert.Equal(t, status, rStatus)
		assert.Equal(t, extThreadID, rThreadID)
		assert.Empty(t, rAssignedUsers)
		assert.WithinDuration(t, createdAt, rCreatedAt, time.Second)

		// Update status to closed and verify
		_, err = pool.Exec(ctx, "UPDATE conversations SET status = 'closed' WHERE id = $1", convoID)
		require.NoError(t, err)

		var verifyStatus string
		err = pool.QueryRow(ctx, "SELECT status FROM conversations WHERE id = $1", convoID).Scan(&verifyStatus)
		require.NoError(t, err)
		assert.Equal(t, "closed", verifyStatus)
	})

	// 4. Round-trip Message test
	t.Run("Message round-trip data shape and field equality", func(t *testing.T) {
		require.False(t, convoID == uuid.Nil, "convoID must exist")

		msgContent := map[string]any{
			"text": "Hello, this is a verified roundtrip message!",
			"meta": map[string]any{"client": "web", "version": 2.0},
		}
		contentJSON, err := json.Marshal(msgContent)
		require.NoError(t, err)

		direction := "inbound"
		senderType := "contact"
		contentType := "text"
		deliveryStatus := "delivered"
		idempotencyKey := "idem_" + uuid.NewString()
		providerMsgID := "prov_" + uuid.NewString()

		var msgID uuid.UUID
		var createdAt time.Time
		err = pool.QueryRow(ctx, `
			INSERT INTO messages (
				account_id, conversation_id, direction, sender_type,
				content_type, content, delivery_status, idempotency_key, provider_message_id
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, created_at;
		`, accountID, convoID, direction, senderType, contentType, contentJSON, deliveryStatus, idempotencyKey, providerMsgID).Scan(&msgID, &createdAt)
		require.NoError(t, err)
		assert.False(t, msgID == uuid.Nil)
		assert.False(t, createdAt.IsZero())

		// Retrieve and assert exact field equality
		var rAccID, rConvoID uuid.UUID
		var rDir, rSender, rContentType, rDelivStatus, rIdemKey, rProvID string
		var rContentRaw []byte
		var rCreatedAt time.Time
		err = pool.QueryRow(ctx, `
			SELECT account_id, conversation_id, direction, sender_type, content_type,
			       content, delivery_status, idempotency_key, provider_message_id, created_at
			FROM messages
			WHERE id = $1
		`, msgID).Scan(
			&rAccID, &rConvoID, &rDir, &rSender, &rContentType,
			&rContentRaw, &rDelivStatus, &rIdemKey, &rProvID, &rCreatedAt,
		)
		require.NoError(t, err)
		assert.Equal(t, accountID, rAccID)
		assert.Equal(t, convoID, rConvoID)
		assert.Equal(t, direction, rDir)
		assert.Equal(t, senderType, rSender)
		assert.Equal(t, contentType, rContentType)
		assert.Equal(t, deliveryStatus, rDelivStatus)
		assert.Equal(t, idempotencyKey, rIdemKey)
		assert.Equal(t, providerMsgID, rProvID)
		assert.WithinDuration(t, createdAt, rCreatedAt, time.Second)

		var rContent map[string]any
		err = json.Unmarshal(rContentRaw, &rContent)
		require.NoError(t, err)
		assert.Equal(t, "Hello, this is a verified roundtrip message!", rContent["text"])

		// Update delivery_status to read and verify
		_, err = pool.Exec(ctx, "UPDATE messages SET delivery_status = 'read' WHERE id = $1", msgID)
		require.NoError(t, err)

		var verifyDeliv string
		err = pool.QueryRow(ctx, "SELECT delivery_status FROM messages WHERE id = $1", msgID).Scan(&verifyDeliv)
		require.NoError(t, err)
		assert.Equal(t, "read", verifyDeliv)
	})
}
