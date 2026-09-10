package types

import "testing"

func TestMessageSenderValid(t *testing.T) {
	tests := []struct {
		name   string
		sender MessageSender
		want   bool
	}{
		{name: "contact", sender: MessageSenderContact, want: true},
		{name: "human", sender: MessageSenderHuman, want: true},
		{name: "AI", sender: MessageSenderAI, want: true},
		{name: "unknown", sender: MessageSender("bot"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sender.Valid(); got != tt.want {
				t.Fatalf("MessageSender(%q).Valid() = %v, want %v", tt.sender, got, tt.want)
			}
		})
	}
}
