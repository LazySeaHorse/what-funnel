package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/service"
)

func testProviderConfig(apiKey, baseURL string) service.AIProviderConfig {
	return service.AIProviderConfig{
		APIKey:         apiKey,
		BaseURL:        baseURL,
		AnalysisModel:  "analysis",
		ReplyModel:     "reply",
		EmbeddingModel: "embedding",
	}
}

func TestUpdateAIProviderConfig_BlankKeyCannotRedirectStoredKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()
	accountID, userID := setupTestTenant(t, pool, "AI Exfil", "ai-exfil@example.com")

	require.NoError(t, svc.UpdateAIProviderConfig(ctx, accountID, userID,
		testProviderConfig("sk-real-secret", "https://api.openai.com/v1")))

	// Blank key + different base URL must be rejected and change nothing.
	err := svc.UpdateAIProviderConfig(ctx, accountID, userID,
		testProviderConfig("", "https://attacker.example.org/v1"))
	require.ErrorIs(t, err, service.ErrAIProviderKeyRequired)
	stored, err := svc.GetAIProviderConfig(ctx, accountID)
	require.NoError(t, err)
	assert.Equal(t, "https://api.openai.com/v1", stored.BaseURL)
	assert.Equal(t, "sk-real-secret", stored.APIKey)

	// Blank key with the same base URL (trailing slash tolerated) still works.
	require.NoError(t, svc.UpdateAIProviderConfig(ctx, accountID, userID,
		testProviderConfig("", "https://api.openai.com/v1/")))

	// Supplying a new key allows the endpoint change.
	require.NoError(t, svc.UpdateAIProviderConfig(ctx, accountID, userID,
		testProviderConfig("sk-new", "https://other.example.org/v1")))
	stored, err = svc.GetAIProviderConfig(ctx, accountID)
	require.NoError(t, err)
	assert.Equal(t, "https://other.example.org/v1", stored.BaseURL)
	assert.Equal(t, "sk-new", stored.APIKey)
}

func TestUpdateAIProviderConfig_DoesNotReenableAIOnLaterSaves(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()
	accountID, userID := setupTestTenant(t, pool, "AI Stay Off", "ai-stay-off@example.com")

	require.NoError(t, svc.UpdateAIProviderConfig(ctx, accountID, userID,
		testProviderConfig("sk-a", "https://api.openai.com/v1")))

	// Manager disables AI after the initial configuration.
	_, err := pool.Exec(ctx, `UPDATE accounts SET settings = settings || '{"ai_enabled": false}'::jsonb WHERE id = $1`, accountID)
	require.NoError(t, err)

	require.NoError(t, svc.UpdateAIProviderConfig(ctx, accountID, userID,
		testProviderConfig("", "https://api.openai.com/v1")))

	var raw []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1`, accountID).Scan(&raw))
	var settings map[string]any
	require.NoError(t, json.Unmarshal(raw, &settings))
	assert.Equal(t, false, settings["ai_enabled"])
}

func TestMergeAccountSettings_RejectsWrongTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()
	accountID, userID := setupTestTenant(t, pool, "Settings Types", "settings-types@example.com")

	err := svc.MergeAccountSettings(ctx, accountID, userID, map[string]any{"ai_enabled": "foo"})
	require.ErrorIs(t, err, service.ErrInvalidSettings)
	err = svc.UpdateAccountSettings(ctx, accountID, userID, map[string]any{"ai_reply_mode_default": "bogus"})
	require.ErrorIs(t, err, service.ErrInvalidSettings)
	require.NoError(t, svc.MergeAccountSettings(ctx, accountID, userID, map[string]any{"ai_enabled": true, "custom": "x"}))
}
