package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

type blockingTransport struct {
	started chan struct{}
	release chan struct{}
}

func (t *blockingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	close(t.started)
	<-t.release
	return nil, errors.New("request released")
}

func TestAdapter_StartWaitsForDynamicallyConfiguredChannels(t *testing.T) {
	transport := &blockingTransport{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	adapter := New()
	adapter.client = &http.Client{Transport: transport}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- adapter.Start(ctx, func(types.InboundEvent) {}, func(types.ExternalOutboundEvent) {})
	}()

	deadline := time.Now().Add(time.Second)
	for {
		adapter.mu.RLock()
		started := adapter.ctx != nil
		adapter.mu.RUnlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Start() did not initialize adapter lifecycle")
		}
		runtime.Gosched()
	}

	adapter.Configure("dynamic-channel", ChannelConfig{
		Credentials: Credentials{
			HomeserverURL: "http://matrix.example",
			AccessToken:   "token",
		},
		BridgeIdentity: "@whatsappbot:matrix.example",
	})
	select {
	case <-transport.started:
	case <-time.After(time.Second):
		t.Fatal("dynamic sync loop did not start")
	}

	cancel()
	select {
	case err := <-done:
		t.Fatalf("Start() returned before dynamic sync loop stopped: %v", err)
	default:
	}

	close(transport.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Start() did not return after dynamic sync loop stopped")
	}
}

func TestWaitForManagementRoomReadyWaitsForBridgeJoin(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		membership := "invite"
		if requests > 1 {
			membership = "join"
		}
		assert.Equal(t, "Bearer matrix-token", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]string{"membership": membership})
	}))
	defer server.Close()

	adapter := New()
	err := adapter.WaitForManagementRoomReady(context.Background(), Credentials{
		HomeserverURL: server.URL,
		AccessToken:   "matrix-token",
	}, "!setup:localhost", "@whatsappbot:localhost")

	assert.NoError(t, err)
	assert.GreaterOrEqual(t, requests, 2)
}

func TestDownloadMediaUsesAuthenticatedClientMediaEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/_matrix/client/v1/media/download/localhost/qr-image", r.URL.Path)
		assert.Equal(t, "Bearer matrix-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-data"))
	}))
	defer server.Close()

	adapter := New()
	body, contentType, err := adapter.DownloadMedia(context.Background(), Credentials{
		HomeserverURL: server.URL,
		AccessToken:   "matrix-token",
	}, "mxc://localhost/qr-image")

	assert.NoError(t, err)
	assert.Equal(t, "image/png", contentType)
	assert.Equal(t, []byte("png-data"), body)
}

func TestReadManagementMessagesSinceStopsAtCommandBoundary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "b", r.URL.Query().Get("dir"))
		assert.Equal(t, "30", r.URL.Query().Get("limit"))
		assert.Empty(t, r.URL.Query().Get("from"))
		assert.Equal(t, "Bearer matrix-token", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"chunk": []map[string]any{
				{"event_id": "$new-qr", "sender": "@whatsappbot:localhost", "type": "m.room.message", "content": map[string]any{"body": "qr.png", "url": "mxc://localhost/new-qr"}},
				{"event_id": "$login-command", "sender": "@user:localhost", "type": "m.room.message", "content": map[string]any{"body": "login qr"}},
				{"event_id": "$old-timeout", "sender": "@whatsappbot:localhost", "type": "m.room.message", "content": map[string]any{"body": "Login failed: timed out"}},
			},
			"end": "older-page",
		})
	}))
	defer server.Close()

	adapter := New()
	messages, err := adapter.ReadManagementMessagesSince(context.Background(), Credentials{
		HomeserverURL: server.URL,
		AccessToken:   "matrix-token",
	}, "!setup:localhost", "$login-command")

	assert.NoError(t, err)
	if assert.Len(t, messages, 1) {
		assert.Equal(t, "$new-qr", messages[0].EventID)
		assert.Equal(t, "mxc://localhost/new-qr", messages[0].MediaURL)
	}
}

func TestReadManagementMessagesSincePaginatesToCommandBoundary(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Query().Get("from") {
		case "":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"chunk": []map[string]any{{"event_id": "$reply", "sender": "@whatsappbot:localhost", "content": map[string]any{"body": "Working"}}},
				"end":   "page-2",
			})
		case "page-2":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"chunk": []map[string]any{
					{"event_id": "$login-command", "sender": "@user:localhost", "content": map[string]any{"body": "login qr"}},
					{"event_id": "$stale", "sender": "@whatsappbot:localhost", "content": map[string]any{"body": "Login failed"}},
				},
				"end": "page-3",
			})
		default:
			t.Fatalf("unexpected pagination token %q", r.URL.Query().Get("from"))
		}
	}))
	defer server.Close()

	adapter := New()
	messages, err := adapter.ReadManagementMessagesSince(context.Background(), Credentials{
		HomeserverURL: server.URL,
		AccessToken:   "matrix-token",
	}, "!setup:localhost", "$login-command")

	assert.NoError(t, err)
	assert.Equal(t, 2, requests)
	if assert.Len(t, messages, 1) {
		assert.Equal(t, "$reply", messages[0].EventID)
	}
}

