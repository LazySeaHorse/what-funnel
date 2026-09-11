package session

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

type failingPublisher struct{}

func (failingPublisher) Publish(context.Context, messaging.Event) error {
	return errors.New("offline")
}

func TestStableMessageID(t *testing.T) {
	t.Parallel()

	first := stableMessageID("send:message-1")
	if first != stableMessageID("send:message-1") {
		t.Fatal("stableMessageID returned different IDs for the same command")
	}
	if first == stableMessageID("send:message-2") {
		t.Fatal("stableMessageID returned the same ID for distinct commands")
	}
	if !regexp.MustCompile(`^3EB0[0-9A-F]{18}$`).MatchString(first) {
		t.Fatalf("stableMessageID() = %q, want WhatsApp web message ID", first)
	}
}

func TestCompleteCommandPersistsEventAtomically(t *testing.T) {
	manager, err := NewManager(t.Context(), t.TempDir()+"/store.db", failingPublisher{}, nil, nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	now := time.Now().UTC()
	event := messaging.Event{
		SchemaVersion: messaging.SchemaVersion,
		ID:            "whatsapp:channel-1:message.created:provider-1",
		Kind:          messaging.EventMessageCreated,
		Provider:      messaging.ProviderWhatsApp,
		ChannelID:     "channel-1",
		OccurredAt:    now,
		Message: &messaging.Message{
			ProviderMessageID: "provider-1",
			ExternalThreadID:  "15551234567@s.whatsapp.net",
			Direction:         messaging.DirectionOutbound,
			Sender:            messaging.Sender{ExternalID: "self"},
			ContentType:       messaging.ContentText,
			Text:              "hello",
			ProviderTimestamp: now,
		},
	}
	if err := manager.completeCommandWithEvent(t.Context(), "command-1", "provider-1", event); err != nil {
		t.Fatalf("completeCommandWithEvent() error = %v", err)
	}

	var commandCount, eventCount int
	if err := manager.db.QueryRow(`SELECT COUNT(*) FROM adapter_commands WHERE command_id = 'command-1'`).Scan(&commandCount); err != nil {
		t.Fatalf("query command: %v", err)
	}
	if err := manager.db.QueryRow(`SELECT COUNT(*) FROM adapter_event_outbox WHERE event_id = ?`, event.ID).Scan(&eventCount); err != nil {
		t.Fatalf("query event: %v", err)
	}
	if commandCount != 1 || eventCount != 1 {
		t.Fatalf("persisted command/event = %d/%d, want 1/1", commandCount, eventCount)
	}
}

type fakeMessageClient struct {
	uploadResponse whatsmeow.UploadResponse
	uploadErr      error
}

func (f *fakeMessageClient) Upload(_ context.Context, _ []byte, _ whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
	return f.uploadResponse, f.uploadErr
}

func (f *fakeMessageClient) SendMessage(
	_ context.Context,
	_ types.JID,
	_ *waE2E.Message,
	_ ...whatsmeow.SendRequestExtra,
) (whatsmeow.SendResponse, error) {
	return whatsmeow.SendResponse{}, nil
}

type staticMediaSource struct {
	file MediaFile
	err  error
}

func (s staticMediaSource) Fetch(context.Context, string) (MediaFile, error) {
	return s.file, s.err
}

func TestBuildMessage(t *testing.T) {
	t.Parallel()

	client := &fakeMessageClient{uploadResponse: whatsmeow.UploadResponse{
		URL:           "https://example.invalid/media",
		DirectPath:    "/media",
		MediaKey:      []byte("key"),
		FileEncSHA256: []byte("encrypted"),
		FileSHA256:    []byte("plain"),
		FileLength:    3,
	}}
	tests := []struct {
		name          string
		message       messaging.Message
		mediaSource   MediaSource
		expectedText  string
		expectedImage bool
		expectedErr   error
	}{
		{
			name:         "plain text",
			message:      messaging.Message{ContentType: messaging.ContentText, Text: "hello"},
			expectedText: "hello",
		},
		{
			name: "reply text",
			message: messaging.Message{
				ContentType:       messaging.ContentText,
				Text:              "reply",
				ReplyToProviderID: "original-1",
			},
			expectedText: "reply",
		},
		{
			name: "image",
			message: messaging.Message{
				ContentType: messaging.ContentImage,
				Text:        "caption",
				Media:       &messaging.Media{ID: "media-1"},
			},
			mediaSource:   staticMediaSource{file: MediaFile{Data: []byte("abc"), MIMEType: "image/png"}},
			expectedImage: true,
		},
		{
			name: "oversized media",
			message: messaging.Message{
				ContentType: messaging.ContentImage,
				Media:       &messaging.Media{ID: "media-1"},
			},
			mediaSource: staticMediaSource{file: MediaFile{Data: make([]byte, messaging.MaxMediaBytes+1)}},
			expectedErr: messaging.ErrMediaTooLarge,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := buildMessage(t.Context(), client, test.mediaSource, test.message)
			if !errors.Is(err, test.expectedErr) {
				t.Fatalf("buildMessage() error = %v, want %v", err, test.expectedErr)
			}
			if err != nil {
				return
			}
			if test.expectedImage && result.GetImageMessage() == nil {
				t.Fatal("image message = nil")
			}
			if test.expectedText != "" {
				text := result.GetConversation()
				if result.GetExtendedTextMessage() != nil {
					text = result.GetExtendedTextMessage().GetText()
				}
				if text != test.expectedText {
					t.Errorf("text = %q, want %q", text, test.expectedText)
				}
			}
		})
	}
}

func TestNewHTTPMediaSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		secret  string
	}{
		{name: "relative url", baseURL: "/media", secret: "secret"},
		{name: "unsupported scheme", baseURL: "file:///tmp/media", secret: "secret"},
		{name: "empty secret", baseURL: "http://conversation-svc:8080"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewHTTPMediaSource(test.baseURL, test.secret); err == nil {
				t.Fatal("NewHTTPMediaSource() error = nil, want error")
			}
		})
	}
}
