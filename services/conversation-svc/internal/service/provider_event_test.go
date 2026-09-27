package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

func TestProviderMessageContent(t *testing.T) {
	t.Parallel()

	mediaID := uuid.New()
	data, err := providerMessageContent(messaging.Message{
		Text: "Open WhatsApp to view this message.", NoticeCode: "unsupported",
	}, mediaID)
	if err != nil {
		t.Fatalf("providerMessageContent() error = %v", err)
	}
	var content map[string]any
	if err := json.Unmarshal(data, &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if content["media_id"] != mediaID.String() {
		t.Errorf("media_id = %v, want %s", content["media_id"], mediaID)
	}
	if content["notice_code"] != "unsupported" {
		t.Errorf("notice_code = %v, want unsupported", content["notice_code"])
	}
}

func TestIsTerminalIngestError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "channel not found error",
			err:      ErrChannelNotFound,
			expected: true,
		},
		{
			name:     "wrapped channel not found error",
			err:      fmt.Errorf("resolve provider channel: %w", ErrChannelNotFound),
			expected: true,
		},
		{
			name:     "invalid envelope error",
			err:      messaging.ErrInvalidEnvelope,
			expected: true,
		},
		{
			name:     "media too large error",
			err:      messaging.ErrMediaTooLarge,
			expected: true,
		},
		{
			name:     "arbitrary database error",
			err:      errors.New("connection reset by peer"),
			expected: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actual := IsTerminalIngestError(tc.err)
			if actual != tc.expected {
				t.Errorf("IsTerminalIngestError(%v) = %v, want %v", tc.err, actual, tc.expected)
			}
		})
	}
}
