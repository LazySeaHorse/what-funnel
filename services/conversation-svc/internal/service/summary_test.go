package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

var summaryTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func TestBuildConversationSummary_ListSchemaOrdersAndLabels(t *testing.T) {
	settings := []byte(`{"summary_schema":[
		{"key":"next_action","label":"Next Step","description":"d"},
		{"key":"budget","description":"no label"},
		{"key":"missing_value","label":"Never stored"}]}`)
	stored := json.RawMessage(`{"budget":"5k","next_action":"call back","legacy_key":"old","a_legacy":"older"}`)

	got := buildConversationSummary(stored, settings, summaryTime, 3, 3)

	assert.Equal(t, []types.SummaryField{
		{Key: "next_action", Label: "Next Step", Value: "call back"},
		{Key: "budget", Label: "Budget", Value: "5k"},
		{Key: "a_legacy", Label: "a_legacy", Value: "older"},
		{Key: "legacy_key", Label: "legacy_key", Value: "old"},
	}, got.Fields)
	assert.Equal(t, summaryTime, got.GeneratedAt)
	assert.Equal(t, 3, got.MessageCountAtGeneration)
	assert.False(t, got.Stale)
}

func TestBuildConversationSummary_MapSchemaFromOnboarding(t *testing.T) {
	settings := []byte(`{"summary_schema":{"budget":"How much","preferred_timeframe":"When"}}`)
	got := buildConversationSummary(json.RawMessage(`{"budget":"1","preferred_timeframe":"soon"}`), settings, summaryTime, 1, 1)
	require.Len(t, got.Fields, 2)
	assert.Equal(t, "Budget", got.Fields[0].Label)
	assert.Equal(t, "Preferred Timeframe", got.Fields[1].Label)
}

func TestBuildConversationSummary_FallsBackToStoredKeys(t *testing.T) {
	for name, settings := range map[string]string{
		"no schema":    `{}`,
		"null":         `{"summary_schema":null}`,
		"scalar":       `{"summary_schema":"x"}`,
		"bad settings": `not json`,
		"empty":        ``,
	} {
		t.Run(name, func(t *testing.T) {
			got := buildConversationSummary(json.RawMessage(`{"b":"2","a":"1"}`), []byte(settings), summaryTime, 1, 1)
			assert.Equal(t, []types.SummaryField{{Key: "a", Label: "a", Value: "1"}, {Key: "b", Label: "b", Value: "2"}}, got.Fields)
		})
	}
}

func TestBuildConversationSummary_StaleAndNonStringValues(t *testing.T) {
	got := buildConversationSummary(json.RawMessage(`{"a":"x","b":5,"c":null}`), nil, summaryTime, 2, 3)
	assert.True(t, got.Stale)
	assert.Equal(t, []types.SummaryField{{Key: "a", Label: "a", Value: "x"}}, got.Fields)

	got = buildConversationSummary(json.RawMessage(`not json`), nil, summaryTime, 2, 2)
	assert.NotNil(t, got.Fields)
	assert.Empty(t, got.Fields)
	body, _ := json.Marshal(got)
	assert.Contains(t, string(body), `"fields":[]`)
}

func TestTitleFromKey(t *testing.T) {
	assert.Equal(t, "Customer Wants", titleFromKey("customer_wants"))
	assert.Equal(t, "Next Action", titleFromKey("NEXT_ACTION"))
	assert.Equal(t, "", titleFromKey("__"))
}
