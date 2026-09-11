package service

import (
	"testing"

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