func TestReadManagementMessagesSinceRejectsMissingBoundary(t *testing.T) {
	adapter := New()
	_, err := adapter.ReadManagementMessagesSince(context.Background(), Credentials{}, "!setup:localhost", "")
	assert.EqualError(t, err, "management command event ID is required")
}

func TestReadManagementMessagesSinceFailsClosedWhenBoundaryIsAbsent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"chunk": []map[string]any{{
				"event_id": "$stale-timeout",
				"sender":   "@whatsappbot:localhost",
				"content":  map[string]any{"body": "Login failed: timed out"},
			}},
		})
	}))
	defer server.Close()

	adapter := New()
	messages, err := adapter.ReadManagementMessagesSince(context.Background(), Credentials{
		HomeserverURL: server.URL,
		AccessToken:   "matrix-token",
	}, "!setup:localhost", "$unknown-command")

	assert.Nil(t, messages)
	assert.EqualError(t, err, "management command event $unknown-command was not found in room history")
}

func TestRegistrationMACUsesSynapseSharedSecretFormat(t *testing.T) {
	assert.Equal(t,
		"2d26005ec6579eb9293b5aeb2a3eefd4427fa361",
		registrationMAC("registration-secret", "nonce-123", "wf_test", "password-123"),
	)
}

func TestMatrixAdapter_NormalizeEvent(t *testing.T) {
	adapter := New()

	// 1. Test Text Message Event
	content := map[string]any{
		"msgtype": "m.text",
		"body":    "Hello, world!",
	}
	ev := adapter.NormalizeEvent("chan-1", "!room:matrix.org", "$msg-1", "@alice:matrix.org", "m.room.message", 1620000000000, content)

	assert.Equal(t, "chan-1", ev.ChannelID)
	assert.Equal(t, "!room:matrix.org", ev.ExternalThreadID)
	assert.Equal(t, "@alice:matrix.org", ev.Contact.ExternalIdentity)
	assert.Equal(t, "text", ev.Message.ContentType)
	assert.Equal(t, "Hello, world!", ev.Message.Text)
	assert.Equal(t, "$msg-1", ev.Message.ExternalMessageID)
	assert.Equal(t, time.UnixMilli(1620000000000), ev.Timestamp)

	// 2. Test Image Message Event
	content = map[string]any{
		"msgtype": "m.image",
		"body":    "image.png",
		"url":     "mxc://matrix.org/abc",
	}
	ev = adapter.NormalizeEvent("chan-1", "!room:matrix.org", "$msg-2", "@alice:matrix.org", "m.room.message", 1620000000000, content)
	assert.Equal(t, "image", ev.Message.ContentType)
	assert.Equal(t, "mxc://matrix.org/abc", ev.Message.MediaURL)

	// 3. Test Reaction Event
	content = map[string]any{
		"m.relates_to": map[string]any{
			"rel_type": "m.annotation",
			"event_id": "$parent-id",
			"key":      "👍",
		},
	}
	ev = adapter.NormalizeEvent("chan-1", "!room:matrix.org", "$msg-3", "@alice:matrix.org", "m.reaction", 1620000000000, content)
	assert.Equal(t, "reaction", ev.Message.ContentType)
	assert.Equal(t, "👍", ev.Message.Text)
	assert.Equal(t, "$parent-id", ev.Message.ReplyToExternalID)

	// 4. Test WhatsApp Contact Card
	content = map[string]any{
		"vcard": "BEGIN:VCARD\nFN:Alice\nEND:VCARD",
	}
	ev = adapter.NormalizeEvent("chan-1", "!room:matrix.org", "$msg-4", "@alice:matrix.org", "net.maunium.whatsapp.contact", 1620000000000, content)
	assert.Equal(t, "contact", ev.Message.ContentType)
	assert.Equal(t, "BEGIN:VCARD\nFN:Alice\nEND:VCARD", ev.Message.Text)

	// 5. Test Fallback for unrecognized type
	content = map[string]any{
		"msgtype": "m.unsupported",
		"body":    "something",
	}
	ev = adapter.NormalizeEvent("chan-1", "!room:matrix.org", "$msg-5", "@alice:matrix.org", "m.room.message", 1620000000000, content)
	assert.Equal(t, "document", ev.Message.ContentType)
}
