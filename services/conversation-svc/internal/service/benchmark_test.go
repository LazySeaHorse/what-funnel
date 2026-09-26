package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// BenchmarkMessagePreparation benchmarks message command construction and payload validation.
func BenchmarkMessagePreparation(b *testing.B) {
	accountID := uuid.New()
	conversationID := uuid.New()
	senderUserID := uuid.New()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cmd := sendMessageCommand{
			accountID:      accountID,
			conversationID: conversationID,
			sender:         types.MessageSenderHuman,
			senderUserID:   &senderUserID,
			contentType:    "text",
			text:           "Benchmark message content payload",
			idempotencyKey: "idem_bench_key",
		}
		if err := cmd.validate(); err != nil {
			b.Fatalf("validation failed: %v", err)
		}
	}
}

// BenchmarkOutboxSerialization benchmarks outbox payload marshaling and command parsing.
func BenchmarkOutboxSerialization(b *testing.B) {
	cmd := messaging.Command{
		SchemaVersion: messaging.SchemaVersion,
		ID:            "cmd:" + uuid.NewString(),
		Kind:          messaging.CommandSendMessage,
		Provider:      messaging.ProviderWhatsApp,
		ChannelID:     uuid.NewString(),
		CreatedAt:     time.Now().UTC(),
		MessageID:     uuid.NewString(),
		Message: &messaging.Message{
			ExternalThreadID: "+15551234567",
			Direction:        messaging.DirectionOutbound,
			Sender:           messaging.Sender{ExternalID: "agent-1", DisplayName: "Agent"},
			ContentType:      messaging.ContentText,
			Text:             "High-performance outbox message payload for testing throughput",
			ProviderTimestamp: time.Now().UTC(),
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		bytes, err := json.Marshal(cmd)
		if err != nil {
			b.Fatalf("marshal error: %v", err)
		}

		var parsed messaging.Command
		if err := json.Unmarshal(bytes, &parsed); err != nil {
			b.Fatalf("unmarshal error: %v", err)
		}
	}
}

// BenchmarkNextBackoff benchmarks backoff calculation under tight loop conditions.
func BenchmarkNextBackoff(b *testing.B) {
	current := 100 * time.Millisecond
	max := 5 * time.Second

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = nextBackoff(current, max)
	}
}

// BenchmarkCommandStream benchmarks provider stream resolution mapping.
func BenchmarkCommandStream(b *testing.B) {
	provider := messaging.ProviderWhatsApp

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		stream, err := commandStream(provider)
		if err != nil || stream == "" {
			b.Fatal("stream mapping error")
		}
	}
}
