package service

import (
	"encoding/json"
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
