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
	if consumer.consumerName != "consumer-1" {
		t.Errorf("consumer name = %q, want consumer-1", consumer.consumerName)
	}
	if consumer.sender == nil {
		t.Fatal("sender = nil")
	}
}
