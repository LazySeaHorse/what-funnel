export type AIMode = 'off' | 'suggest' | 'auto';

export interface AIModePatch {
	ai_enabled: boolean;
	ai_reply_mode_default?: 'auto_send' | 'draft_only';
}

/** Derives the workspace-wide AI mode from the stored settings. */
export function aiModeFromSettings(settings: {
	ai_enabled?: boolean;
	ai_reply_mode_default?: 'auto_send' | 'draft_only';
}): AIMode {
	if (settings.ai_enabled !== true) return 'off';
	return settings.ai_reply_mode_default === 'auto_send' ? 'auto' : 'suggest';
}

/**
 * Settings patch for a mode. Turning AI off leaves ai_reply_mode_default
 * untouched so it is not clobbered by an unrelated choice.
 */
export function aiModePatch(mode: AIMode): AIModePatch {
	if (mode === 'off') return { ai_enabled: false };
	return {
		ai_enabled: true,
		ai_reply_mode_default: mode === 'auto' ? 'auto_send' : 'draft_only'
	};
}

/**
 * Determines if global AI auto-reply is currently active.
 * Requires both global AI to be enabled and the default reply mode to be auto_send.
 */
export function isGlobalAutoReplyActive(params: {
	aiEnabled: boolean;
	aiReplyModeDefault: 'auto_send' | 'draft_only';
}): boolean {
	return params.aiEnabled === true && params.aiReplyModeDefault === 'auto_send';
}
