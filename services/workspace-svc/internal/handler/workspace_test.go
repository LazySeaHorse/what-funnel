package handler

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeAIProviderConfigRetainsExistingKey(t *testing.T) {
	merged, err := mergeAIProviderConfig(
		`{"api_key":"","base_url":"https://example.test/v1","completion_model":"custom-chat","embedding_model":"custom-embed"}`,
		`{"api_key":"secret-key","base_url":"https://old.test/v1","completion_model":"old-chat","embedding_model":"old-embed","organization":"org-123"}`,
	)
	require.NoError(t, err)

	var config map[string]string
	require.NoError(t, json.Unmarshal([]byte(merged), &config))
	assert.Equal(t, "secret-key", config["api_key"])
	assert.Equal(t, "https://example.test/v1", config["base_url"])
	assert.Equal(t, "custom-chat", config["completion_model"])
	assert.Equal(t, "custom-embed", config["embedding_model"])
	assert.Equal(t, "org-123", config["organization"])
}

func TestMergeAIProviderConfigUsesReplacementKey(t *testing.T) {
	merged, err := mergeAIProviderConfig(
		`{"api_key":"new-key","base_url":"https://example.test/v1"}`,
		`{"api_key":"old-key","base_url":"https://old.test/v1"}`,
	)
	require.NoError(t, err)
	assert.True(t, aiProviderConfigHasKey(merged))
	assert.Contains(t, merged, "new-key")
	assert.NotContains(t, merged, "old-key")
}

func TestPublicAIProviderConfigOmitsAPIKey(t *testing.T) {
	status := publicAIProviderConfig(`{"api_key":"secret-key","base_url":"https://example.test/v1","completion_model":"custom-chat","embedding_model":"custom-embed"}`)

	assert.Equal(t, true, status["configured"])
	assert.Equal(t, "https://example.test/v1", status["base_url"])
	assert.Equal(t, "custom-chat", status["completion_model"])
	assert.Equal(t, "custom-embed", status["embedding_model"])
	_, exposesKey := status["api_key"]
	assert.False(t, exposesKey)
}

func TestPublicAIProviderConfigReportsUnconfigured(t *testing.T) {
	assert.Equal(t, map[string]any{"configured": false}, publicAIProviderConfig(""))
}
