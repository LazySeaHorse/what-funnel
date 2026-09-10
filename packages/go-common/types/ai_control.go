package types

// AIState is the durable ownership state for a conversation's AI worker.
type AIState string

const (
	AIStateActive         AIState = "active"
	AIStatePausedHuman    AIState = "paused_human"
	AIStateCooldown       AIState = "cooldown"
	AIStateReviewRequired AIState = "review_required"
	AIStateBlockedSpam    AIState = "blocked_spam"
	AIStateBlockedManual  AIState = "blocked_manual"
)

func (s AIState) Valid() bool {
	switch s {
	case AIStateActive, AIStatePausedHuman, AIStateCooldown,
		AIStateReviewRequired, AIStateBlockedSpam, AIStateBlockedManual:
		return true
	default:
		return false
	}
}

func (s AIState) Blocked() bool {
	return s == AIStateBlockedSpam || s == AIStateBlockedManual
}

// AIStateReason records why an AI state transition occurred.
type AIStateReason string

const (
	AIStateReasonManualPause          AIStateReason = "manual_pause"
	AIStateReasonManualResume         AIStateReason = "manual_resume"
	AIStateReasonManualBlock          AIStateReason = "manual_block"
	AIStateReasonHumanMessageSent     AIStateReason = "human_message_sent"
	AIStateReasonExternalHumanMessage AIStateReason = "external_human_message"
	AIStateReasonConversationClosed   AIStateReason = "conversation_closed"
)

// AIReplyOverride is the per-conversation override for automatic replies.
type AIReplyOverride string

const (
	AIReplyOverrideInherit  AIReplyOverride = "inherit"
	AIReplyOverrideEnabled  AIReplyOverride = "enabled"
	AIReplyOverrideDisabled AIReplyOverride = "disabled"
)

func (o AIReplyOverride) Valid() bool {
	switch o {
	case AIReplyOverrideInherit, AIReplyOverrideEnabled, AIReplyOverrideDisabled:
		return true
	default:
		return false
	}
}

// AIRunState describes the execution state of the AI worker.
type AIRunState string

const (
	AIRunStateIdle     AIRunState = "idle"
	AIRunStateQueued   AIRunState = "queued"
	AIRunStateReplying AIRunState = "replying"
)

func (s AIRunState) Valid() bool {
	switch s {
	case AIRunStateIdle, AIRunStateQueued, AIRunStateReplying:
		return true
	default:
		return false
	}
}

// AIControlAction is a user-requested ownership transition.
type AIControlAction string

const (
	AIControlActionPause  AIControlAction = "pause"
	AIControlActionResume AIControlAction = "resume"
	AIControlActionBlock  AIControlAction = "block"
)

func (a AIControlAction) Valid() bool {
	switch a {
	case AIControlActionPause, AIControlActionResume, AIControlActionBlock:
		return true
	default:
		return false
	}
}
