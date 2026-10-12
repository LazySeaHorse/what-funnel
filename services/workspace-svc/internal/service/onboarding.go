package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/onboarding"
)

// OnboardingState represents the current onboarding progress stored in
// accounts.settings["onboarding"].
type OnboardingState struct {
	BusinessType   *string  `json:"business_type"`   // "salon"|"photography"|"tutoring"|"home_services"|"other"|null
	CompletedSteps []string `json:"completed_steps"` // e.g. ["signup","mode_selected",...]
	SkippedSteps   []string `json:"skipped_steps"`   // e.g. ["channel_connect"]
	CompletedAt    *string  `json:"completed_at"`    // ISO timestamp or null
}

// GetOnboardingStatus reads the onboarding sub-key from accounts.settings.
// Pre-existing accounts with no onboarding key, or with settings that cannot be
// parsed, return an empty state — never 500.
func (svc *Service) GetOnboardingStatus(ctx context.Context, accountID uuid.UUID) (*OnboardingState, error) {
	var settingsRaw []byte
	err := svc.pool.QueryRow(ctx,
		`SELECT settings FROM accounts WHERE id = $1`, accountID).Scan(&settingsRaw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("account not found")
		}
		return nil, fmt.Errorf("get onboarding status: %w", err)
	}

	return parseOnboardingState(parseSettings(settingsRaw)), nil
}

// PatchOnboardingStatus atomically updates the onboarding sub-key in accounts.settings.
// action must be "complete" or "skip".
// If step == "done", completed_at is set to the current UTC timestamp.
func (svc *Service) PatchOnboardingStatus(ctx context.Context, accountID uuid.UUID, step, action string) error {
	if action != "complete" && action != "skip" {
		return fmt.Errorf("invalid action %q: must be \"complete\" or \"skip\"", action)
	}

	return audit.RunInTx(ctx, svc.pool, func(tx pgx.Tx) (audit.Entry, error) {
		err := updateSettingsLocked(ctx, tx, accountID, func(settings map[string]any) {
			state := parseOnboardingState(settings)

			switch action {
			case "complete":
				state.CompletedSteps = appendDedup(state.CompletedSteps, step)
				state.SkippedSteps = removeItem(state.SkippedSteps, step)
			case "skip":
				state.SkippedSteps = appendDedup(state.SkippedSteps, step)
				state.CompletedSteps = removeItem(state.CompletedSteps, step)
			}

			// step == "done" marks the entire onboarding flow as finished
			if step == "done" {
				ts := time.Now().UTC().Format(time.RFC3339)
				state.CompletedAt = &ts
			}

			settings["onboarding"] = state
		})
		if err != nil {
			return audit.Entry{}, err
		}

		return audit.Entry{
			AccountID:  accountID,
			Action:     "onboarding.step_updated",
			TargetType: audit.TargetAccount,
			TargetID:   &accountID,
			Metadata:   map[string]any{"step": step, "action": action},
		}, nil
	})
}

// updateSettingsLocked is the read-modify-write of accounts.settings: it locks
// the account row, parses the settings, lets mutate change them in place, and
// writes them back, all inside the caller's transaction.
func updateSettingsLocked(ctx context.Context, tx pgx.Tx, accountID uuid.UUID, mutate func(settings map[string]any)) error {
	var settingsRaw []byte
	err := tx.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1 FOR UPDATE`, accountID).Scan(&settingsRaw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("account not found")
		}
		return fmt.Errorf("read settings: %w", err)
	}

	settings := parseSettings(settingsRaw)
	mutate(settings)

	rawSettings, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE accounts SET settings = $1 WHERE id = $2`, rawSettings, accountID); err != nil {
		return fmt.Errorf("update settings: %w", err)
	}
	return nil
}

// GetOnboardingTemplates returns all business templates in stable sorted order.
// Pure in-memory; no database access.
func (svc *Service) GetOnboardingTemplates() []onboarding.BusinessTemplate {
	return onboarding.SortedTemplates
}

