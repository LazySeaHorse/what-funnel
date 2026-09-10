package types

import "testing"

func TestAIControlValues(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{name: "known state", valid: AIStatePausedHuman.Valid()},
		{name: "unknown state", valid: !AIState("paused_humna").Valid()},
		{name: "blocked state", valid: AIStateBlockedSpam.Blocked()},
		{name: "active is not blocked", valid: !AIStateActive.Blocked()},
		{name: "known reply override", valid: AIReplyOverrideDisabled.Valid()},
		{name: "unknown reply override", valid: !AIReplyOverride("disable").Valid()},
		{name: "known run state", valid: AIRunStateReplying.Valid()},
		{name: "unknown run state", valid: !AIRunState("running").Valid()},
		{name: "known action", valid: AIControlActionResume.Valid()},
		{name: "unknown action", valid: !AIControlAction("restart").Valid()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.valid {
				t.Fatal("value validation returned an unexpected result")
			}
		})
	}
}
