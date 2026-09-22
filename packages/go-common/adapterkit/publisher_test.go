package adapterkit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

type mockStreamPublisher struct {
	stream  string
	payload any
	err     error
}

func (m *mockStreamPublisher) Publish(_ context.Context, stream string, payload any) (string, error) {
	m.stream = stream
	m.payload = payload
	return "mock-id-123", m.err
}

func validTestEvent() messaging.Event {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	return messaging.Event{
		SchemaVersion: messaging.SchemaVersion,
		ID:            "event-1",
		Kind:          messaging.EventMessageCreated,
		Provider:      messaging.ProviderTelegram,
		ChannelID:     "channel-1",
		OccurredAt:    now,
		Message: &messaging.Message{
			ProviderMessageID: "p-msg-1",
			ExternalThreadID:  "chat-1",
			Direction:         messaging.DirectionInbound,
			Sender:            messaging.Sender{ExternalID: "user-1"},
			ContentType:       messaging.ContentText,
			Text:              "hello",
			ProviderTimestamp: now,
		},
	}
}

func TestEventPublisher_Success(t *testing.T) {
	mock := &mockStreamPublisher{}
	publisher := NewEventPublisher(mock)

	event := validTestEvent()
	if err := publisher.Publish(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.stream != DefaultEventsStream {
		t.Errorf("stream = %q, want %q", mock.stream, DefaultEventsStream)
	}
}

func TestEventPublisher_CustomStream(t *testing.T) {
	mock := &mockStreamPublisher{}
	publisher := NewEventPublisher(mock, "custom.events")

	event := validTestEvent()
	if err := publisher.Publish(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.stream != "custom.events" {
		t.Errorf("stream = %q, want custom.events", mock.stream)
	}
}

func TestEventPublisher_ValidationFailure(t *testing.T) {
	mock := &mockStreamPublisher{}
	publisher := NewEventPublisher(mock)

	// Missing required fields -> validation error
	invalidEvent := messaging.Event{}
	if err := publisher.Publish(context.Background(), invalidEvent); err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func TestEventPublisher_BackendError(t *testing.T) {
	mock := &mockStreamPublisher{err: errors.New("redis unavailable")}
	publisher := NewEventPublisher(mock)

	event := validTestEvent()
	if err := publisher.Publish(context.Background(), event); err == nil {
		t.Fatal("expected backend error, got nil")
	}
}