// ApplyOnboardingTemplate applies a business-type template to the account.
// It updates summary_schema in settings, sets onboarding.business_type,
// and — for a full-workspace account — updates the first pipeline's states and
// name to match the template.
func (svc *Service) ApplyOnboardingTemplate(ctx context.Context, accountID, actorID uuid.UUID, businessType, productMode string) error {
	tmpl, ok := onboarding.Templates[businessType]
	if !ok {
		return fmt.Errorf("unknown business type: %q", businessType)
	}

	return audit.RunInTx(ctx, svc.pool, func(tx pgx.Tx) (audit.Entry, error) {
		err := updateSettingsLocked(ctx, tx, accountID, func(settings map[string]any) {
			settings["summary_schema"] = tmpl.SummarySchema

			onboardingMap, ok := settings["onboarding"].(map[string]any)
			if !ok {
				onboardingMap = map[string]any{}
			}
			onboardingMap["business_type"] = businessType
			settings["onboarding"] = onboardingMap
		})
		if err != nil {
			return audit.Entry{}, err
		}

		// For full_workspace mode, update the first pipeline with template states
		if productMode == types.ProductModeFullWorkspace {
			var pipelineID uuid.UUID
			err = tx.QueryRow(ctx,
				`SELECT id FROM lead_pipelines WHERE account_id = $1 ORDER BY created_at ASC LIMIT 1`,
				accountID).Scan(&pipelineID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return audit.Entry{}, fmt.Errorf("query first pipeline: %w", err)
			}
			if err == nil {
				statesJSON, err := json.Marshal(tmpl.PipelineStates)
				if err != nil {
					return audit.Entry{}, fmt.Errorf("marshal pipeline states: %w", err)
				}
				_, err = tx.Exec(ctx,
					`UPDATE lead_pipelines SET name = $1, states = $2 WHERE id = $3 AND account_id = $4`,
					tmpl.Label, statesJSON, pipelineID, accountID)
				if err != nil {
					return audit.Entry{}, fmt.Errorf("update pipeline: %w", err)
				}

				if err := audit.NewWriterFromTx(tx).Write(ctx, audit.Entry{
					AccountID:   accountID,
					ActorUserID: &actorID,
					Action:      audit.ActionPipelineUpdated,
					TargetType:  audit.TargetPipeline,
					TargetID:    &pipelineID,
					Metadata:    map[string]any{"name": tmpl.Label, "template": businessType},
				}); err != nil {
					return audit.Entry{}, err
				}
			}
		}

		return audit.Entry{
			AccountID:   accountID,
			ActorUserID: &actorID,
			Action:      "onboarding.template_applied",
			TargetType:  audit.TargetAccount,
			TargetID:    &accountID,
			Metadata:    map[string]any{"business_type": businessType, "product_mode": productMode},
		}, nil
	})
}

// appendDedup appends item to slice only if it isn't already present.
func appendDedup(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

// removeItem returns a new slice with all occurrences of item removed.
func removeItem(slice []string, item string) []string {
	out := slice[:0:0]
	for _, s := range slice {
		if s != item {
			out = append(out, s)
		}
	}
	if out == nil {
		out = []string{}
	}
	return out
}

// parseOnboardingState extracts the "onboarding" sub-key from a parsed
// settings map and returns it as an *OnboardingState with non-nil slices.
func parseOnboardingState(settings map[string]any) *OnboardingState {
	state := &OnboardingState{
		CompletedSteps: []string{},
		SkippedSteps:   []string{},
	}
	raw, ok := settings["onboarding"]
	if !ok || raw == nil {
		return state
	}
	if b, err := json.Marshal(raw); err == nil {
		_ = json.Unmarshal(b, state)
	}
	if state.CompletedSteps == nil {
		state.CompletedSteps = []string{}
	}
	if state.SkippedSteps == nil {
		state.SkippedSteps = []string{}
	}
	return state
}
