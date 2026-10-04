package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db/dbgen"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// SummaryService serves conversation summaries and requests new ones.
//
// Generation is owned by ai-answer-svc (it holds the provider credentials and
// the prompt/validation logic). This service never calls an LLM: a request is
// published to the conversation.summary_requested stream and the result comes
// back through conversation.summary_updated / conversation.summary_failed,
// which notification-svc forwards over the websocket.
type SummaryService struct {
	pool   *pgxpool.Pool
	pubsub *pubsub.Client
}

func NewSummaryService(pool *pgxpool.Pool, ps *pubsub.Client) *SummaryService {
	return &SummaryService{pool: pool, pubsub: ps}
}

// ConversationSummaryRequestedEvent is published to conversation.summary_requested.
type ConversationSummaryRequestedEvent struct {
	AccountID      uuid.UUID `json:"account_id"`
	ConversationID uuid.UUID `json:"conversation_id"`
	RequestedBy    uuid.UUID `json:"requested_by"`
}

// GetConversationSummary returns the stored summary or nil if none exists.
// Conversation visibility is checked first, so a user who cannot see the
// conversation gets the same not-found as for a missing one.
func (s *SummaryService) GetConversationSummary(ctx context.Context, accountID, userID, conversationID uuid.UUID, role string) (*types.ConversationSummary, error) {
	if err := canSeeConversation(ctx, s.pool, accountID, userID, conversationID, role); err != nil {
		return nil, err
	}
	return s.loadSummary(ctx, accountID, conversationID)
}

// RequestConversationSummary asks ai-answer-svc to (re)generate the summary.
// When the stored summary is already current nothing is published and the
// status is up_to_date. The caller always receives the current summary, if any.
func (s *SummaryService) RequestConversationSummary(ctx context.Context, accountID, userID, conversationID uuid.UUID, role string) (string, *types.ConversationSummary, error) {
	if err := canSeeConversation(ctx, s.pool, accountID, userID, conversationID, role); err != nil {
		return "", nil, err
	}
	current, err := s.loadSummary(ctx, accountID, conversationID)
	if err != nil {
		return "", nil, err
	}
	if current != nil && !current.Stale {
		return types.SummaryStatusUpToDate, current, nil
	}
	if _, err := s.pubsub.Publish(ctx, types.SummaryStreamRequested, ConversationSummaryRequestedEvent{
		AccountID: accountID, ConversationID: conversationID, RequestedBy: userID,
	}); err != nil {
		return "", nil, fmt.Errorf("publish summary request: %w", err)
	}
	return types.SummaryStatusQueued, current, nil
}

func (s *SummaryService) loadSummary(ctx context.Context, accountID, conversationID uuid.UUID) (*types.ConversationSummary, error) {
	q := dbgen.New(s.pool)
	row, err := q.GetConversationSummary(ctx, dbgen.GetConversationSummaryParams{ConversationID: conversationID, AccountID: accountID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get conversation summary: %w", err)
	}
	settings, err := q.GetAccountSettings(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get account settings: %w", err)
	}
	return buildConversationSummary(row.SummaryFields, settings, row.GeneratedAt, int(row.MessageCountAtGeneration), int(row.CurrentMessageCount)), nil
}

type schemaLabel struct{ key, label string }

// summarySchemaLabels reads accounts.settings.summary_schema, which is either
// a list of {key,label,...} objects or an onboarding-style {key: description}
// map, in document order. Anything else yields no labels.
func summarySchemaLabels(settings []byte) []schemaLabel {
	var wrapper struct {
		Schema json.RawMessage `json:"summary_schema"`
	}
	if err := json.Unmarshal(settings, &wrapper); err != nil || len(wrapper.Schema) == 0 {
		return nil
	}
	raw := bytes.TrimSpace(wrapper.Schema)
	var out []schemaLabel
	switch {
	case len(raw) > 0 && raw[0] == '[':
		var items []struct {
			Key   string `json:"key"`
			Label string `json:"label"`
		}
		if json.Unmarshal(raw, &items) != nil {
			return nil
		}
		for _, it := range items {
			if it.Key != "" {
				out = append(out, schemaLabel{it.Key, strings.TrimSpace(it.Label)})
			}
		}
	case len(raw) > 0 && raw[0] == '{':
		dec := json.NewDecoder(bytes.NewReader(raw))
		if _, err := dec.Token(); err != nil {
			return nil
		}
		for dec.More() {
			tok, err := dec.Token()
			if err != nil {
				return nil
			}
			key, _ := tok.(string)
			var skip json.RawMessage
			if dec.Decode(&skip) != nil {
				return nil
			}
			if key != "" {
				out = append(out, schemaLabel{key: key})
			}
		}
	}
	return out
}

func titleFromKey(key string) string {
	words := strings.Fields(strings.ReplaceAll(key, "_", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(words, " ")
}

// buildConversationSummary orders and labels stored values by the account's
// current schema; stored keys the schema no longer lists follow, sorted, with
// their key as the label. Schema keys with no stored value are omitted.
func buildConversationSummary(stored json.RawMessage, settings []byte, generatedAt time.Time, countAtGeneration, currentCount int) *types.ConversationSummary {
	values := map[string]string{}
	var asAny map[string]any
	if json.Unmarshal(stored, &asAny) == nil {
		for k, v := range asAny {
			if str, ok := v.(string); ok {
				values[k] = str
			}
		}
	}

	fields := make([]types.SummaryField, 0, len(values))
	used := map[string]bool{}
	for _, sl := range summarySchemaLabels(settings) {
		value, ok := values[sl.key]
		if !ok || used[sl.key] {
			continue
		}
		used[sl.key] = true
		label := sl.label
		if label == "" {
			label = titleFromKey(sl.key)
		}
		fields = append(fields, types.SummaryField{Key: sl.key, Label: label, Value: value})
	}
	var rest []string
	for k := range values {
		if !used[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		fields = append(fields, types.SummaryField{Key: k, Label: k, Value: values[k]})
	}

	return &types.ConversationSummary{
		Fields:                   fields,
		GeneratedAt:              generatedAt,
		MessageCountAtGeneration: countAtGeneration,
		Stale:                    currentCount > countAtGeneration,
	}
}
