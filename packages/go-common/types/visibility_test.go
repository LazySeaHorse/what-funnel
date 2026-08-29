package types

import (
	"testing"

	"github.com/google/uuid"
)

func TestCanSeeConversation(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()

	tests := []struct {
		name              string
		userRole          string
		userID            uuid.UUID
		assignedUserIDs   []uuid.UUID
		unassignedVisible bool
		want              bool
	}{
		// Manager cases (should always be true)
		{"Manager - unassigned, setting false", RoleManager, userA, []uuid.UUID{}, false, true},
		{"Manager - assigned to self, setting false", RoleManager, userA, []uuid.UUID{userA}, false, true},
		{"Manager - assigned to other, setting false", RoleManager, userA, []uuid.UUID{userB}, false, true},
		{"Manager - unassigned, setting true", RoleManager, userA, []uuid.UUID{}, true, true},
		{"Manager - assigned to self, setting true", RoleManager, userA, []uuid.UUID{userA}, true, true},
		{"Manager - assigned to other, setting true", RoleManager, userA, []uuid.UUID{userB}, true, true},

		// Agent cases
		{"Agent - assigned to self, setting false", RoleAgent, userA, []uuid.UUID{userA}, false, true},
		{"Agent - assigned to self, setting true", RoleAgent, userA, []uuid.UUID{userA}, true, true},
		{"Agent - assigned to other, setting false", RoleAgent, userA, []uuid.UUID{userB}, false, false},
		{"Agent - assigned to other, setting true", RoleAgent, userA, []uuid.UUID{userB}, true, false},
		{"Agent - unassigned, setting true", RoleAgent, userA, []uuid.UUID{}, true, true},
		{"Agent - unassigned, setting false", RoleAgent, userA, []uuid.UUID{}, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanSeeConversation(tt.userRole, tt.userID, tt.assignedUserIDs, tt.unassignedVisible)
			if got != tt.want {
				t.Errorf("CanSeeConversation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsLeadTrackingEnabledForProduct(t *testing.T) {
	tests := []struct {
		name        string
		productMode string
		settings    []byte
		want        bool
	}{
		{"full workspace default", "full_workspace", nil, true},
		{"full workspace disabled", "full_workspace", []byte(`{"lead_tracking_enabled": false}`), false},
		{"chatbot legacy default", "chatbot_only", nil, false},
		{"chatbot ignores enabled setting", "chatbot_only", []byte(`{"lead_tracking_enabled": true}`), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsLeadTrackingEnabledForProduct(tt.productMode, tt.settings); got != tt.want {
				t.Fatalf("IsLeadTrackingEnabledForProduct() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsUnassignedVisible(t *testing.T) {
	tests := []struct {
		name          string
		settingsBytes []byte
		want          bool
	}{
		{"empty settings", nil, true},
		{"empty JSON object", []byte("{}"), true},
		{"explicit true", []byte(`{"unassigned_conversations_visible_to_members": true}`), true},
		{"explicit false", []byte(`{"unassigned_conversations_visible_to_members": false}`), false},
		{"invalid JSON", []byte(`{invalid}`), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsUnassignedVisible(tt.settingsBytes)
			if got != tt.want {
				t.Errorf("IsUnassignedVisible() = %v, want %v", got, tt.want)
			}
		})
	}
}
