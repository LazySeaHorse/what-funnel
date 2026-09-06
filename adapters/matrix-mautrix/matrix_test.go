package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
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

func TestAdapter_StartJoinsTrustedInviteAndRetriesBeforePublishing(t *testing.T) {
	const (
		channelID     = "whatsapp-channel"
		roomID        = "!customer:localhost"
		channelUserID = "@wf_whatsapp:localhost"
		bridgeUserID  = "@whatsappbot:localhost"
	)

	var syncRequests atomic.Int32
	var joinRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authorization := r.Header.Get("Authorization"); authorization != "Bearer matrix-token" {
			t.Errorf("Authorization = %q, want Bearer matrix-token", authorization)
		}

		switch {
		case strings.HasSuffix(r.URL.Path, "/sync"):
			switch syncRequests.Add(1) {
			case 1:
				_ = json.NewEncoder(w).Encode(map[string]any{
					"next_batch": "after-invite",
					"rooms": map[string]any{"invite": map[string]any{
						roomID: map[string]any{"invite_state": map[string]any{"events": []map[string]any{{
							"type":      "m.room.member",
							"sender":    bridgeUserID,
							"state_key": channelUserID,
							"content":   map[string]any{"membership": "invite"},
						}}}},
					}},
				})
			case 2:
				if since := r.URL.Query().Get("since"); since != "after-invite" {
					t.Errorf("second sync since = %q, want after-invite", since)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"next_batch": "after-retry",
					"rooms":      map[string]any{},
				})
			case 3:
				if since := r.URL.Query().Get("since"); since != "after-retry" {
					t.Errorf("third sync since = %q, want after-retry", since)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"next_batch": "after-message",
					"rooms": map[string]any{"join": map[string]any{
						roomID: map[string]any{"timeline": map[string]any{"events": []map[string]any{{
							"type":             "m.room.message",
							"sender":           "@whatsapp_customer:localhost",
							"event_id":         "$first-message",
							"origin_server_ts": int64(1_788_622_527_000),
							"content":          map[string]any{"msgtype": "m.text", "body": "hello"},
						}}}},
					}},
				})
			default:
				<-r.Context().Done()
			}
		case strings.HasSuffix(r.URL.Path, "/join"):
			if r.Method != http.MethodPost {
				t.Errorf("join method = %s, want POST", r.Method)
			}
			if request := joinRequests.Add(1); request == 1 {
				http.Error(w, "temporary failure", http.StatusBadGateway)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"room_id": roomID})
		default:
			t.Errorf("unexpected Matrix request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter := New()
	adapter.Configure(channelID, ChannelConfig{
		Credentials: Credentials{
			HomeserverURL: server.URL,
			UserID:        channelUserID,
			AccessToken:   "matrix-token",
		},
		BridgeIdentity: bridgeUserID,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	published := make(chan types.InboundEvent, 1)
	done := make(chan error, 1)
	go func() {
		done <- adapter.Start(ctx, func(event types.InboundEvent) {
			published <- event
		}, func(types.ExternalOutboundEvent) {})
	}()

	select {
	case event := <-published:
		assert.Equal(t, channelID, event.ChannelID)
		assert.Equal(t, roomID, event.ExternalThreadID)
		assert.Equal(t, "$first-message", event.Message.ExternalMessageID)
		assert.Equal(t, "hello", event.Message.Text)
	case <-time.After(time.Second):
		t.Fatal("trusted room message was not published")
	}

	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("adapter did not stop after cancellation")
	}
	assert.Equal(t, int32(2), joinRequests.Load())
	assert.GreaterOrEqual(t, syncRequests.Load(), int32(3))
}

func TestAdapter_StartIgnoresUntrustedInvite(t *testing.T) {
	const roomID = "!untrusted:localhost"

	var syncRequests atomic.Int32
	var joinRequests atomic.Int32
	secondSync := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/join") {
			joinRequests.Add(1)
			http.Error(w, "unexpected join", http.StatusInternalServerError)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/sync") {
			http.NotFound(w, r)
			return
		}

		if syncRequests.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"next_batch": "after-untrusted-invite",
				"rooms": map[string]any{"invite": map[string]any{
					roomID: map[string]any{"invite_state": map[string]any{"events": []map[string]any{{
						"type":      "m.room.member",
						"sender":    "@attacker:localhost",
						"state_key": "@wf_whatsapp:localhost",
						"content":   map[string]any{"membership": "invite"},
					}}}},
				}},
			})
			return
		}

		close(secondSync)
		<-r.Context().Done()
	}))
	defer server.Close()

	adapter := New()
	adapter.Configure("whatsapp-channel", ChannelConfig{
		Credentials: Credentials{
			HomeserverURL: server.URL,
			UserID:        "@wf_whatsapp:localhost",
			AccessToken:   "matrix-token",
		},
		BridgeIdentity: "@whatsappbot:localhost",
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- adapter.Start(ctx, func(types.InboundEvent) {}, func(types.ExternalOutboundEvent) {})
	}()

	select {
	case <-secondSync:
	case <-time.After(time.Second):
		t.Fatal("adapter did not continue syncing after untrusted invite")
	}
	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("adapter did not stop after cancellation")
	}
	assert.Zero(t, joinRequests.Load())
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
