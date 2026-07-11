package types

import (
	"time"

	"github.com/google/uuid"
)

// Account is the root tenant boundary. One business per account.
type Account struct {
	ID                uuid.UUID  `json:"id" db:"id"`
	Name              string     `json:"name" db:"name"`
	Plan              string     `json:"plan" db:"plan"`
	AIProviderConfig  []byte     `json:"-" db:"ai_provider_config"` // encrypted; never serialised to JSON directly
	Settings          []byte     `json:"settings" db:"settings"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
}

// User belongs to exactly one account and has a role: admin or member.
type User struct {
	ID           uuid.UUID `json:"id" db:"id"`
	AccountID    uuid.UUID `json:"account_id" db:"account_id"`
	Email        string    `json:"email" db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"` // managed by authboss; never serialised
	Role         string    `json:"role" db:"role"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// AuditLog records every state-changing action.
type AuditLog struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	AccountID   uuid.UUID  `json:"account_id" db:"account_id"`
	ActorUserID *uuid.UUID `json:"actor_user_id" db:"actor_user_id"`
	Action      string     `json:"action" db:"action"`
	TargetType  string     `json:"target_type" db:"target_type"`
	TargetID    *uuid.UUID `json:"target_id" db:"target_id"`
	Metadata    []byte     `json:"metadata" db:"metadata"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}

// PipelineState is one state in a lead pipeline.
type PipelineState struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Color string `json:"color"`
}

// LeadPipeline is the admin-configured ordered list of lead states.
type LeadPipeline struct {
	ID        uuid.UUID       `json:"id" db:"id"`
	AccountID uuid.UUID       `json:"account_id" db:"account_id"`
	Name      string          `json:"name" db:"name"`
	States    []PipelineState `json:"states" db:"states"` // serialised as JSONB
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
}

// InviteToken holds a pending user invite.
type InviteToken struct {
	Token     string    `json:"token"`
	AccountID uuid.UUID `json:"account_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// ContextKey is a typed key for values stored in request context.
type ContextKey string

const (
	ContextKeyAccountID ContextKey = "account_id"
	ContextKeyUserID    ContextKey = "user_id"
	ContextKeyUserRole  ContextKey = "user_role"
)

// Role constants.
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// Plan constants.
const (
	PlanSelfHosted = "self_hosted"
)

// DefaultPipelineStates are seeded for every new account.
var DefaultPipelineStates = []PipelineState{
	{Key: "new", Label: "New", Color: "#6366f1"},
	{Key: "contacted", Label: "Contacted", Color: "#3b82f6"},
	{Key: "follow_up", Label: "Follow-up", Color: "#f59e0b"},
	{Key: "won", Label: "Won", Color: "#22c55e"},
	{Key: "lost", Label: "Lost", Color: "#ef4444"},
}
