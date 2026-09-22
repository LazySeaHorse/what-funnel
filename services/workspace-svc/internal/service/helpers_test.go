package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/onboarding"
)

func TestParseSettings(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected map[string]any
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: map[string]any{},
		},
		{
			name:     "empty input",
			input:    []byte{},
			expected: map[string]any{},
		},
		{
			name:     "corrupt/invalid json",
			input:    []byte("{invalid json"),
			expected: map[string]any{},
		},
		{
			name:     "valid json object",
			input:    []byte(`{"lead_tracking_enabled": true, "product_mode": "chatbot_only"}`),
			expected: map[string]any{"lead_tracking_enabled": true, "product_mode": "chatbot_only"},
		},
		{
			name:     "valid json with nested object",
			input:    []byte(`{"onboarding": {"completed_steps": ["step1"], "business_type": "salon"}}`),
			expected: map[string]any{"onboarding": map[string]any{"completed_steps": []any{"step1"}, "business_type": "salon"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := parseSettings(tt.input)
			assert.NotNil(t, res)
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestBoolSetting(t *testing.T) {
	settings := map[string]any{
		"bool_true":  true,
		"bool_false": false,
		"string_val": "true",
		"int_val":    1,
		"nil_val":    nil,
		"slice_val":  []string{"a"},
	}

	tests := []struct {
		name       string
		key        string
		defaultVal bool
		expected   bool
	}{
		{
			name:       "existing true bool",
			key:        "bool_true",
			defaultVal: false,
			expected:   true,
		},
		{
			name:       "existing false bool",
			key:        "bool_false",
			defaultVal: true,
			expected:   false,
		},
		{
			name:       "missing key with default true",
			key:        "non_existent",
			defaultVal: true,
			expected:   true,
		},
		{
			name:       "missing key with default false",
			key:        "non_existent",
			defaultVal: false,
			expected:   false,
		},
		{
			name:       "string value fallback to default",
			key:        "string_val",
			defaultVal: false,
			expected:   false,
		},
		{
			name:       "int value fallback to default",
			key:        "int_val",
			defaultVal: true,
			expected:   true,
		},
		{
			name:       "nil value fallback to default",
			key:        "nil_val",
			defaultVal: true,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := boolSetting(settings, tt.key, tt.defaultVal)
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestParseOnboardingState(t *testing.T) {
	tests := []struct {
		name                   string
		settings               map[string]any
		expectedCompletedSteps []string
		expectedSkippedSteps   []string
		expectedBusinessType   *string
	}{
		{
			name:                   "empty settings",
			settings:               map[string]any{},
			expectedCompletedSteps: []string{},
			expectedSkippedSteps:   []string{},
		},
		{
			name:                   "nil onboarding key",
			settings:               map[string]any{"onboarding": nil},
			expectedCompletedSteps: []string{},
			expectedSkippedSteps:   []string{},
		},
		{
			name:                   "corrupt/non-map onboarding value",
			settings:               map[string]any{"onboarding": "not a map"},
			expectedCompletedSteps: []string{},
			expectedSkippedSteps:   []string{},
		},
		{
			name: "populated onboarding map",
			settings: map[string]any{
				"onboarding": map[string]any{
					"completed_steps": []string{"signup", "channel_connect"},
					"skipped_steps":   []string{"kb_setup"},
					"business_type":   "salon",
				},
			},
			expectedCompletedSteps: []string{"signup", "channel_connect"},
			expectedSkippedSteps:   []string{"kb_setup"},
			expectedBusinessType:   ptr("salon"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := parseOnboardingState(tt.settings)
			assert.NotNil(t, state)
			assert.NotNil(t, state.CompletedSteps)
			assert.NotNil(t, state.SkippedSteps)
			assert.Equal(t, tt.expectedCompletedSteps, state.CompletedSteps)
			assert.Equal(t, tt.expectedSkippedSteps, state.SkippedSteps)
			if tt.expectedBusinessType != nil {
				assert.Equal(t, tt.expectedBusinessType, state.BusinessType)
			}
		})
	}
}

func TestMarshalAny(t *testing.T) {
	assert.Nil(t, marshalAny(nil))

	m := map[string]string{"foo": "bar"}
	b := marshalAny(m)
	assert.NotNil(t, b)

	var recovered map[string]string
	err := json.Unmarshal(b, &recovered)
	assert.NoError(t, err)
	assert.Equal(t, m, recovered)

	// Test with cyclic or invalid type
	assert.Nil(t, marshalAny(make(chan int)))
}

func TestSortedTemplates(t *testing.T) {
	// SortedTemplates must have the same count as Templates
	assert.Equal(t, len(onboarding.Templates), len(onboarding.SortedTemplates))

	// SortedTemplates must be strictly sorted by Type
	for i := 1; i < len(onboarding.SortedTemplates); i++ {
		assert.True(t, onboarding.SortedTemplates[i-1].Type < onboarding.SortedTemplates[i].Type,
			"Templates must be sorted in ascending order of Type")
	}

	// Verify types match expectations
	expectedTypes := []string{"home_services", "other", "photography", "salon", "tutoring"}
	actualTypes := make([]string, len(onboarding.SortedTemplates))
	for i, tmpl := range onboarding.SortedTemplates {
		actualTypes[i] = tmpl.Type
	}
	assert.Equal(t, expectedTypes, actualTypes)
}

func ptr[T any](v T) *T {
	return &v
}

func TestTestAIProviderConfig_Success(t *testing.T) {
	chatModels := make([]string, 0, 2)
	var embedCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		if r.URL.Path == "/chat/completions" {
			var payload struct {
				Model           string `json:"model"`
				ReasoningEffort string `json:"reasoning_effort"`
				MaxTokens       *int   `json:"max_tokens"`
				ResponseFormat  struct {
					Type       string `json:"type"`
					JSONSchema struct {
						Name   string `json:"name"`
						Strict bool   `json:"strict"`
						Schema struct {
							Type                 string         `json:"type"`
							Required             []string       `json:"required"`
							AdditionalProperties bool           `json:"additionalProperties"`
							Properties           map[string]any `json:"properties"`
						} `json:"schema"`
					} `json:"json_schema"`
				} `json:"response_format"`
			}
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			chatModels = append(chatModels, payload.Model)
			assert.Equal(t, "json_schema", payload.ResponseFormat.Type)
			assert.Equal(t, "provider_connection_test", payload.ResponseFormat.JSONSchema.Name)
			assert.True(t, payload.ResponseFormat.JSONSchema.Strict)
			assert.Equal(t, "object", payload.ResponseFormat.JSONSchema.Schema.Type)
			assert.Equal(t, []string{"ok"}, payload.ResponseFormat.JSONSchema.Schema.Required)
			assert.False(t, payload.ResponseFormat.JSONSchema.Schema.AdditionalProperties)
			assert.Contains(t, payload.ResponseFormat.JSONSchema.Schema.Properties, "ok")
			assert.Equal(t, "minimal", payload.ReasoningEffort)
			assert.Nil(t, payload.MaxTokens)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"model":"resolved-chat-model","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{\"ok\":true}"}}]}`))
			return
		}
		if r.URL.Path == "/embeddings" {
			embedCalled = true
			writeSuccessfulEmbedding(t, w)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, _ := New(nil, "test-key-exactly-32-bytes-padded")
	result, err := svc.TestAIProviderConfig(context.Background(), AIProviderConfig{
		APIKey:         "test-key",
		BaseURL:        srv.URL,
		AnalysisModel:  "analysis-model",
		ReplyModel:     "reply-model",
		EmbeddingModel: "test-embed",
	})
	assert.NoError(t, err)
	assert.True(t, result.OK)
	assert.Len(t, result.Checks, 3)
	assert.Equal(t, "resolved-chat-model", result.Checks[0].ResolvedModel)
	assert.ElementsMatch(t, []string{"analysis-model", "reply-model"}, chatModels)
	assert.True(t, embedCalled)
}

func TestTestAIProviderConfig_RequiresStructuredOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"[{\"ok\":true}]"}}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, _ := New(nil, "test-key-exactly-32-bytes-padded")
	result, err := svc.TestAIProviderConfig(context.Background(), AIProviderConfig{
		APIKey:         "test-key",
		BaseURL:        srv.URL,
		AnalysisModel:  "analysis-model",
		ReplyModel:     "reply-model",
		EmbeddingModel: "test-embed",
	})
	assert.NoError(t, err)
	assert.False(t, result.OK)
	assert.Equal(t, "schema_validation", result.Checks[0].Kind)
	assert.Equal(t, "Output did not match the required JSON schema", result.Checks[0].Message)
}

func TestTestAIProviderConfig_ChatErrorLeakedKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`[{"error":{"code":403,"message":"Your API key was reported as leaked. Please use another API key.","status":"PERMISSION_DENIED"}}]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, _ := New(nil, "test-key-exactly-32-bytes-padded")
	result, err := svc.TestAIProviderConfig(context.Background(), AIProviderConfig{
		APIKey:         "leaked-key",
		BaseURL:        srv.URL,
		AnalysisModel:  "analysis-model",
		ReplyModel:     "reply-model",
		EmbeddingModel: "test-embed",
	})
	assert.NoError(t, err)
	assert.False(t, result.OK)
	assert.Equal(t, "provider", result.Checks[0].Kind)
	assert.Contains(t, result.Checks[0].Message, "Your API key was reported as leaked. Please use another API key.")
}

func TestTestAIProviderConfig_EmbeddingError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"ok\":true}"}}]}`))
			return
		}
		if r.URL.Path == "/embeddings" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"Model 'unknown-embed' not found."}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, _ := New(nil, "test-key-exactly-32-bytes-padded")
	result, err := svc.TestAIProviderConfig(context.Background(), AIProviderConfig{
		APIKey:         "test-key",
		BaseURL:        srv.URL,
		AnalysisModel:  "analysis-model",
		ReplyModel:     "reply-model",
		EmbeddingModel: "unknown-embed",
	})
	assert.NoError(t, err)
	assert.False(t, result.OK)
	assert.Equal(t, "provider", result.Checks[2].Kind)
	assert.Contains(t, result.Checks[2].Message, "Model 'unknown-embed' not found.")
}

func TestTestAIProviderConfig_CleansFencedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/embeddings" {
			writeSuccessfulEmbedding(t, w)
			return
		}
		_, _ = w.Write([]byte("{\"choices\":[{\"finish_reason\":\"stop\",\"message\":{\"content\":\"<thought>done</thought>```json\\n{\\\"ok\\\":true}\\n```\"}}]}"))
	}))
	defer srv.Close()

	svc, _ := New(nil, "test-key-exactly-32-bytes-padded")
	result, err := svc.TestAIProviderConfig(context.Background(), testAIProviderConfig(srv.URL))
	assert.NoError(t, err)
	assert.True(t, result.OK)
}

