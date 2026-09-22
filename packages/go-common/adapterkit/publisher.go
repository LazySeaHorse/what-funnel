package adapterkit

import (
	"context"
	"fmt"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

const DefaultEventsStream = "adapter.events"

// StreamPublisher abstracts event publishing to a stream backend (e.g. Redis Streams).
type StreamPublisher interface {
	Publish(ctx context.Context, stream string, payload any) (string, error)
}

// EventPublisher validates and publishes adapter messaging events to a stream.
type EventPublisher struct {
	client StreamPublisher
	stream string
}

// NewEventPublisher creates a new EventPublisher.
func NewEventPublisher(client StreamPublisher, stream ...string) *EventPublisher {
	s := DefaultEventsStream
	if len(stream) > 0 && stream[0] != "" {
		s = stream[0]
	}
	return &EventPublisher{
		client: client,
		stream: s,
	}
}

// Publish validates and publishes a normalized messaging event.
func (p *EventPublisher) Publish(ctx context.Context, event messaging.Event) error {
	if p == nil || p.client == nil {
		return fmt.Errorf("publish adapter event: uninitialized publisher")
	}
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate event: %w", err)
	}
	if _, err := p.client.Publish(ctx, p.stream, event); err != nil {
		return fmt.Errorf("publish adapter event: %w", err)
	}
	return nil
}
