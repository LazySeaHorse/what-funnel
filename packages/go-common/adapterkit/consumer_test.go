package adapterkit

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

type mockStreamConsumer struct {
	stream   string
	group    string
	consumer string
	handler  func(ctx context.Context, id string, payload []byte) error
	err      error
}

func (m *mockStreamConsumer) Consume(ctx context.Context, stream, group, consumer string, handler func(ctx context.Context, id string, payload []byte) error) error {
	m.stream = stream
	m.group = group
	m.consumer = consumer
	m.handler = handler
	return m.err
}

type recordSender struct {
	commands []messaging.Command
	err      error
}

func (r *recordSender) Send(_ context.Context, cmd messaging.Command) error {
	r.commands = append(r.commands, cmd)
	return r.err
}

func validTestCommand(provider messaging.Provider) messaging.Command {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	return messaging.Command{
		SchemaVersion: messaging.SchemaVersion,
		ID:            "cmd-1",
		Kind:          messaging.CommandSendMessage,
		Provider:      provider,
		ChannelID:     "channel-1",
		CreatedAt:     now,
		MessageID:     "msg-id-1",
		Message: &messaging.Message{
			ExternalThreadID:  "chat-1",
			Direction:         messaging.DirectionOutbound,
			Sender:            messaging.Sender{ExternalID: "business"},
			ContentType:       messaging.ContentText,
			Text:              "outbound text",
			ProviderTimestamp: now,
		},
	}
}

func TestNewCommandConsumer_Defaults(t *testing.T) {
	consumer := NewCommandConsumer(nil, nil, ConsumerConfig{
		Provider:     messaging.ProviderTelegram,
		ConsumerName: "worker-1",
	})

	if consumer.Stream() != "adapter.commands.telegram" {
		t.Errorf("stream = %q, want adapter.commands.telegram", consumer.Stream())
	}
	if consumer.Group() != "telegram-adapter" {
		t.Errorf("group = %q, want telegram-adapter", consumer.Group())
	}
	if consumer.ConsumerName() != "worker-1" {
		t.Errorf("consumer name = %q, want worker-1", consumer.ConsumerName())
	}
}

func TestNewCommandConsumer_CustomConfig(t *testing.T) {
	consumer := NewCommandConsumer(nil, nil, ConsumerConfig{
		Provider:     messaging.ProviderWhatsApp,
		ConsumerName: "worker-2",
		Stream:       "custom.commands.whatsapp",
		Group:        "custom-group",
	})

	if consumer.Stream() != "custom.commands.whatsapp" {
		t.Errorf("stream = %q, want custom.commands.whatsapp", consumer.Stream())
	}
	if consumer.Group() != "custom-group" {
		t.Errorf("group = %q, want custom-group", consumer.Group())
	}
}

func TestCommandConsumer_FiltersAndDispatches(t *testing.T) {
	mock := &mockStreamConsumer{}
	sender := &recordSender{}
	consumer := NewCommandConsumer(mock, sender, ConsumerConfig{
		Provider:     messaging.ProviderTelegram,
		ConsumerName: "worker-1",
	})

	ctx := context.Background()
	if err := consumer.Run(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.handler == nil {
		t.Fatal("handler was not registered")
	}

	// 1. Valid Telegram command -> should dispatch
	tgCmd := validTestCommand(messaging.ProviderTelegram)
	payload, _ := json.Marshal(tgCmd)
	if err := mock.handler(ctx, "msg-1", payload); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(sender.commands) != 1 || sender.commands[0].ID != "cmd-1" {
		t.Fatalf("command not dispatched as expected: %+v", sender.commands)
	}

	// 2. WhatsApp command -> should be filtered out
	waCmd := validTestCommand(messaging.ProviderWhatsApp)
	payloadWA, _ := json.Marshal(waCmd)
	if err := mock.handler(ctx, "msg-2", payloadWA); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(sender.commands) != 1 {
		t.Fatalf("wrong provider command was not filtered: len = %d", len(sender.commands))
	}

	// 3. Malformed JSON -> ignored without error
	if err := mock.handler(ctx, "msg-3", []byte("invalid-json")); err != nil {
		t.Errorf("expected malformed JSON to be ignored, got: %v", err)
	}
	if len(sender.commands) != 1 {
		t.Fatalf("malformed command triggered dispatch")
	}

	// 4. Invalid command (fails Validate()) -> ignored without error
	invalidCmd := messaging.Command{SchemaVersion: messaging.SchemaVersion, ID: "empty-provider"}
	payloadInv, _ := json.Marshal(invalidCmd)
	if err := mock.handler(ctx, "msg-4", payloadInv); err != nil {
		t.Errorf("expected invalid command to be ignored, got: %v", err)
	}
	if len(sender.commands) != 1 {
		t.Fatalf("invalid command triggered dispatch")
	}
}

func TestCommandConsumer_ConsumeError(t *testing.T) {
	mock := &mockStreamConsumer{err: errors.New("connection failed")}
	sender := &recordSender{}
	consumer := NewCommandConsumer(mock, sender, ConsumerConfig{
		Provider:     messaging.ProviderTelegram,
		ConsumerName: "worker-1",
	})

	if err := consumer.Run(context.Background()); err == nil {
		t.Fatal("expected error from Run(), got nil")
	}
}
