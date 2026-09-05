package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	matrixadapter "github.com/whatfunnel/whatfunnel/adapters/matrix-mautrix"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

func TestStartBridgeConnectionPersistsAttemptBoundaryOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	var commandCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authorization := r.Header.Get("Authorization"); authorization != "Bearer matrix-token" {
			t.Errorf("Authorization = %q, want Bearer matrix-token", authorization)
		}
		switch r.Method {
		case http.MethodPut:
			commandCount.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]string{"event_id": "$current-login-command"})
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"chunk": []map[string]any{{
					"event_id": "$current-login-command",
					"sender":   "@setup:localhost",
					"content":  map[string]any{"body": "login qr"},
				}},
			})
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	svc, pool, _ := testService(t)
	accountID, _ := setupTestTenant(t, pool, "bridge-attempt-boundary")
	adapter := matrixadapter.New()
	svc.RegisterAdapter("matrix_whatsapp", adapter)
	svc.ConfigureBridgeConnections(service.BridgeConnectionConfig{
		Provisioning: matrixadapter.ProvisioningConfig{
			HomeserverURL:            server.URL,
			ServerName:               "localhost",
			RegistrationSharedSecret: "shared-secret",
		},
		BridgeIdentities: map[string]string{"whatsapp": "@whatsappbot:localhost"},
	})

	credentials, err := json.Marshal(matrixadapter.Credentials{
		HomeserverURL:    server.URL,
		UserID:           "@setup:localhost",
		AccessToken:      "matrix-token",
		ManagementRoomID: "!setup:localhost",
	})
	require.NoError(t, err)
	bridgeIdentity := "@whatsappbot:localhost"
	channel, err := svc.CreateChannel(context.Background(), accountID, "matrix_whatsapp", &bridgeIdentity, credentials)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `
		INSERT INTO channel_connections (
			channel_id, account_id, platform, bridge_identity, management_room_id, state, detail
		) VALUES ($1, $2, 'whatsapp', $3, '!setup:localhost', 'failed', 'old timeout')
	`, channel.ID, accountID, bridgeIdentity)
	require.NoError(t, err)

	results := make(chan string, 2)
	errors := make(chan error, 2)
	var callers sync.WaitGroup
	for range 2 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			connection, startErr := svc.StartBridgeConnection(context.Background(), accountID, "whatsapp")
			if startErr != nil {
				errors <- startErr
				return
			}
			results <- connection.LastEventID
		}()
	}
	callers.Wait()
	close(results)
	close(errors)

	for startErr := range errors {
		require.NoError(t, startErr)
	}
	for eventID := range results {
		require.Equal(t, "$current-login-command", eventID)
	}
	require.Equal(t, int32(1), commandCount.Load())

	var state, detail, lastEventID, channelStatus string
	err = pool.QueryRow(context.Background(), `
		SELECT cc.state, cc.detail, cc.last_event_id, c.status
		FROM channel_connections cc
		JOIN channels c ON c.id = cc.channel_id
		WHERE cc.channel_id = $1
	`, channel.ID).Scan(&state, &detail, &lastEventID, &channelStatus)
	require.NoError(t, err)
	require.Equal(t, "awaiting_scan", state)
	require.Contains(t, detail, "Scan the QR code")
	require.Equal(t, "$current-login-command", lastEventID)
	require.Equal(t, "pending", channelStatus)
}

func TestBridgeRefreshCannotOverwriteNewerAttempt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	requestStarted := make(chan struct{})
	releaseResponse := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseResponse
		_ = json.NewEncoder(w).Encode(map[string]any{
			"chunk": []map[string]any{
				{"event_id": "$attempt-1-timeout", "sender": "@whatsappbot:localhost", "content": map[string]any{"body": "Login failed: timed out"}},
				{"event_id": "$attempt-1", "sender": "@setup:localhost", "content": map[string]any{"body": "login qr"}},
			},
		})
	}))
	defer server.Close()

	svc, pool, _ := testService(t)
	accountID, _ := setupTestTenant(t, pool, "bridge-refresh-cas")
	adapter := matrixadapter.New()
	svc.RegisterAdapter("matrix_whatsapp", adapter)

	credentials, err := json.Marshal(matrixadapter.Credentials{
		HomeserverURL:    server.URL,
		UserID:           "@setup:localhost",
		AccessToken:      "matrix-token",
		ManagementRoomID: "!setup:localhost",
	})
	require.NoError(t, err)
	bridgeIdentity := "@whatsappbot:localhost"
	channel, err := svc.CreateChannel(context.Background(), accountID, "matrix_whatsapp", &bridgeIdentity, credentials)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `
		INSERT INTO channel_connections (
			channel_id, account_id, platform, bridge_identity, management_room_id,
			state, detail, last_event_id
		) VALUES ($1, $2, 'whatsapp', $3, '!setup:localhost', 'awaiting_scan', 'attempt one', '$attempt-1')
	`, channel.ID, accountID, bridgeIdentity)
	require.NoError(t, err)

	type refreshResult struct {
		connectionState  string
		connectionDetail string
		lastEventID      string
		err              error
	}
	result := make(chan refreshResult, 1)
	go func() {
		connection, refreshErr := svc.GetBridgeConnection(context.Background(), accountID, channel.ID, true)
		if refreshErr != nil {
			result <- refreshResult{err: refreshErr}
			return
		}
		result <- refreshResult{
			connectionState:  connection.State,
			connectionDetail: connection.Detail,
			lastEventID:      connection.LastEventID,
		}
	}()

	select {
	case <-requestStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("bridge refresh did not request management messages")
	}
	_, err = pool.Exec(context.Background(), `
		UPDATE channel_connections
		SET state = 'awaiting_scan', detail = 'attempt two', last_event_id = '$attempt-2'
		WHERE channel_id = $1
	`, channel.ID)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `UPDATE channels SET status = 'pending', status_detail = 'attempt two' WHERE id = $1`, channel.ID)
	require.NoError(t, err)
	close(releaseResponse)

	var refresh refreshResult
	select {
	case refresh = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("bridge refresh did not finish")
	}
	require.NoError(t, refresh.err)
	require.Equal(t, "awaiting_scan", refresh.connectionState)
	require.Equal(t, "attempt two", refresh.connectionDetail)
	require.Equal(t, "$attempt-2", refresh.lastEventID)

	var state, detail, lastEventID, channelStatus, channelDetail string
	err = pool.QueryRow(context.Background(), `
		SELECT cc.state, cc.detail, cc.last_event_id, c.status, c.status_detail
		FROM channel_connections cc
		JOIN channels c ON c.id = cc.channel_id
		WHERE cc.channel_id = $1
	`, channel.ID).Scan(&state, &detail, &lastEventID, &channelStatus, &channelDetail)
	require.NoError(t, err)
	require.Equal(t, "awaiting_scan", state)
	require.Equal(t, "attempt two", detail)
	require.Equal(t, "$attempt-2", lastEventID)
	require.Equal(t, "pending", channelStatus)
	require.Equal(t, "attempt two", channelDetail)
}
