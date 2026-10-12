package types

import "testing"

func TestValidProductMode(t *testing.T) {
	for mode, want := range map[string]bool{
		ProductModeFullWorkspace: true,
		ProductModeChatbotOnly:   true,
		"":                       false,
		"Full_Workspace":         false,
		"other":                  false,
	} {
		if got := ValidProductMode(mode); got != want {
			t.Errorf("ValidProductMode(%q) = %v, want %v", mode, got, want)
		}
	}
}

func TestValidReplyMode(t *testing.T) {
	for mode, want := range map[string]bool{
		ReplyModeAutoSend:  true,
		ReplyModeDraftOnly: true,
		"":                 false,
		"manual":           false,
	} {
		if got := ValidReplyMode(mode); got != want {
			t.Errorf("ValidReplyMode(%q) = %v, want %v", mode, got, want)
		}
	}
}
