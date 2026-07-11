package service_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fakeadapter "github.com/whatfunnel/whatfunnel/adapters/fake"
	"github.com/whatfunnel/whatfunnel/packages/go-common/crypto"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

const testEncryptionKey = "test-key-exactly-32-bytes-padded"

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

func testService(t *testing.T) (*service.Service, *pgxpool.Pool, *pubsub.Client) {
	t.Helper()
	pool := testPool(t)

	redisAddr := os.Getenv("REDIS_URL")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	ps, err := pubsub.NewClient(redisAddr)
	require.NoError(t, err)

	cipher, err := crypto.NewCipherFromHex(testEncryptionKey)
	require.NoError(t, err)

	svc := service.New(pool, cipher, ps)
	t.Cleanup(func() {
		ps.Close()
	})
	return svc, pool, ps
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

func TestIngestInbound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, ps := testService(t)
	ctx := context.Background()

	accountID, _ := setupTestTenant(t, pool, "ingest-test")

	// Create a channel
	var channelID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, status)
		VALUES ($1, 'matrix_whatsapp', 'connected') RETURNING id
	`, accountID).Scan(&channelID)
	require.NoError(t, err)

	// Clean up Redis stream
	ps.RawClient().Del(ctx, "conversation.updated")

	// 1. Ingest a message for a new contact (new contact + new conversation)
	event1 := types.InboundEvent{
		ChannelID:        channelID.String(),
		ExternalThreadID: "whatsapp-thread-1",
		Contact: types.ContactRef{
			ExternalIdentity: "12345@s.whatsapp.net",
			DisplayName:      "Bob",
			AvatarURL:        "http://avatar.url/bob",
		},
		Message: types.NormalizedMessage{
			ContentType:       "text",
			Text:              "Hello business!",
			ExternalMessageID: "msg-id-1",
		},
		Timestamp: time.Now(),
	}

	err = svc.IngestInbound(ctx, event1)
	require.NoError(t, err)

	// Verify DB state
	var contactID uuid.UUID
	var displayName, avatarURL string
	err = pool.QueryRow(ctx, `SELECT id, display_name, avatar_url FROM contacts WHERE channel_id = $1 AND external_identity = $2`, channelID, "12345@s.whatsapp.net").
		Scan(&contactID, &displayName, &avatarURL)
	require.NoError(t, err)
	assert.Equal(t, "Bob", displayName)
	assert.Equal(t, "http://avatar.url/bob", avatarURL)

	var convoID uuid.UUID
	var convoStatus string
	err = pool.QueryRow(ctx, `SELECT id, status FROM conversations WHERE contact_id = $1`, contactID).
		Scan(&convoID, &convoStatus)
	require.NoError(t, err)
	assert.Equal(t, "open", convoStatus)

	var msgCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id = $1`, convoID).Scan(&msgCount)
	require.NoError(t, err)
	assert.Equal(t, 1, msgCount)

	// Check Redis event published
	streams, err := ps.RawClient().XRead(ctx, &redis.XReadArgs{
		Streams: []string{"conversation.updated", "0"},
		Count:   1,
	}).Result()
	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.Len(t, streams[0].Messages, 1)

	// 2. Ingest duplicate message (idempotency check)
	err = svc.IngestInbound(ctx, event1)
	require.NoError(t, err) // Should skip without failing

	err = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id = $1`, convoID).Scan(&msgCount)
	require.NoError(t, err)
	assert.Equal(t, 1, msgCount, "message count should still be 1 (deduplicated)")

	// 3. Second message from same contact (reuses conversation)
	event2 := types.InboundEvent{
		ChannelID:        channelID.String(),
		ExternalThreadID: "whatsapp-thread-1",
		Contact: types.ContactRef{
			ExternalIdentity: "12345@s.whatsapp.net",
			DisplayName:      "Bob Modified",
			AvatarURL:        "", // empty should not overwrite
		},
		Message: types.NormalizedMessage{
			ContentType:       "image",
			Text:              "Look at this!",
			MediaURL:          "http://media.url/image.jpg",
			ExternalMessageID: "msg-id-2",
		},
		Timestamp: time.Now(),
	}

	err = svc.IngestInbound(ctx, event2)
	require.NoError(t, err)

	err = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id = $1`, convoID).Scan(&msgCount)
	require.NoError(t, err)
	assert.Equal(t, 2, msgCount)

	// Display name updated, avatar retained
	err = pool.QueryRow(ctx, `SELECT display_name, avatar_url FROM contacts WHERE id = $1`, contactID).
		Scan(&displayName, &avatarURL)
	require.NoError(t, err)
	assert.Equal(t, "Bob Modified", displayName)
	assert.Equal(t, "http://avatar.url/bob", avatarURL)

	// 4. Test all content types normalize correctly
	contentTypes := []string{"text", "image", "video", "audio", "document", "reaction", "location", "contact"}
	for i, ct := range contentTypes {
		event := types.InboundEvent{
			ChannelID:        channelID.String(),
			ExternalThreadID: "whatsapp-thread-1",
			Contact: types.ContactRef{
				ExternalIdentity: "12345@s.whatsapp.net",
			},
			Message: types.NormalizedMessage{
				ContentType:       ct,
				Text:              "test " + ct,
				ExternalMessageID: "msg-ct-" + ct,
			},
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		}
		err = svc.IngestInbound(ctx, event)
		require.NoError(t, err)
	}

	// Verify count is 2 + 8 = 10
	err = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id = $1`, convoID).Scan(&msgCount)
	require.NoError(t, err)
	assert.Equal(t, 10, msgCount)
}

func TestSendMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	ctx := context.Background()

	accountID, userID := setupTestTenant(t, pool, "send-test")

	// Create a channel
	var channelID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, status)
		VALUES ($1, 'matrix_whatsapp', 'connected') RETURNING id
	`, accountID).Scan(&channelID)
	require.NoError(t, err)

	// Register fake adapter for WhatsApp
	fakeAdapter := fakeadapter.New()
	svc.RegisterAdapter("matrix_whatsapp", fakeAdapter)

	// Create a contact
	var contactID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
		VALUES ($1, $2, '98765@s.whatsapp.net', 'Alice') RETURNING id
	`, accountID, channelID).Scan(&contactID)
	require.NoError(t, err)

	// Create a conversation
	var convoID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO conversations (account_id, contact_id, channel_id)
		VALUES ($1, $2, $3) RETURNING id
	`, accountID, contactID, channelID).Scan(&convoID)
	require.NoError(t, err)

	// Send outbound message
	msg, err := svc.SendMessage(ctx, accountID, convoID, "human", &userID, "text", "Hello Alice from agent", "")
	require.NoError(t, err)
	require.NotNil(t, msg)

	// Assert message stored in DB
	var dbDirection, dbSenderType, dbContentType string
	err = pool.QueryRow(ctx, `SELECT direction, sender_type, content_type FROM messages WHERE id = $1`, msg.ID).
		Scan(&dbDirection, &dbSenderType, &dbContentType)
	require.NoError(t, err)
	assert.Equal(t, "outbound", dbDirection)
	assert.Equal(t, "human", dbSenderType)
	assert.Equal(t, "text", dbContentType)

	// Assert adapter SendMessage was called
	sent := fakeAdapter.GetSentMessages()
	require.Len(t, sent, 1)
	assert.Equal(t, channelID.String(), sent[0].ChannelID)
	assert.Equal(t, "98765@s.whatsapp.net", sent[0].ExternalThreadID)
	assert.Equal(t, "Hello Alice from agent", sent[0].Message.Text)
}
