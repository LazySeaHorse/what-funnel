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

// InvalidCredentialError reports that a caller-supplied credential was rejected.
// Its message is safe to show to end users.
type InvalidCredentialError struct{ Message string }

func (e *InvalidCredentialError) Error() string { return e.Message }

// ErrInvalidCredential is the sentinel matched by errors.Is for InvalidCredentialError.
var ErrInvalidCredential = errors.New("adapter: invalid credential")

func (e *InvalidCredentialError) Is(target error) bool { return target == ErrInvalidCredential }

// NewInvalidCredential returns a user-safe invalid credential error.
func NewInvalidCredential(message string) error { return &InvalidCredentialError{Message: message} }

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
