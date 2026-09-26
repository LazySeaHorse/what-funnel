package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAndNormalizeProductMode(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:        "empty defaults to full_workspace",
			input:       "",
			expected:    "full_workspace",
			expectError: false,
		},
		{
			name:        "explicit full_workspace",
			input:       "full_workspace",
			expected:    "full_workspace",
			expectError: false,
		},
		{
			name:        "explicit chatbot_only",
			input:       "chatbot_only",
			expected:    "chatbot_only",
			expectError: false,
		},
		{
			name:        "invalid mode rejected",
			input:       "unknown_mode",
			expected:    "",
			expectError: true,
		},
		{
			name:        "arbitrary string rejected",
			input:       "enterprise",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, err := validateAndNormalizeProductMode(tt.input)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid product mode: "+tt.input)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, mode)
			}
		})
	}
}

func TestBuildDefaultAccountSettings(t *testing.T) {
	t.Run("full_workspace enables lead tracking and sets defaults", func(t *testing.T) {
		data, err := buildDefaultAccountSettings("full_workspace")
		require.NoError(t, err)
		require.NotEmpty(t, data)

		var parsed map[string]any
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, true, parsed["ai_enabled"])
		assert.Equal(t, "draft_only", parsed["ai_reply_mode_default"])
		assert.Equal(t, true, parsed["allow_member_reply_mode_override"])
		assert.Equal(t, false, parsed["ai_may_auto_answer_mixed_conversations"])
		assert.Equal(t, true, parsed["lead_tracking_enabled"])

		schemas, ok := parsed["summary_schema"].([]any)
		require.True(t, ok, "summary_schema should be a slice")
		require.Len(t, schemas, 4)

		keys := make([]string, 0, len(schemas))
		for _, s := range schemas {
			m, ok := s.(map[string]any)
			require.True(t, ok)
			keys = append(keys, m["key"].(string))
		}
		assert.Equal(t, []string{"customer_wants", "preferred_timeframe", "objections", "next_action"}, keys)
	})

	t.Run("chatbot_only disables lead tracking", func(t *testing.T) {
		data, err := buildDefaultAccountSettings("chatbot_only")
		require.NoError(t, err)
		require.NotEmpty(t, data)

		var parsed map[string]any
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, false, parsed["lead_tracking_enabled"])
		assert.Equal(t, true, parsed["ai_enabled"])
	})
}
