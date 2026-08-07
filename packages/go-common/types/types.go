package types

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Account is the root tenant boundary. One business per account.
type Account struct {
	ID                uuid.UUID  `json:"id" db:"id"`
	Name              string     `json:"name" db:"name"`
	Plan              string     `json:"plan" db:"plan"`
	ProductMode       string     `json:"product_mode" db:"product_mode"`
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

// Lead represents a lead associated with a conversation.
type Lead struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	AccountID       uuid.UUID  `json:"account_id" db:"account_id"`
	ConversationID  uuid.UUID  `json:"conversation_id" db:"conversation_id"`
	PipelineID      uuid.UUID  `json:"pipeline_id" db:"pipeline_id"`
	CurrentStateKey string     `json:"current_state_key" db:"current_state_key"`
	Tags            []string   `json:"tags" db:"tags"`
	CreatedBy       *uuid.UUID `json:"created_by" db:"created_by"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

// LeadNote is a note left on a lead.
type LeadNote struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	AccountID    uuid.UUID  `json:"account_id" db:"account_id"`
	LeadID       uuid.UUID  `json:"lead_id" db:"lead_id"`
	AuthorUserID *uuid.UUID `json:"author_user_id" db:"author_user_id"`
	AuthorEmail  string     `json:"author_email,omitempty"`
	Body         string     `json:"body" db:"body"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
}

// LeadStateHistory is the history of state changes for a lead.
type LeadStateHistory struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	AccountID uuid.UUID  `json:"account_id" db:"account_id"`
	LeadID    uuid.UUID  `json:"lead_id" db:"lead_id"`
	FromState *string    `json:"from_state" db:"from_state"`
	ToState   string     `json:"to_state" db:"to_state"`
	ChangedBy *uuid.UUID `json:"changed_by" db:"changed_by"`
	ActorEmail string     `json:"actor_email,omitempty"`
	ChangedAt time.Time  `json:"changed_at" db:"changed_at"`
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

// Channel represents a connected messaging surface.
type Channel struct {
	ID                uuid.UUID  `json:"id" db:"id"`
	AccountID         uuid.UUID  `json:"account_id" db:"account_id"`
	Type              string     `json:"type" db:"type"`
	BridgeIdentity    *string    `json:"bridge_identity" db:"bridge_identity"`
	BridgeCredentials []byte     `json:"-" db:"bridge_credentials"` // encrypted bytes
	Status            string     `json:"status" db:"status"`
	StatusDetail      *string    `json:"status_detail" db:"status_detail"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
}

// Contact represents a remote identity on a channel.
type Contact struct {
	ID                  uuid.UUID  `json:"id" db:"id"`
	AccountID           uuid.UUID  `json:"account_id" db:"account_id"`
	ChannelID           uuid.UUID  `json:"channel_id" db:"channel_id"`
	ExternalIdentity    string     `json:"external_identity" db:"external_identity"`
	DisplayName         *string    `json:"display_name" db:"display_name"`
	AvatarURL           *string    `json:"avatar_url" db:"avatar_url"`
	MergedIntoContactID *uuid.UUID `json:"merged_into_contact_id" db:"merged_into_contact_id"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
}

// Conversation is a thread with one contact on one channel.
type Conversation struct {
	ID              uuid.UUID   `json:"id" db:"id"`
	AccountID       uuid.UUID   `json:"account_id" db:"account_id"`
	ContactID       uuid.UUID   `json:"contact_id" db:"contact_id"`
	ChannelID       uuid.UUID   `json:"channel_id" db:"channel_id"`
	Status          string      `json:"status" db:"status"`
	AssignedUserIDs []uuid.UUID `json:"assigned_user_ids" db:"assigned_user_ids"`
	LastMessageAt   *time.Time  `json:"last_message_at" db:"last_message_at"`
	AIModeActive    bool        `json:"ai_mode_active" db:"ai_mode_active"`
	CreatedAt       time.Time   `json:"created_at" db:"created_at"`
}

// ConversationListItem is a list item representing a conversation with unread status and last message preview.
type ConversationListItem struct {
	Conversation
	Unread             bool      `json:"unread"`
	LastMessagePreview *Message  `json:"last_message_preview"` // Nullable
	ContactName        *string   `json:"contact_name"`
	ContactAvatarURL   *string   `json:"contact_avatar_url"`
	Lead               *Lead     `json:"lead"` // Nullable
}

// Message is an individual message in a conversation.
type Message struct {
	ID                uuid.UUID  `json:"id" db:"id"`
	AccountID         uuid.UUID  `json:"account_id" db:"account_id"`
	ConversationID    uuid.UUID  `json:"conversation_id" db:"conversation_id"`
	Direction         string     `json:"direction" db:"direction"`
	SenderType        string     `json:"sender_type" db:"sender_type"`
	SenderUserID      *uuid.UUID `json:"sender_user_id" db:"sender_user_id"`
	ContentType       string          `json:"content_type" db:"content_type"`
	Content           json.RawMessage `json:"content" db:"content"` // JSONB payload
	ExternalMessageID *string         `json:"external_message_id" db:"external_message_id"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
}
