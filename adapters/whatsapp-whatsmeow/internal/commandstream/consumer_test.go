package commandstream

import (
	"context"
	"testing"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

type fakeSender struct{}

func (fakeSender) Send(context.Context, messaging.Command) error { return nil }

func TestNewConsumer(t *testing.T) {
	t.Parallel()

	consumer := NewConsumer(nil, fakeSender{}, "consumer-1")
	if consumer.Stream() != CommandsStream {
		t.Errorf("stream = %q, want %q", consumer.Stream(), CommandsStream)
	}
	if consumer.Group() != consumerGroup {
		t.Errorf("group = %q, want %q", consumer.Group(), consumerGroup)
	}
}
