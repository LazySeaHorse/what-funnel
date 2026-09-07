package handler

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/service"
)

func TestAIProviderStatusOmitsAPIKey(t *testing.T) {
	status := service.AIProviderStatus{
		Configured:     true,
		BaseURL:        "https://example.test/v1",
		AnalysisModel:  "analysis-model",
		ReplyModel:     "reply-model",
		EmbeddingModel: "embedding-model",
	}

	encoded, err := json.Marshal(status)
	assert.NoError(t, err)
	assert.NotContains(t, string(encoded), "api_key")
	assert.Contains(t, string(encoded), `"analysis_model":"analysis-model"`)
	assert.Contains(t, string(encoded), `"reply_model":"reply-model"`)
}
