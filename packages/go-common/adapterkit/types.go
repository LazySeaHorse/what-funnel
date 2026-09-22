package adapterkit

import (
	"errors"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

var (
	ErrNotFound      = errors.New("adapter: resource not found")
	ErrAlreadyExists = errors.New("adapter: resource already exists")
	ErrMediaNotFound = errors.New("adapter: media not found")
	ErrNotConnected  = errors.New("adapter: channel not connected")
)

// Snapshot represents the connection state of a messaging adapter channel.
type Snapshot struct {
	ChannelID       string                     `json:"channel_id"`
	State           messaging.ConnectionStatus `json:"state"`
	Detail          string                     `json:"detail,omitempty"`
	QRData          string                     `json:"qr_data,omitempty"`
	RemoteAccountID string                     `json:"remote_account_id,omitempty"`
}

// MediaFile represents downloaded or outbound media content with metadata.
type MediaFile struct {
	Filename string `json:"filename,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
	Data     []byte `json:"-"`
}
