package service

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// ErrInvalidSettings is returned when an account settings payload contains a
// known key with the wrong type or an unsupported value.
var ErrInvalidSettings = errors.New("invalid account settings")

type settingKind int

const (
	settingBool settingKind = iota
	settingString
	settingArray
	settingObject
)

// knownSettings lists the account settings keys that other services read with
// a fixed type (for example SQL ::boolean casts). Unknown keys are preserved
// as-is so feature-specific keys (onboarding, business profile) keep working.
// maxGreetingTextRunes bounds the account's canned first-message greeting.
const maxGreetingTextRunes = 500

var knownSettings = map[string]settingKind{
	"ai_enabled":                                  settingBool,
	"lead_tracking_enabled":                       settingBool,
	"allow_member_reply_mode_override":            settingBool,
	"ai_may_auto_answer_mixed_conversations":      settingBool,
	"unassigned_conversations_visible_to_members": settingBool,
	"ai_reply_mode_default":                       settingString,
	"ai_greeting_text":                            settingString,
	"time_format":                                 settingString,
	"timezone":                                    settingString,
	"language":                                    settingString,
	"date_format":                                 settingString,
	"business_type":                               settingString,
	"business_category":                           settingString,
	"business_phone":                              settingString,
	"business_email":                              settingString,
	"business_address":                            settingString,
	"business_website":                            settingString,
	"business_hours":                              settingString,
	"summary_schema":                              settingArray,
	"onboarding":                                  settingObject,
}

// validateSettings checks the types (and enum values) of known settings keys.
// A JSON null clears a key and is always accepted.
func validateSettings(settings map[string]any) error {
	for key, value := range settings {
		kind, known := knownSettings[key]
		if !known || value == nil {
			continue
		}
		var ok bool
		switch kind {
		case settingBool:
			_, ok = value.(bool)
		case settingString:
			_, ok = value.(string)
		case settingArray:
			_, ok = value.([]any)
			if !ok {
				_, ok = value.([]map[string]string)
			}
		case settingObject:
			_, ok = value.(map[string]any)
		}
		if !ok {
			return fmt.Errorf("%w: %q has the wrong type", ErrInvalidSettings, key)
		}
	}
	if v, ok := settings["ai_reply_mode_default"].(string); ok && v != "auto_send" && v != "draft_only" {
		return fmt.Errorf("%w: ai_reply_mode_default must be auto_send or draft_only", ErrInvalidSettings)
	}
	if v, ok := settings["ai_greeting_text"].(string); ok && utf8.RuneCountInString(v) > maxGreetingTextRunes {
		return fmt.Errorf("%w: ai_greeting_text must be at most %d characters", ErrInvalidSettings, maxGreetingTextRunes)
	}
	if v, ok := settings["time_format"].(string); ok && v != "12" && v != "24" {
		return fmt.Errorf("%w: time_format must be 12 or 24", ErrInvalidSettings)
	}
	return nil
}
