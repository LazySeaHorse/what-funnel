package service

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateSettingsAIGreetingText(t *testing.T) {
	if err := validateSettings(map[string]any{"ai_greeting_text": "Hi! How can we help?"}); err != nil {
		t.Fatalf("valid greeting rejected: %v", err)
	}
	if err := validateSettings(map[string]any{"ai_greeting_text": nil}); err != nil {
		t.Fatalf("null clears the greeting and must be accepted: %v", err)
	}
	if err := validateSettings(map[string]any{"ai_greeting_text": strings.Repeat("é", maxGreetingTextRunes)}); err != nil {
		t.Fatalf("greeting at the limit rejected: %v", err)
	}
	for name, value := range map[string]any{
		"too long":   strings.Repeat("a", maxGreetingTextRunes+1),
		"wrong type": 42,
	} {
		if err := validateSettings(map[string]any{"ai_greeting_text": value}); !errors.Is(err, ErrInvalidSettings) {
			t.Fatalf("%s: expected ErrInvalidSettings, got %v", name, err)
		}
	}
}