func TestTestAIProviderConfig_ReportsTruncatedOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/embeddings" {
			writeSuccessfulEmbedding(t, w)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"MAX_TOKENS","message":{"content":""}}]}`))
	}))
	defer srv.Close()

	svc, _ := New(nil, "test-key-exactly-32-bytes-padded")
	result, err := svc.TestAIProviderConfig(context.Background(), testAIProviderConfig(srv.URL))
	assert.NoError(t, err)
	assert.False(t, result.OK)
	assert.Equal(t, "truncated", result.Checks[0].Kind)
	assert.Contains(t, result.Checks[0].Message, "truncated")
}

func TestTestAIProviderConfig_RetriesTransientFailure(t *testing.T) {
	analysisAttempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/embeddings" {
			writeSuccessfulEmbedding(t, w)
			return
		}
		var payload struct {
			Model string `json:"model"`
		}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		if payload.Model == "analysis-model" {
			analysisAttempts++
			if analysisAttempts == 1 {
				http.Error(w, "try again", http.StatusServiceUnavailable)
				return
			}
		}
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}"}}]}`))
	}))
	defer srv.Close()

	svc, _ := New(nil, "test-key-exactly-32-bytes-padded")
	svc.aiProviderTestRetryDelay = 0
	result, err := svc.TestAIProviderConfig(context.Background(), testAIProviderConfig(srv.URL))
	assert.NoError(t, err)
	assert.True(t, result.OK)
	assert.Equal(t, 2, analysisAttempts)
}

