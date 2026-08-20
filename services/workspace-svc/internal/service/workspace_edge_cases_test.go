package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChangeUserRole_CrossAccount_EXISTS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountA, adminA := setupTestTenant(t, pool, "Role Tenant A", "admin_a@example.com")
	accountB, _ := setupTestTenant(t, pool, "Role Tenant B", "admin_b@example.com")

	// Create a user in Tenant B
	var userB uuid.UUID
	err := pool.QueryRow(ctx,
		`INSERT INTO users (account_id, email, password_hash, role)
		 VALUES ($1, 'user_b@example.com', 'hashed', 'member') RETURNING id`,
		accountB).Scan(&userB)
	require.NoError(t, err)

	// Admin A attempts to change role of User B (who is in Account B) under Account A
	err = svc.ChangeUserRole(ctx, accountA, adminA, userB, "admin")
	assert.Error(t, err, "cross-account role change must be rejected")
	assert.EqualError(t, err, "user not found in account")

	// Admin A attempts to change role of a non-existent user under Account A
	err = svc.ChangeUserRole(ctx, accountA, adminA, uuid.New(), "admin")
	assert.Error(t, err, "non-existent user role change must be rejected")
	assert.EqualError(t, err, "user not found in account")
}

func TestPatchOnboardingStatus_PreservesSettingsAndTransitions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, _ := setupTestTenant(t, pool, "OnboardingTransitions", "ob_trans@example.com")

	// Set initial custom settings
	initialSettings := map[string]any{
		"allow_member_reply_mode_override": false,
		"custom_property":                  "retained_val",
	}
	initialBytes, _ := json.Marshal(initialSettings)
	_, err := pool.Exec(ctx, `UPDATE accounts SET settings = $1 WHERE id = $2`, initialBytes, accountID)
	require.NoError(t, err)

	// 1. Skip step 1
	err = svc.PatchOnboardingStatus(ctx, accountID, "step1", "skip")
	require.NoError(t, err)

	// 2. Complete step 2
	err = svc.PatchOnboardingStatus(ctx, accountID, "step2", "complete")
	require.NoError(t, err)

	state, err := svc.GetOnboardingStatus(ctx, accountID)
	require.NoError(t, err)
	assert.Contains(t, state.SkippedSteps, "step1")
	assert.Contains(t, state.CompletedSteps, "step2")

	// 3. Complete step 1 (overrides skip)
	err = svc.PatchOnboardingStatus(ctx, accountID, "step1", "complete")
	require.NoError(t, err)

	state, err = svc.GetOnboardingStatus(ctx, accountID)
	require.NoError(t, err)
	assert.Contains(t, state.CompletedSteps, "step1")
	assert.NotContains(t, state.SkippedSteps, "step1")
	assert.Contains(t, state.CompletedSteps, "step2")

	// 4. Skip step 2 (overrides complete)
	err = svc.PatchOnboardingStatus(ctx, accountID, "step2", "skip")
	require.NoError(t, err)

	state, err = svc.GetOnboardingStatus(ctx, accountID)
	require.NoError(t, err)
	assert.Contains(t, state.SkippedSteps, "step2")
	assert.NotContains(t, state.CompletedSteps, "step2")

	// 5. Verify invalid action is rejected
	err = svc.PatchOnboardingStatus(ctx, accountID, "step1", "invalid_action")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `invalid action "invalid_action": must be "complete" or "skip"`)

	// 6. Verify custom settings are still intact!
	var settingsRaw []byte
	err = pool.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1`, accountID).Scan(&settingsRaw)
	require.NoError(t, err)

	var currentSettings map[string]any
	err = json.Unmarshal(settingsRaw, &currentSettings)
	require.NoError(t, err)
	assert.Equal(t, false, currentSettings["allow_member_reply_mode_override"])
	assert.Equal(t, "retained_val", currentSettings["custom_property"])
}

func TestUpdateUserReplyMode_SettingsEdgeCases(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, adminID := setupTestTenant(t, pool, "ReplyModeEdge", "rme@example.com")

	// Case 1: Empty settings JSON object -> should default allow_member_reply_mode_override to true
	_, err := pool.Exec(ctx, `UPDATE accounts SET settings = '{}' WHERE id = $1`, accountID)
	require.NoError(t, err)

	mode := "draft_only"
	err = svc.UpdateUserReplyMode(ctx, accountID, adminID, &mode)
	require.NoError(t, err, "empty settings should default to allowing override")

	// Case 2: Other settings present, but allow_member_reply_mode_override missing
	_, err = pool.Exec(ctx, `UPDATE accounts SET settings = '{"other_key": "some_value"}' WHERE id = $1`, accountID)
	require.NoError(t, err)

	err = svc.UpdateUserReplyMode(ctx, accountID, adminID, &mode)
	require.NoError(t, err, "missing allow_member_reply_mode_override should default to allowing override")

	// Case 3: Explicitly false in settings -> blocked
	_, err = pool.Exec(ctx, `UPDATE accounts SET settings = '{"allow_member_reply_mode_override": false}' WHERE id = $1`, accountID)
	require.NoError(t, err)

	err = svc.UpdateUserReplyMode(ctx, accountID, adminID, &mode)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "member reply mode overrides are not allowed")

	// Case 4: Explicitly true in settings -> allowed
	_, err = pool.Exec(ctx, `UPDATE accounts SET settings = '{"allow_member_reply_mode_override": true}' WHERE id = $1`, accountID)
	require.NoError(t, err)

	err = svc.UpdateUserReplyMode(ctx, accountID, adminID, &mode)
	require.NoError(t, err)
}
