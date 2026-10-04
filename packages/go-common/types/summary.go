package types

import "time"

// SummaryField is one labelled value of a conversation summary.
type SummaryField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value"`
}

// ConversationSummary is the read model served to the inbox. Stale means the
// conversation has messages newer than the ones the summary was generated from.
type ConversationSummary struct {
	Fields                   []SummaryField `json:"fields"`
	GeneratedAt              time.Time      `json:"generated_at"`
	MessageCountAtGeneration int            `json:"message_count_at_generation"`
	Stale                    bool           `json:"stale"`
}

// Summary request statuses returned by POST /conversations/{id}/summary.
const (
	SummaryStatusQueued    = "queued"
	SummaryStatusUpToDate  = "up_to_date"
	SummaryStreamRequested = "conversation.summary_requested"
	SummaryStreamUpdated   = "conversation.summary_updated"
	SummaryStreamFailed    = "conversation.summary_failed"
)
