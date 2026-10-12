export interface GlobalAutoReplyTogglePatch {
	ai_reply_mode_default: 'auto_send' | 'draft_only';
	ai_enabled?: boolean;
}

/**
 * Computes the settings patch needed to toggle global AI auto-reply.
 * When turning auto-reply ON, explicitly includes `ai_enabled: true`
 * so workspaces that completed onboarding in manual mode (or skipped AI)
 * are properly transitioned to an enabled state.
 */
export function computeGlobalAutoReplyPatch(params: {
	currentlyEnabled: boolean;
}): GlobalAutoReplyTogglePatch {
	const turningOn = !params.currentlyEnabled;
	const mode = turningOn ? 'auto_send' : 'draft_only';
	return {
		ai_reply_mode_default: mode,
		...(turningOn ? { ai_enabled: true } : {})
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
