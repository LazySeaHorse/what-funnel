package eventstream

import (
	"github.com/whatfunnel/whatfunnel/packages/go-common/adapterkit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
)

const EventsStream = adapterkit.DefaultEventsStream

type Publisher = adapterkit.EventPublisher

func NewPublisher(client *pubsub.Client) *Publisher {
	return adapterkit.NewEventPublisher(client)
}
