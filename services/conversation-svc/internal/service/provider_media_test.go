package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

func TestIngestProviderEvent_MediaMessageStoresMediaIDInContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool, _ := testService(t)
	ctx := context.Background()
	accountID, _ := setupTestTenant(t, pool, "inbound-media")
	channelID := newTelegramChannel(t, pool, accountID, "media bot")
	cleanupProviderEvents(t, pool, channelID)

	now := time.Now().UTC().Truncate(time.Microsecond)
	event := providerCreatedEvent(channelID, "evt-"+uuid.NewString(), "700", "cust-media", now)
	event.Message.ContentType = messaging.ContentImage
	event.Message.Text = ""
	event.Message.Media = &messaging.Media{ProviderRef: "file-1", MIMEType: "image/png", SizeBytes: 1024}
	require.NoError(t, svc.IngestProviderEvent(ctx, event))

	// A redelivery under a new event ID is skipped and must not leave an
	// orphan media row behind.
	event.ID = "evt-" + uuid.NewString()
	require.NoError(t, svc.IngestProviderEvent(ctx, event))

	var mediaCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM media_objects WHERE account_id = $1`, accountID).Scan(&mediaCount))
	require.Equal(t, 1, mediaCount)

	var mediaID, contentMediaID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT id::text FROM media_objects WHERE account_id = $1`, accountID).Scan(&mediaID))
	require.NoError(t, pool.QueryRow(ctx, `SELECT content->>'media_id' FROM messages WHERE account_id = $1 AND provider_message_id = '700'`, accountID).Scan(&contentMediaID))
	require.Equal(t, mediaID, contentMediaID)
}
