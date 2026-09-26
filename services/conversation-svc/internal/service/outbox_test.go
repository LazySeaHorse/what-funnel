package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

func TestCommandStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		provider messaging.Provider
		want     string
		wantErr  bool
	}{
		{provider: messaging.ProviderWhatsApp, want: "adapter.commands.whatsapp"},
		{provider: messaging.ProviderTelegram, want: "adapter.commands.telegram"},
		{provider: messaging.Provider("unknown"), wantErr: true},
	}
	for _, test := range tests {
		got, err := commandStream(test.provider)
		if (err != nil) != test.wantErr {
			t.Fatalf("commandStream(%q) error = %v, wantErr %v", test.provider, err, test.wantErr)
		}
		if got != test.want {
			t.Errorf("commandStream(%q) = %q, want %q", test.provider, got, test.want)
		}
	}
}

func TestNextBackoff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		current  time.Duration
		max      time.Duration
		expected time.Duration
	}{
		{current: 100 * time.Millisecond, max: 5 * time.Second, expected: 200 * time.Millisecond},
		{current: 200 * time.Millisecond, max: 5 * time.Second, expected: 400 * time.Millisecond},
		{current: 3 * time.Second, max: 5 * time.Second, expected: 5 * time.Second},
		{current: 5 * time.Second, max: 5 * time.Second, expected: 5 * time.Second},
		{current: 10 * time.Second, max: 5 * time.Second, expected: 5 * time.Second},
	}

	for _, tc := range tests {
		got := nextBackoff(tc.current, tc.max)
		if got != tc.expected {
			t.Errorf("nextBackoff(%v, %v) = %v, want %v", tc.current, tc.max, got, tc.expected)
		}
	}
}

func TestDispatchOutbox_CanceledContext(t *testing.T) {
	t.Parallel()

	s := NewOutboxService(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.DispatchOutbox(ctx)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

