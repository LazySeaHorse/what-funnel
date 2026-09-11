package service

import (
	"testing"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

func TestRequireProviderCapability(t *testing.T) {
	t.Parallel()
	telegram := messaging.Capabilities{Reactions: true, Edits: true, Deletes: true}
	for _, kind := range []messaging.CommandKind{messaging.CommandEditMessage, messaging.CommandDeleteMessage, messaging.CommandChangeReaction} {
		if err := requireProviderCapability(telegram, kind); err != nil {
			t.Errorf("Telegram capability %s rejected: %v", kind, err)
		}
	}
	if err := requireProviderCapability(messaging.Capabilities{}, messaging.CommandEditMessage); err == nil {
		t.Fatal("provider without edit capability was accepted")
	}
}

func TestTelegramCapabilitiesDoNotFabricateReceipts(t *testing.T) {
	t.Parallel()
	capabilities := providerCapabilities(messaging.ProviderTelegram)
	if !capabilities.Media || !capabilities.Replies || !capabilities.Reactions || !capabilities.Edits || !capabilities.Deletes {
		t.Fatalf("Telegram capabilities missing supported operation: %+v", capabilities)
	}
	if capabilities.Receipts {
		t.Fatal("Telegram must not advertise delivery or read receipts")
	}
}
