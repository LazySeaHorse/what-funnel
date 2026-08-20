package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	matrixadapter "github.com/whatfunnel/whatfunnel/adapters/matrix-mautrix"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

func TestInterpretBridgeMessagesOnlyTrustsConfiguredBridge(t *testing.T) {
	connection := &types.BridgeConnection{
		ChannelID:      uuid.New(),
		BridgeIdentity: "@whatsappbot:example.test",
		State:          "awaiting_scan",
		Detail:         "Scan the QR code.",
	}

	state, detail := interpretBridgeMessages(connection, []matrixadapter.BridgeMessage{
		{Sender: "@attacker:example.test", Body: "Successfully logged in"},
		{Sender: "@whatsappbot:example.test", Body: "Successfully logged in"},
	})

	assert.Equal(t, "connected", state)
	assert.Equal(t, "Connected", detail)
}

func TestInterpretBridgeMessagesRecognizesFreshQRCode(t *testing.T) {
	connection := &types.BridgeConnection{BridgeIdentity: "@telegrambot:example.test", State: "connecting", Detail: "Waiting"}
	state, detail := interpretBridgeMessages(connection, []matrixadapter.BridgeMessage{{
		Sender: "@telegrambot:example.test", MediaURL: "mxc://example.test/qr-code",
	}})
	assert.Equal(t, "awaiting_scan", state)
	assert.Contains(t, detail, "fresh QR")
}
