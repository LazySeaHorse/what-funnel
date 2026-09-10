package service

import (
	"testing"

	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

func TestParseAIControlCommand(t *testing.T) {
	tests := []struct {
		name         string
		action       string
		override     string
		wantAction   bool
		wantOverride bool
		wantErr      string
	}{
		{name: "pause", action: "pause", wantAction: true},
		{name: "override only", override: "disabled", wantOverride: true},
		{name: "both", action: "resume", override: "inherit", wantAction: true, wantOverride: true},
		{name: "empty", wantErr: "an action or reply_override is required"},
		{name: "invalid action", action: "restart", wantErr: "invalid AI control action"},
		{name: "invalid override", override: "default", wantErr: "invalid reply_override"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := parseAIControlCommand(tt.action, tt.override)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("parseAIControlCommand() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAIControlCommand() error = %v", err)
			}
			if cmd.hasAction != tt.wantAction || cmd.hasReplyOverride != tt.wantOverride {
				t.Fatalf("parseAIControlCommand() flags = (%v, %v), want (%v, %v)", cmd.hasAction, cmd.hasReplyOverride, tt.wantAction, tt.wantOverride)
			}
		})
	}
}

func TestPlanAIControlTransition(t *testing.T) {
	tests := []struct {
		name           string
		current        types.AIState
		action         types.AIControlAction
		canUnblockSpam bool
		wantState      types.AIState
		wantReason     types.AIStateReason
		wantChanged    bool
		wantErr        string
	}{
		{name: "override only", current: types.AIStateCooldown, wantState: types.AIStateCooldown},
		{name: "pause", current: types.AIStateActive, action: types.AIControlActionPause, wantState: types.AIStatePausedHuman, wantReason: types.AIStateReasonManualPause, wantChanged: true},
		{name: "resume", current: types.AIStatePausedHuman, action: types.AIControlActionResume, wantState: types.AIStateActive, wantReason: types.AIStateReasonManualResume, wantChanged: true},
		{name: "block", current: types.AIStateActive, action: types.AIControlActionBlock, wantState: types.AIStateBlockedManual, wantReason: types.AIStateReasonManualBlock, wantChanged: true},
		{name: "manager unblocks spam", current: types.AIStateBlockedSpam, action: types.AIControlActionResume, canUnblockSpam: true, wantState: types.AIStateActive, wantReason: types.AIStateReasonManualResume, wantChanged: true},
		{name: "agent cannot unblock spam", current: types.AIStateBlockedSpam, action: types.AIControlActionResume, wantErr: "manager role required to unblock suspected spam"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := aiControlCommand{action: tt.action, hasAction: tt.action != ""}
			got, err := planAIControlTransition(tt.current, cmd, tt.canUnblockSpam)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("planAIControlTransition() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("planAIControlTransition() error = %v", err)
			}
			if got.state != tt.wantState || got.reason != tt.wantReason || got.stateChanged != tt.wantChanged {
				t.Fatalf("planAIControlTransition() = %+v, want state=%q reason=%q changed=%v", got, tt.wantState, tt.wantReason, tt.wantChanged)
			}
		})
	}
}
