package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
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
	var replyMode *string
	var settingsBytes []byte
	if err := svc.pool.QueryRow(ctx, `
		SELECT u.reply_mode_override, a.settings
		FROM users u
		JOIN accounts a ON a.id = u.account_id
		WHERE u.id = $1 AND u.account_id = $2
	`, userID, accountID).Scan(&replyMode, &settingsBytes); err != nil {
		return nil, fmt.Errorf("lookup user reply mode: %w", err)
	}

	settings := parseSettings(settingsBytes)
	workspaceDefault, _ := settings["ai_reply_mode_default"].(string)
	if workspaceDefault != "auto_send" && workspaceDefault != "draft_only" {
		workspaceDefault = "draft_only"
	}
	overrideAllowed := boolSetting(settings, "allow_member_reply_mode_override", true)
	effective := workspaceDefault
	if overrideAllowed && replyMode != nil && (*replyMode == "auto_send" || *replyMode == "draft_only") {
		effective = *replyMode
	}

	return &UserReplyModePreferences{
		ReplyMode:          replyMode,
		WorkspaceDefault:   workspaceDefault,
		EffectiveReplyMode: effective,
		OverrideAllowed:    overrideAllowed,
	}, nil
}

// UpdateUserReplyMode modifies the user's reply mode preference if overrides are allowed by the workspace.
func (svc *Service) UpdateUserReplyMode(ctx context.Context, accountID, userID uuid.UUID, replyMode *string) error {
	// 1. Fetch account settings to check if override is allowed
	var settingsBytes []byte
	err := svc.pool.QueryRow(ctx, "SELECT settings FROM accounts WHERE id = $1", accountID).Scan(&settingsBytes)
	if err != nil {
		return fmt.Errorf("lookup account settings: %w", err)
	}

	// default is true when the key is absent
	if !boolSetting(parseSettings(settingsBytes), "allow_member_reply_mode_override", true) {
		return fmt.Errorf("agent reply mode overrides are not allowed by the manager")
	}

	// 2. Update user override in DB
	_, err = svc.pool.Exec(ctx,
		"UPDATE users SET reply_mode_override = $1 WHERE id = $2 AND account_id = $3",
		replyMode, userID, accountID)
	if err != nil {
		return fmt.Errorf("update user reply mode: %w", err)
	}

	return nil
}
