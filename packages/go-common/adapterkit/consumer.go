package adapterkit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

// StreamConsumer abstracts consuming messages from a stream group.
type StreamConsumer interface {
	Consume(ctx context.Context, stream, group, consumer string, handler func(ctx context.Context, id string, payload []byte) error) error
}

// CommandSender is the interface implemented by adapter session managers to send outbound messages.
type CommandSender interface {
	Send(context.Context, messaging.Command) error
}

// ConsumerConfig specifies stream consumer settings for an adapter.
type ConsumerConfig struct {
	Provider     messaging.Provider
	ConsumerName string
	Stream       string // defaults to "adapter.commands." + Provider
	Group        string // defaults to Provider + "-adapter"
}

// CommandConsumer consumes commands from a Redis stream for a specific messaging provider.
type CommandConsumer struct {
	client StreamConsumer
	sender CommandSender
	cfg    ConsumerConfig
}

// NewCommandConsumer initializes a new CommandConsumer.
func NewCommandConsumer(client StreamConsumer, sender CommandSender, cfg ConsumerConfig) *CommandConsumer {
	if cfg.Stream == "" {
		cfg.Stream = "adapter.commands." + string(cfg.Provider)
	}
	if cfg.Group == "" {
		cfg.Group = string(cfg.Provider) + "-adapter"
	}
	return &CommandConsumer{
		client: client,
		sender: sender,
		cfg:    cfg,
	}
}

// Stream returns the resolved stream name.
func (c *CommandConsumer) Stream() string {
	return c.cfg.Stream
}

// Group returns the resolved group name.
func (c *CommandConsumer) Group() string {
	return c.cfg.Group
}

// ConsumerName returns the consumer identifier.
func (c *CommandConsumer) ConsumerName() string {
	return c.cfg.ConsumerName
}

// Run blocks listening for commands on the configured stream until context is cancelled or an error occurs.
func (c *CommandConsumer) Run(ctx context.Context) error {
	if c.client == nil || c.sender == nil {
		return fmt.Errorf("consume %s commands: uninitialized consumer or sender", c.cfg.Provider)
	}
	err := c.client.Consume(
		ctx,
		c.cfg.Stream,
		c.cfg.Group,
		c.cfg.ConsumerName,
		func(ctx context.Context, _ string, payload []byte) error {
			var command messaging.Command
			if err := json.Unmarshal(payload, &command); err != nil {
				return nil
			}
			if err := command.Validate(); err != nil || command.Provider != c.cfg.Provider {
				return nil
			}
			return c.sender.Send(ctx, command)
		},
	)
	if err != nil {
		return fmt.Errorf("consume %s commands: %w", c.cfg.Provider, err)
	}
	return nil
}
