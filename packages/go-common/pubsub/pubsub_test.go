package pubsub

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestPayload struct {
	Message string `json:"message"`
	Value   int    `json:"value"`
}

func TestPubSub(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	client, err := NewClient("localhost:6379")
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	stream := "test.stream.pubsub"
	client.RawClient().Del(ctx, stream)

	payload := TestPayload{Message: "hello redis", Value: 42}
	id, err := client.Publish(ctx, stream, payload)
	require.NoError(t, err)
	assert.NotEmpty(t, id)

	ctxCancel, cancel := context.WithCancel(context.Background())
	defer cancel()

	receivedChan := make(chan TestPayload, 1)

	go func() {
		_ = client.Consume(ctxCancel, stream, "test-group", "consumer-1", func(ctx context.Context, id string, data []byte) error {
			var p TestPayload
			if err := json.Unmarshal(data, &p); err != nil {
				return err
			}
			receivedChan <- p
			return nil
		})
	}()

	select {
	case received := <-receivedChan:
		assert.Equal(t, "hello redis", received.Message)
		assert.Equal(t, 42, received.Value)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for consumed message")
	}
}
