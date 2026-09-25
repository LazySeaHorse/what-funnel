package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db/dbgen"
)

// UserReplyModePreferences represents user and workspace reply mode configuration.
type UserReplyModePreferences struct {
	ReplyMode          *string `json:"reply_mode"`
	WorkspaceDefault   string  `json:"workspace_default"`
	EffectiveReplyMode string  `json:"effective_reply_mode"`
	OverrideAllowed    bool    `json:"override_allowed"`
}

// GetUserReplyMode retrieves the user's explicit reply mode and computes the effective setting.
func (svc *Service) GetUserReplyMode(ctx context.Context, accountID, userID uuid.UUID) (*UserReplyModePreferences, error) {
	row, err := dbgen.New(svc.pool).GetUserReplyMode(ctx, dbgen.GetUserReplyModeParams{
		ID:        userID,
		AccountID: accountID,
	})
	if err != nil {
		return nil, fmt.Errorf("lookup user reply mode: %w", err)
	}

	settings := parseSettings(row.Settings)
	workspaceDefault, _ := settings["ai_reply_mode_default"].(string)
	if workspaceDefault != "auto_send" && workspaceDefault != "draft_only" {
		workspaceDefault = "draft_only"
	}
	overrideAllowed := boolSetting(settings, "allow_member_reply_mode_override", true)
	effective := workspaceDefault
	if overrideAllowed && row.ReplyModeOverride != nil && (*row.ReplyModeOverride == "auto_send" || *row.ReplyModeOverride == "draft_only") {
		effective = *row.ReplyModeOverride
	}

	return &UserReplyModePreferences{
		ReplyMode:          row.ReplyModeOverride,
		WorkspaceDefault:   workspaceDefault,
		EffectiveReplyMode: effective,
		OverrideAllowed:    overrideAllowed,
	}, nil
}

// UpdateUserReplyMode modifies the user's reply mode preference if overrides are allowed by the workspace.
func (svc *Service) UpdateUserReplyMode(ctx context.Context, accountID, userID uuid.UUID, replyMode *string) error {
	// 1. Fetch account settings to check if override is allowed
	settingsBytes, err := dbgen.New(svc.pool).GetAccountSettings(ctx, accountID)
	if err != nil {
		return fmt.Errorf("lookup account settings: %w", err)
	}

	// default is true when the key is absent
	if !boolSetting(parseSettings(settingsBytes), "allow_member_reply_mode_override", true) {
		return fmt.Errorf("agent reply mode overrides are not allowed by the manager")
	}

	// 2. Update user override in DB
	err = dbgen.New(svc.pool).UpdateUserReplyMode(ctx, dbgen.UpdateUserReplyModeParams{
		ReplyModeOverride: replyMode,
		ID:                userID,
		AccountID:         accountID,
	})
	if err != nil {
		return fmt.Errorf("update user reply mode: %w", err)
	}

	return nil
}
