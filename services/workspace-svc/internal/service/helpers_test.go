package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
		"bool_true":     true,
		"bool_false":    false,
		"string_val":    "true",
		"int_val":       1,
		"nil_val":       nil,
		"slice_val":     []string{"a"},
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
	var chatCalled, embedCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		if r.URL.Path == "/chat/completions" {
			chatCalled = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"pong"}}]}`))
			return
		}
		if r.URL.Path == "/embeddings" {
			embedCalled = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1, 0.2]}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, _ := New(nil, "test-key-exactly-32-bytes-padded")
	configJSON, err := json.Marshal(map[string]string{
		"api_key":          "test-key",
		"base_url":         srv.URL,
		"completion_model": "test-model",
		"embedding_model":  "test-embed",
	})
	assert.NoError(t, err)

	err = svc.TestAIProviderConfig(context.Background(), string(configJSON))
	assert.NoError(t, err)
	assert.True(t, chatCalled)
	assert.True(t, embedCalled)
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
	configJSON, err := json.Marshal(map[string]string{
		"api_key":          "leaked-key",
		"base_url":         srv.URL,
		"completion_model": "test-model",
		"embedding_model":  "test-embed",
	})
	assert.NoError(t, err)

	err = svc.TestAIProviderConfig(context.Background(), string(configJSON))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Your API key was reported as leaked. Please use another API key.")
}

func TestTestAIProviderConfig_EmbeddingError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"pong"}}]}`))
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
	configJSON, err := json.Marshal(map[string]string{
		"api_key":          "test-key",
		"base_url":         srv.URL,
		"completion_model": "test-model",
		"embedding_model":  "unknown-embed",
	})
	assert.NoError(t, err)

	err = svc.TestAIProviderConfig(context.Background(), string(configJSON))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Model 'unknown-embed' not found.")
}

