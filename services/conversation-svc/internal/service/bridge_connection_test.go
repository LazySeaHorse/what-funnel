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

func TestBridgeSetupInProgress(t *testing.T) {
	tests := []struct {
		name  string
		state string
		want  bool
	}{
		{name: "waiting for scan", state: "awaiting_scan", want: true},
		{name: "waiting for phone", state: "awaiting_phone", want: true},
		{name: "waiting for code", state: "awaiting_code", want: true},
		{name: "waiting for session", state: "awaiting_session", want: true},
		{name: "connecting", state: "connecting", want: true},
		{name: "connected", state: "connected", want: false},
		{name: "failed", state: "failed", want: false},
		{name: "cancelled", state: "cancelled", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, bridgeSetupInProgress(tt.state))
		})
	}
}
