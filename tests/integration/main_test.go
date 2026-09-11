package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
)

func createTestProviderChannel(t *testing.T, pool *pgxpool.Pool, accountID uuid.UUID, label string) string {
	t.Helper()
	var channelID uuid.UUID
	err := pool.QueryRow(t.Context(), `
		WITH channel AS (
			INSERT INTO channels (account_id, type, provider, label, status, capabilities)
			VALUES ($1, 'whatsapp', 'whatsapp', $2, 'connected', '{"media":true,"replies":true,"reactions":true,"edits":true,"deletes":true,"receipts":true}')
			RETURNING id
		), connection AS (
			INSERT INTO provider_connections (channel_id, account_id, provider, state)
			SELECT id, $1, 'whatsapp', 'connected' FROM channel
		)
		SELECT id FROM channel
	`, accountID, label).Scan(&channelID)
	if err != nil {
		t.Fatalf("create test provider channel: %v", err)
	}
	return channelID.String()
}

func publishTestProviderMessage(
	t *testing.T,
	ps *pubsub.Client,
	channelID, threadID, senderID, displayName, text, providerMessageID string,
	direction messaging.Direction,
) {
	t.Helper()
	now := time.Now().UTC()
	event := messaging.Event{
		SchemaVersion: messaging.SchemaVersion,
		ID:            "test:" + uuid.NewString(),
		Kind:          messaging.EventMessageCreated,
		Provider:      messaging.ProviderWhatsApp,
		ChannelID:     channelID,
		OccurredAt:    now,
		Message: &messaging.Message{
			ProviderMessageID: providerMessageID,
			ExternalThreadID:  threadID,
			Direction:         direction,
			Sender: messaging.Sender{
				ExternalID: senderID, DisplayName: displayName,
			},
			ContentType: messaging.ContentText, Text: text, ProviderTimestamp: now,
		},
	}
	if _, err := ps.Publish(t.Context(), "adapter.events", event); err != nil {
		t.Fatalf("publish test provider event: %v", err)
	}
}

func cleanupIntegrationTestData() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return
	}
	defer pool.Close()

	// Never touch foo@barr.com or account Foobarr
	_, _ = pool.Exec(ctx, `
		DELETE FROM accounts
		WHERE id NOT IN (
			SELECT account_id FROM users WHERE email = 'foo@barr.com'
		) AND (
			id IN (
				SELECT DISTINCT account_id FROM users
				WHERE email LIKE '%@example.com' OR email LIKE '%@e2e.local' OR email LIKE '%@local.test'
			)
			OR name LIKE 'E2E %'
			OR name LIKE 'TestTenant%'
		);

		DELETE FROM users
		WHERE (email LIKE '%@example.com' OR email LIKE '%@e2e.local' OR email LIKE '%@local.test')
		  AND email != 'foo@barr.com';
	`)
}

func TestMain(m *testing.M) {
	cleanupIntegrationTestData()
	code := m.Run()
	cleanupIntegrationTestData()
	os.Exit(code)
}
