package types

import (
	"encoding/json"

	"github.com/google/uuid"
)

// AccountSettings represents the settings payload stored in accounts.settings jsonb.
type AccountSettings struct {
	UnassignedConversationsVisibleToMembers *bool `json:"unassigned_conversations_visible_to_members,omitempty"`
}

// IsUnassignedVisible returns the value of unassigned_conversations_visible_to_members setting,
// defaulting to true if absent or invalid.
func IsUnassignedVisible(settingsBytes []byte) bool {
	if len(settingsBytes) == 0 {
		return true
	}
	var settings AccountSettings
	if err := json.Unmarshal(settingsBytes, &settings); err != nil {
		return true
	}
	if settings.UnassignedConversationsVisibleToMembers == nil {
		return true
	}
	return *settings.UnassignedConversationsVisibleToMembers
}

// CanSeeConversation checks if a user is allowed to see a conversation based on the rules in §2:
// - they are admin, or
// - their user_id is in conversations.assigned_user_ids, or
// - assigned_user_ids is empty and the account setting unassigned_conversations_visible_to_members is true.
func CanSeeConversation(userRole string, userID uuid.UUID, assignedUserIDs []uuid.UUID, unassignedConversationsVisibleToMembers bool) bool {
	if userRole == RoleAdmin {
		return true
	}
	// Check if user is assigned
	for _, id := range assignedUserIDs {
		if id == userID {
			return true
		}
	}
	// Check if unassigned and setting is true
	if len(assignedUserIDs) == 0 && unassignedConversationsVisibleToMembers {
		return true
	}
	return false
}
