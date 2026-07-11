package fake

import (
	"context"
	"sync"

	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// SentMessage records an outbound message captured by the fake adapter.
type SentMessage struct {
	ChannelID        string
	ExternalThreadID string
	Message          types.NormalizedMessage
}

// Adapter is a test-only implementation of types.ChannelAdapter.
type Adapter struct {
	mu           sync.RWMutex
	publishFn    func(types.InboundEvent)
	sentMessages []SentMessage
	statuses     map[string]types.ChannelStatus
}

// New creates a new fake adapter instance.
func New() *Adapter {
	return &Adapter{
		statuses: make(map[string]types.ChannelStatus),
	}
}

// Start registers the publish callback and blocks until context cancellation.
func (a *Adapter) Start(ctx context.Context, publish func(types.InboundEvent)) error {
	a.mu.Lock()
	a.publishFn = publish
	a.mu.Unlock()
	<-ctx.Done()
	return nil
}

// SendMessage records the outbound send attempt.
func (a *Adapter) SendMessage(ctx context.Context, channelID, externalThreadID string, msg types.NormalizedMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sentMessages = append(a.sentMessages, SentMessage{
		ChannelID:        channelID,
		ExternalThreadID: externalThreadID,
		Message:          msg,
	})
	return nil
}

// Status returns the configured status for the given channel, defaulting to connected.
func (a *Adapter) Status(channelID string) types.ChannelStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if status, ok := a.statuses[channelID]; ok {
		return status
	}
	return types.ChannelStatus{
		Status: "connected",
		Detail: "Fake adapter connected",
	}
}

// SetStatus sets the status for a channel.
func (a *Adapter) SetStatus(channelID string, status types.ChannelStatus) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.statuses[channelID] = status
}

// SimulateInbound invokes the registered publish callback with a mock event.
func (a *Adapter) SimulateInbound(event types.InboundEvent) {
	a.mu.RLock()
	publish := a.publishFn
	a.mu.RUnlock()
	if publish != nil {
		publish(event)
	}
}

// GetSentMessages returns a copy of all captured outbound messages.
func (a *Adapter) GetSentMessages() []SentMessage {
	a.mu.RLock()
	defer a.mu.RUnlock()
	copied := make([]SentMessage, len(a.sentMessages))
	copy(copied, a.sentMessages)
	return copied
}

// ClearSentMessages resets the list of captured outbound messages.
func (a *Adapter) ClearSentMessages() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sentMessages = nil
}

var _ types.ChannelAdapter = (*Adapter)(nil)