func TestTestAIProviderConfig_ReportsTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	svc, _ := New(nil, "test-key-exactly-32-bytes-padded")
	svc.aiProviderTestTimeout = time.Millisecond
	svc.aiProviderTestMaxRetries = 0
	result, err := svc.TestAIProviderConfig(context.Background(), testAIProviderConfig(srv.URL))
	assert.NoError(t, err)
	assert.False(t, result.OK)
	assert.Equal(t, "timeout", result.Checks[0].Kind)
	assert.Contains(t, result.Checks[0].Message, "timeout")
}

func testAIProviderConfig(baseURL string) AIProviderConfig {
	return AIProviderConfig{
		APIKey:         "real-test-key",
		BaseURL:        baseURL,
		AnalysisModel:  "analysis-model",
		ReplyModel:     "reply-model",
		EmbeddingModel: "embedding-model",
	}
}

func writeSuccessfulEmbedding(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	embedding := make([]float64, 1536)
	w.Header().Set("Content-Type", "application/json")
	assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
		"model": "resolved-embedding-model",
		"data":  []any{map[string]any{"embedding": embedding}},
	}))
}

func TestAIProviderConfigValidate(t *testing.T) {
	tests := []struct {
		name     string
		config   AIProviderConfig
		expected string
	}{
		{name: "missing key", config: AIProviderConfig{BaseURL: "https://api.openai.com/v1", AnalysisModel: "analysis", ReplyModel: "reply", EmbeddingModel: "embed"}, expected: "api key"},
		{name: "missing base url", config: AIProviderConfig{APIKey: "key", AnalysisModel: "analysis", ReplyModel: "reply", EmbeddingModel: "embed"}, expected: "base url"},
		{name: "missing analysis model", config: AIProviderConfig{APIKey: "key", BaseURL: "https://api.openai.com/v1", ReplyModel: "reply", EmbeddingModel: "embed"}, expected: "analysis model"},
		{name: "missing reply model", config: AIProviderConfig{APIKey: "key", BaseURL: "https://api.openai.com/v1", AnalysisModel: "analysis", EmbeddingModel: "embed"}, expected: "reply model"},
		{name: "missing embedding model", config: AIProviderConfig{APIKey: "key", BaseURL: "https://api.openai.com/v1", AnalysisModel: "analysis", ReplyModel: "reply"}, expected: "embedding model"},
		{name: "invalid base url scheme", config: AIProviderConfig{APIKey: "key", BaseURL: "ftp://api.openai.com", AnalysisModel: "analysis", ReplyModel: "reply", EmbeddingModel: "embed"}, expected: "http or https URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.normalized().validate(true)
			assert.ErrorContains(t, err, tt.expected)
		})
	}
}

func TestTestAIProviderConfig_BlocksSSRF(t *testing.T) {
	svc, err := New(nil, "test-key-exactly-32-bytes-padded")
	assert.NoError(t, err)

	ssrfTargets := []string{
		"http://169.254.169.254/latest/meta-data",
		"http://10.0.0.1:8080/v1",
		"http://172.16.0.1:5432/v1",
		"http://192.168.1.1:6379/v1",
	}

	for _, target := range ssrfTargets {
		t.Run(target, func(t *testing.T) {
			result, err := svc.TestAIProviderConfig(context.Background(), AIProviderConfig{
				APIKey:         "custom-user-supplied-key",
				BaseURL:        target,
				AnalysisModel:  "analysis-model",
				ReplyModel:     "reply-model",
				EmbeddingModel: "embed-model",
			})
			assert.NoError(t, err)
			assert.False(t, result.OK)
			// At least one check must fail due to SSRF block
			assert.NotEmpty(t, result.Checks)
			assert.Contains(t, result.Checks[0].Message, "SSRF protection")
		})
	}
}
