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
		// Admin cases (should always be true)
		{"Admin - unassigned, setting false", RoleAdmin, userA, []uuid.UUID{}, false, true},
		{"Admin - assigned to self, setting false", RoleAdmin, userA, []uuid.UUID{userA}, false, true},
		{"Admin - assigned to other, setting false", RoleAdmin, userA, []uuid.UUID{userB}, false, true},
		{"Admin - unassigned, setting true", RoleAdmin, userA, []uuid.UUID{}, true, true},
		{"Admin - assigned to self, setting true", RoleAdmin, userA, []uuid.UUID{userA}, true, true},
		{"Admin - assigned to other, setting true", RoleAdmin, userA, []uuid.UUID{userB}, true, true},

		// Member cases
		{"Member - assigned to self, setting false", RoleMember, userA, []uuid.UUID{userA}, false, true},
		{"Member - assigned to self, setting true", RoleMember, userA, []uuid.UUID{userA}, true, true},
		{"Member - assigned to other, setting false", RoleMember, userA, []uuid.UUID{userB}, false, false},
		{"Member - assigned to other, setting true", RoleMember, userA, []uuid.UUID{userB}, true, false},
		{"Member - unassigned, setting true", RoleMember, userA, []uuid.UUID{}, true, true},
		{"Member - unassigned, setting false", RoleMember, userA, []uuid.UUID{}, false, false},
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
