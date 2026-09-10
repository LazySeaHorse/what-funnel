package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	matrixadapter "github.com/whatfunnel/whatfunnel/adapters/matrix-mautrix"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

func TestDeleteChannelLogsOutBridgeAndDeletesAssociatedChats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logoutCommands := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if authorization := r.Header.Get("Authorization"); authorization != "Bearer matrix-token" {
			t.Errorf("Authorization = %q, want Bearer matrix-token", authorization)
		}
		var body struct {
			Command string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode management command: %v", err)
		}
		logoutCommands <- body.Command
		_ = json.NewEncoder(w).Encode(map[string]string{"event_id": "$logout"})
	}))
	defer server.Close()

	svc, pool, _ := testService(t)
	accountID, actorID := setupTestTenant(t, pool, "channel-delete-active-"+uuid.NewString())
	adapter := matrixadapter.New()
	svc.RegisterAdapter("matrix_whatsapp", adapter)

	credentials, err := json.Marshal(matrixadapter.Credentials{
		HomeserverURL:    server.URL,
		UserID:           "@channel:localhost",
		AccessToken:      "matrix-token",
		ManagementRoomID: "!management:localhost",
	})
	require.NoError(t, err)
	bridgeIdentity := "@whatsappbot:localhost"
	channel, err := svc.CreateChannel(context.Background(), accountID, "matrix_whatsapp", &bridgeIdentity, credentials)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `
		INSERT INTO channel_connections (
			channel_id, account_id, platform, bridge_identity, management_room_id, state
		) VALUES ($1, $2, 'whatsapp', $3, '!management:localhost', 'connected')
	`, channel.ID, accountID, bridgeIdentity)
	require.NoError(t, err)

	var contactID uuid.UUID
	err = pool.QueryRow(context.Background(), `
		INSERT INTO contacts (account_id, channel_id, external_identity)
		VALUES ($1, $2, 'customer') RETURNING id
	`, accountID, channel.ID).Scan(&contactID)
	require.NoError(t, err)
	var conversationID uuid.UUID
	err = pool.QueryRow(context.Background(), `
		INSERT INTO conversations (account_id, contact_id, channel_id)
		VALUES ($1, $2, $3) RETURNING id
	`, accountID, contactID, channel.ID).Scan(&conversationID)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `
		INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content)
		VALUES ($1, $2, 'inbound', 'contact', 'text', '{"text":"hello"}')
	`, accountID, conversationID)
	require.NoError(t, err)
	var pipelineID uuid.UUID
	err = pool.QueryRow(context.Background(), `SELECT id FROM lead_pipelines WHERE account_id = $1`, accountID).Scan(&pipelineID)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `
		INSERT INTO leads (account_id, conversation_id, pipeline_id, current_state_key)
		VALUES ($1, $2, $3, 'new')
	`, accountID, conversationID, pipelineID)
	require.NoError(t, err)

	require.NoError(t, svc.DeleteChannel(context.Background(), accountID, actorID, channel.ID))
	require.Equal(t, "logout", <-logoutCommands)

	for _, table := range []string{"channels", "channel_connections", "contacts", "conversations", "messages", "leads"} {
		var count int
		err = pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table+" WHERE account_id = $1", accountID).Scan(&count) //nolint:gosec // table names are fixed test data
		require.NoError(t, err)
		require.Zero(t, count, "%s rows were not deleted", table)
	}

	var auditActorID uuid.UUID
	var auditAction, targetType string
	err = pool.QueryRow(context.Background(), `
		SELECT actor_user_id, action, target_type
		FROM audit_logs
		WHERE account_id = $1 AND target_id = $2
	`, accountID, channel.ID).Scan(&auditActorID, &auditAction, &targetType)
	require.NoError(t, err)
	require.Equal(t, actorID, auditActorID)
	require.Equal(t, audit.ActionChannelDeleted, auditAction)
	require.Equal(t, audit.TargetChannel, targetType)
	require.Contains(t, adapter.Status(channel.ID.String()).Detail, "not configured")
}

func TestDeleteChannelDeletesPreviouslyDisconnectedBridgeWithoutCredentials(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	accountID, actorID := setupTestTenant(t, pool, "channel-delete-disconnected-"+uuid.NewString())
	channel, err := svc.CreateChannel(context.Background(), accountID, "matrix_whatsapp", nil, nil)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `UPDATE channels SET status = 'disconnected' WHERE id = $1`, channel.ID)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `
		INSERT INTO channel_connections (
			channel_id, account_id, platform, bridge_identity, management_room_id, state
		) VALUES ($1, $2, 'whatsapp', '@whatsappbot:localhost', '!management:localhost', 'cancelled')
	`, channel.ID, accountID)
	require.NoError(t, err)

	require.NoError(t, svc.DeleteChannel(context.Background(), accountID, actorID, channel.ID))
	err = pool.QueryRow(context.Background(), `SELECT id FROM channels WHERE id = $1`, channel.ID).Scan(new(uuid.UUID))
	require.ErrorIs(t, err, pgx.ErrNoRows)
	require.ErrorIs(t, svc.DeleteChannel(context.Background(), accountID, actorID, channel.ID), service.ErrChannelNotFound)
}
