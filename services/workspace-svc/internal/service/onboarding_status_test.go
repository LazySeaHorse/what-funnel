package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GetOnboardingStatus must never fail on odd stored settings and must always
// return non-nil step slices.
func TestOnboardingStatus_OddStoredSettings(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	cases := []struct {
		name         string
		settings     string
		wantBusiness *string
		wantComplete []string
	}{
		{"no onboarding key", `{"ai_enabled": true}`, nil, []string{}},
		{"onboarding null", `{"onboarding": null}`, nil, []string{}},
		{"null step lists", `{"onboarding": {"business_type": "salon", "completed_steps": null, "skipped_steps": null}}`, strPtr("salon"), []string{}},
		{"wrong onboarding type", `{"onboarding": "oops"}`, nil, []string{}},
		{"populated", `{"onboarding": {"completed_steps": ["signup"]}}`, nil, []string{"signup"}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			accountID, _ := setupTestTenant(t, pool, "OnboardingOdd"+tc.name, "ob_odd_"+string(rune('a'+i))+"@example.com")
			_, err := pool.Exec(ctx, `UPDATE accounts SET settings = $1::jsonb WHERE id = $2`, tc.settings, accountID)
			require.NoError(t, err)

			state, err := svc.GetOnboardingStatus(ctx, accountID)
			require.NoError(t, err)
			require.NotNil(t, state)
			assert.Equal(t, tc.wantBusiness, state.BusinessType)
			assert.Equal(t, tc.wantComplete, state.CompletedSteps)
			assert.Equal(t, []string{}, state.SkippedSteps)
			assert.Nil(t, state.CompletedAt)
		})
	}
}

func strPtr(s string) *string { return &s }
