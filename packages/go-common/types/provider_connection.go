package types

import (
	"time"

	"github.com/google/uuid"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

type ProviderConnection struct {
	ChannelID       uuid.UUID                  `json:"channel_id" db:"channel_id"`
	AccountID       uuid.UUID                  `json:"account_id" db:"account_id"`
	Provider        messaging.Provider         `json:"provider" db:"provider"`
	Label           string                     `json:"label" db:"label"`
	State           messaging.ConnectionStatus `json:"state" db:"state"`
	Detail          string                     `json:"detail,omitempty" db:"detail"`
	QRData          string                     `json:"qr_data,omitempty"`
	RemoteAccountID string                     `json:"remote_account_id,omitempty" db:"remote_account_id"`
	Capabilities    messaging.Capabilities     `json:"capabilities"`
	CreatedAt       time.Time                  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time                  `json:"updated_at" db:"updated_at"`
}
