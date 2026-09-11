package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
)

type recordingPublisher struct {
	mu     sync.Mutex
	events []messaging.Event
	notify chan struct{}
}

type flakyPublisher struct {
	mu       sync.Mutex
	attempts int
	notify   chan struct{}
}

func (p *flakyPublisher) Publish(context.Context, messaging.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.attempts++
	if p.attempts == 1 {
		return errors.New("redis unavailable")
	}
	select {
	case p.notify <- struct{}{}:
	default:
	}
	return nil
}

func (p *recordingPublisher) Publish(_ context.Context, event messaging.Event) error {
	p.mu.Lock()
	p.events = append(p.events, event)
	p.mu.Unlock()
	select {
	case p.notify <- struct{}{}:
	default:
	}
	return nil
}

func TestNewManager(t *testing.T) {
	publisher := &recordingPublisher{notify: make(chan struct{}, 1)}
	manager, err := NewManager(t.Context(), t.TempDir()+"/sessions/store.db", publisher, nil, nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	if _, err := manager.Snapshot("missing"); err != ErrNotFound {
		t.Fatalf("Snapshot() error = %v, want %v", err, ErrNotFound)
	}

	manager.setStatus(&clientSession{snapshot: Snapshot{ChannelID: "channel-1"}}, messaging.ConnectionConnected, "", "")
	select {
	case <-publisher.notify:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for status event")
	}

	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	if len(publisher.events) != 1 {
		t.Fatalf("published events = %d, want 1", len(publisher.events))
	}
	if publisher.events[0].Status.State != messaging.ConnectionConnected {
		t.Errorf("state = %q, want %q", publisher.events[0].Status.State, messaging.ConnectionConnected)
	}
}

func TestMarshalDownloadable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		downloadable any
		expectedKind string
		expectedErr  bool
	}{
		{name: "image", downloadable: &waE2E.ImageMessage{}, expectedKind: "image"},
		{name: "video", downloadable: &waE2E.VideoMessage{}, expectedKind: "video"},
		{name: "audio", downloadable: &waE2E.AudioMessage{}, expectedKind: "audio"},
		{name: "document", downloadable: &waE2E.DocumentMessage{}, expectedKind: "document"},
		{name: "unsupported", downloadable: nil, expectedErr: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			downloadable, _ := test.downloadable.(interface {
				GetDirectPath() string
				GetMediaKey() []byte
				GetFileSHA256() []byte
				GetFileEncSHA256() []byte
			})
			kind, _, err := marshalDownloadable(downloadable)
			if (err != nil) != test.expectedErr {
				t.Fatalf("marshalDownloadable() error = %v, expected error %v", err, test.expectedErr)
			}
			if kind != test.expectedKind {
				t.Errorf("kind = %q, want %q", kind, test.expectedKind)
			}
		})
	}
}

func TestManagerRetriesPersistedEvents(t *testing.T) {
	publisher := &flakyPublisher{notify: make(chan struct{}, 1)}
	manager, err := NewManager(t.Context(), t.TempDir()+"/sessions/store.db", publisher, nil, nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	manager.setStatus(&clientSession{snapshot: Snapshot{ChannelID: "channel-retry"}}, messaging.ConnectionConnected, "", "")
	select {
	case <-publisher.notify:
	case <-time.After(4 * time.Second):
		t.Fatal("timeout waiting for retried event")
	}
	publisher.mu.Lock()
	attempts := publisher.attempts
	publisher.mu.Unlock()
	if attempts != 2 {
		t.Errorf("publish attempts = %d, want 2", attempts)
	}
}
