package commandstream

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
)

const (
	CommandsStream = "adapter.commands.telegram"
	consumerGroup  = "telegram-adapter"
)

type Sender interface {
	Send(context.Context, messaging.Command) error
}

type Consumer struct {
	client       *pubsub.Client
	sender       Sender
	consumerName string
}

func NewConsumer(client *pubsub.Client, sender Sender, consumerName string) *Consumer {
	return &Consumer{client: client, sender: sender, consumerName: consumerName}
}

func (c *Consumer) Run(ctx context.Context) error {
	err := c.client.Consume(ctx, CommandsStream, consumerGroup, c.consumerName, func(ctx context.Context, _ string, payload []byte) error {
		var command messaging.Command
		if err := json.Unmarshal(payload, &command); err != nil {
			return nil
		}
		if err := command.Validate(); err != nil || command.Provider != messaging.ProviderTelegram {
			return nil
		}
		return c.sender.Send(ctx, command)
	})
	if err != nil {
		return fmt.Errorf("consume telegram commands: %w", err)
	}
	return nil
}
