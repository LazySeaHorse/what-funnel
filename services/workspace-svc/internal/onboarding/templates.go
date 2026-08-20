// Package onboarding holds static business-type onboarding templates.
// Templates are pure in-memory configuration — no database access required.
package onboarding

import (
	"sort"

	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)


// KBPrompt is a knowledge-base question shown during onboarding.
type KBPrompt struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder"`
}

// BusinessTemplate is the onboarding configuration for a specific business type.
type BusinessTemplate struct {
	Type           string                `json:"type"`
	Label          string                `json:"label"`
	PipelineStates []types.PipelineState `json:"pipeline_states"`
	SummarySchema  map[string]string     `json:"summary_schema"` // field_key -> description
	KBPrompts      []KBPrompt            `json:"kb_prompts"`
}

// Templates is the static map of all supported business-type templates.
var Templates = map[string]BusinessTemplate{
	"salon": {
		Type:  "salon",
		Label: "Salon / Beauty",
		PipelineStates: []types.PipelineState{
			{Key: "new_inquiry", Label: "New Inquiry", Color: "#6366f1"},
			{Key: "service_selected", Label: "Service Selected", Color: "#8b5cf6"},
			{Key: "date_requested", Label: "Date Requested", Color: "#f59e0b"},
			{Key: "confirmed", Label: "Confirmed", Color: "#10b981"},
			{Key: "completed", Label: "Completed", Color: "#6b7280"},
		},
		SummarySchema: map[string]string{
			"service_interest": "Which service the customer is interested in",
			"preferred_date":   "Preferred appointment date/time",
			"contact_name":     "Customer name",
		},
		KBPrompts: []KBPrompt{
			{ID: "hours", Label: "What are your hours?", Placeholder: "e.g. Mon\u2013Sat 9am\u20137pm, closed Sundays"},
			{ID: "services", Label: "What services and prices do you offer?", Placeholder: "e.g. Haircut $45, Highlights from $80..."},
			{ID: "policy", Label: "What's your cancellation/rescheduling policy?", Placeholder: "e.g. 24 hours notice required..."},
			{ID: "faq", Label: "Anything else customers often ask?", Placeholder: "e.g. Do you accept walk-ins? Is parking available?"},
		},
	},
	"photography": {
		Type:  "photography",
		Label: "Photography",
		PipelineStates: []types.PipelineState{
			{Key: "new_lead", Label: "New Lead", Color: "#6366f1"},
			{Key: "package_sent", Label: "Package Sent", Color: "#8b5cf6"},
			{Key: "follow_up", Label: "Follow-up", Color: "#f59e0b"},
			{Key: "booked", Label: "Booked", Color: "#10b981"},
			{Key: "delivered", Label: "Delivered", Color: "#6b7280"},
		},
		SummarySchema: map[string]string{
			"event_type":       "Type of photography session (wedding, portrait, etc.)",
			"event_date":       "Date of the event or session",
			"package_interest": "Which package they're interested in",
			"contact_name":     "Client name",
		},
		KBPrompts: []KBPrompt{
			{ID: "packages", Label: "What photography packages do you offer?", Placeholder: "e.g. Portrait session $299 (2hr, 30 edited photos)..."},
			{ID: "style", Label: "What's your photography style?", Placeholder: "e.g. Natural light, candid documentary style..."},
			{ID: "booking", Label: "How does your booking process work?", Placeholder: "e.g. 50% deposit to hold date, balance due 2 weeks before..."},
			{ID: "delivery", Label: "How do you deliver photos?", Placeholder: "e.g. Online gallery within 4 weeks of session..."},
		},
	},
	"tutoring": {
		Type:  "tutoring",
		Label: "Tutoring / Education",
		PipelineStates: []types.PipelineState{
			{Key: "inquiry", Label: "Inquiry", Color: "#6366f1"},
			{Key: "trial_scheduled", Label: "Trial Scheduled", Color: "#8b5cf6"},
			{Key: "trial_complete", Label: "Trial Complete", Color: "#f59e0b"},
			{Key: "enrolled", Label: "Enrolled", Color: "#10b981"},
			{Key: "inactive", Label: "Inactive", Color: "#6b7280"},
		},
		SummarySchema: map[string]string{
			"subject_interest":   "Subject(s) the student needs help with",
			"student_grade":      "Student's grade/year level",
			"preferred_schedule": "Preferred session times",
			"contact_name":       "Parent/guardian name",
		},
		KBPrompts: []KBPrompt{
			{ID: "subjects", Label: "What subjects do you tutor?", Placeholder: "e.g. Math (K-12), SAT prep, Python programming..."},
			{ID: "rates", Label: "What are your rates?", Placeholder: "e.g. $60/hr for one-on-one, $40/hr for group sessions..."},
			{ID: "schedule", Label: "How do sessions work?", Placeholder: "e.g. 1hr sessions online via Zoom, flexible scheduling..."},
			{ID: "faq", Label: "Common questions from parents?", Placeholder: "e.g. Do you offer a free trial? How do I track my child's progress?"},
		},
	},
	"home_services": {
		Type:  "home_services",
		Label: "Home Services",
		PipelineStates: []types.PipelineState{
			{Key: "new_request", Label: "New Request", Color: "#6366f1"},
			{Key: "quote_sent", Label: "Quote Sent", Color: "#8b5cf6"},
			{Key: "scheduled", Label: "Scheduled", Color: "#f59e0b"},
			{Key: "completed", Label: "Completed", Color: "#10b981"},
		},
		SummarySchema: map[string]string{
			"service_type":     "Type of service requested",
			"property_address": "Property/job address",
			"preferred_date":   "Preferred service date",
			"contact_name":     "Customer name",
		},
		KBPrompts: []KBPrompt{
			{ID: "services", Label: "What services do you offer?", Placeholder: "e.g. Lawn care, gutter cleaning, pressure washing..."},
			{ID: "pricing", Label: "How does your pricing work?", Placeholder: "e.g. Free estimates, prices vary by job size..."},
			{ID: "area", Label: "What area do you serve?", Placeholder: "e.g. Austin, TX and surrounding suburbs within 30 miles..."},
			{ID: "booking", Label: "How do customers book?", Placeholder: "e.g. Call or text for a free estimate, same-week scheduling available..."},
		},
	},
	"other": {
		Type:  "other",
		Label: "Other Business",
		PipelineStates: []types.PipelineState{
			{Key: "new", Label: "New", Color: "#6366f1"},
			{Key: "contacted", Label: "Contacted", Color: "#8b5cf6"},
			{Key: "follow_up", Label: "Follow-up", Color: "#f59e0b"},
			{Key: "won", Label: "Won", Color: "#10b981"},
			{Key: "lost", Label: "Lost", Color: "#ef4444"},
		},
		SummarySchema: map[string]string{
			"inquiry_type":        "What the customer is asking about",
			"preferred_timeframe": "When they need the service",
			"contact_name":        "Customer name",
		},
		KBPrompts: []KBPrompt{
			{ID: "about", Label: "What does your business do?", Placeholder: "Describe your main products or services..."},
			{ID: "hours", Label: "What are your hours and how can people reach you?", Placeholder: "e.g. Mon\u2013Fri 9am\u20135pm, phone, email..."},
			{ID: "pricing", Label: "How does pricing work?", Placeholder: "e.g. Quotes available on request, starting from..."},
			{ID: "faq", Label: "What do customers often ask?", Placeholder: "Your most common questions and answers..."},
		},
	},
}

// SortedTemplates is Templates as a slice sorted by Type, computed once at
// package init time. Use this instead of iterating Templates directly when a
// stable, sorted list is needed — avoids a per-call map iteration + sort.
var SortedTemplates []BusinessTemplate

func init() {
	SortedTemplates = make([]BusinessTemplate, 0, len(Templates))
	for _, t := range Templates {
		SortedTemplates = append(SortedTemplates, t)
	}
	sort.Slice(SortedTemplates, func(i, j int) bool {
		return SortedTemplates[i].Type < SortedTemplates[j].Type
	})
}
