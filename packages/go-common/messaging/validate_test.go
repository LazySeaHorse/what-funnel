package messaging

import (
	"errors"
	"testing"
	"time"
)

func TestEventValidate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	valid := Event{
		SchemaVersion: SchemaVersion,
		ID:            "event-1",
		Kind:          EventMessageCreated,
		Provider:      ProviderWhatsApp,
		ChannelID:     "channel-1",
		OccurredAt:    now,
		Message: &Message{
			ProviderMessageID: "provider-message-1",
			ExternalThreadID:  "15551234567@s.whatsapp.net",
			Direction:         DirectionInbound,
			Sender:            Sender{ExternalID: "15551234567@s.whatsapp.net"},
			ContentType:       ContentText,
			Text:              "hello",
			ProviderTimestamp: now,
		},
	}

	tests := []struct {
		name    string
		mutate  func(*Event)
		wantErr error
	}{
		{name: "valid message", mutate: func(*Event) {}},
		{name: "unknown schema", mutate: func(event *Event) { event.SchemaVersion = 2 }, wantErr: ErrInvalidEnvelope},
		{name: "unknown provider", mutate: func(event *Event) { event.Provider = "unknown" }, wantErr: ErrInvalidEnvelope},
		{name: "wrong payload", mutate: func(event *Event) { event.Status = &Status{State: ConnectionConnected} }, wantErr: ErrInvalidEnvelope},
		{name: "missing payload", mutate: func(event *Event) { event.Message = nil }, wantErr: ErrInvalidEnvelope},
		{name: "empty text", mutate: func(event *Event) { event.Message.Text = "" }, wantErr: ErrInvalidEnvelope},
		{
			name: "oversized media",
			mutate: func(event *Event) {
				event.Message.ContentType = ContentImage
				event.Message.Text = ""
				event.Message.Media = &Media{ProviderRef: "opaque", SizeBytes: MaxMediaBytes + 1}
			},
			wantErr: ErrMediaTooLarge,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := valid
			message := *valid.Message
			got.Message = &message
			test.mutate(&got)

			err := got.Validate()
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestCommandValidate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	command := Command{
		SchemaVersion: SchemaVersion,
		ID:            "command-1",
		Kind:          CommandSendMessage,
		Provider:      ProviderWhatsApp,
		ChannelID:     "channel-1",
		CreatedAt:     now,
		MessageID:     "message-1",
		Message: &Message{
			ExternalThreadID:  "15551234567@s.whatsapp.net",
			Direction:         DirectionOutbound,
			Sender:            Sender{ExternalID: "business"},
			ContentType:       ContentText,
			Text:              "hello",
			ProviderTimestamp: now,
		},
	}

	if err := command.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	command.Reaction = &Reaction{
		ProviderMessageID: "provider-message-1",
		SenderExternalID:  "business",
		Emoji:             "👍",
		ProviderTimestamp: now,
	}
	if err := command.Validate(); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidEnvelope)
	}
}

func TestRetentionForSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		sizeBytes         int64
		expectedRetention time.Duration
		expectedError     error
	}{
		{name: "empty", sizeBytes: 0, expectedRetention: 7 * 24 * time.Hour},
		{name: "one mib", sizeBytes: 1024 * 1024, expectedRetention: 7 * 24 * time.Hour},
		{name: "ten mib", sizeBytes: 10 * 1024 * 1024, expectedRetention: 24 * time.Hour},
		{name: "twenty mib", sizeBytes: MaxMediaBytes, expectedRetention: time.Hour},
		{name: "over limit", sizeBytes: MaxMediaBytes + 1, expectedError: ErrMediaTooLarge},
		{name: "negative", sizeBytes: -1, expectedError: ErrInvalidEnvelope},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			retention, err := RetentionForSize(test.sizeBytes)
			if !errors.Is(err, test.expectedError) {
				t.Fatalf("RetentionForSize() error = %v, want %v", err, test.expectedError)
			}
			if retention != test.expectedRetention {
				t.Errorf("RetentionForSize() = %v, want %v", retention, test.expectedRetention)
			}
		})
	}
}
