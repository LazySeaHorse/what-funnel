package fake

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

func TestFakeAdapter(t *testing.T) {
	adapter := New()

	// Test status default
	status := adapter.Status("channel-1")
	assert.Equal(t, "connected", status.Status)

	// Test custom status
	adapter.SetStatus("channel-1", types.ChannelStatus{Status: "error", Detail: "Failed connection"})
	status = adapter.Status("channel-1")
	assert.Equal(t, "error", status.Status)
	assert.Equal(t, "Failed connection", status.Detail)

	// Test publish and simulate inbound
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedEvent types.InboundEvent
	receivedChan := make(chan types.InboundEvent, 1)

	go func() {
		_ = adapter.Start(ctx, func(ev types.InboundEvent) {
			receivedChan <- ev
		})
	}()

	// Give a tiny moment to start the goroutine
	time.Sleep(10 * time.Millisecond)

	event := types.InboundEvent{
		ChannelID:        "chan-1",
		ExternalThreadID: "thread-1",
		Contact: types.ContactRef{
			ExternalIdentity: "jid-1",
			DisplayName:      "Alice",
		},
		Message: types.NormalizedMessage{
			ContentType: "text",
			Text:        "hello",
		},
		Timestamp: time.Now(),
	}

	adapter.SimulateInbound(event)

	select {
	case receivedEvent = <-receivedChan:
		assert.Equal(t, "chan-1", receivedEvent.ChannelID)
		assert.Equal(t, "hello", receivedEvent.Message.Text)
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for simulated inbound event")
	}

	// Test send message capture
	msg := types.NormalizedMessage{
		ContentType: "text",
		Text:        "reply",
	}
	err := adapter.SendMessage(ctx, "chan-1", "thread-1", msg)
	assert.NoError(t, err)

	sent := adapter.GetSentMessages()
	assert.Len(t, sent, 1)
	assert.Equal(t, "chan-1", sent[0].ChannelID)
	assert.Equal(t, "reply", sent[0].Message.Text)

	adapter.ClearSentMessages()
	assert.Empty(t, adapter.GetSentMessages())
}
