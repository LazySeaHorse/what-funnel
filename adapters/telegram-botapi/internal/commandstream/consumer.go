package commandstream

import (
	"github.com/whatfunnel/whatfunnel/packages/go-common/adapterkit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
)

const (
	CommandsStream = "adapter.commands.telegram"
	consumerGroup  = "telegram-adapter"
)

type (
	Sender   = adapterkit.CommandSender
	Consumer = adapterkit.CommandConsumer
)

func NewConsumer(client *pubsub.Client, sender Sender, consumerName string) *Consumer {
	return adapterkit.NewCommandConsumer(client, sender, adapterkit.ConsumerConfig{
		Provider:     messaging.ProviderTelegram,
		ConsumerName: consumerName,
		Stream:       CommandsStream,
		Group:        consumerGroup,
	})
}
