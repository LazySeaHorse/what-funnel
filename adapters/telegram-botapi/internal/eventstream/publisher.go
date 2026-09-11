package eventstream

import (
	"context"
	"fmt"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
)

const EventsStream = "adapter.events"

type Publisher struct{ client *pubsub.Client }

func NewPublisher(client *pubsub.Client) *Publisher { return &Publisher{client: client} }

func (p *Publisher) Publish(ctx context.Context, event messaging.Event) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate event: %w", err)
	}
	if _, err := p.client.Publish(ctx, EventsStream, event); err != nil {
		return fmt.Errorf("publish adapter event: %w", err)
	}
	return nil
}
