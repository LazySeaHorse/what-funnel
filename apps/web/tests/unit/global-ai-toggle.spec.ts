import { expect, test } from '@playwright/test';
import { computeGlobalAutoReplyPatch, isGlobalAutoReplyActive } from '../../src/lib/global-ai-toggle';
import { decodeWorkspaceSettings } from '../../src/lib/workspace-settings';

test.describe('Global AI auto-reply toggle state machine', () => {
	test('turning on auto-reply from skipped onboarding activates both ai_enabled and auto_send', () => {
		// Workspace settings after onboarding where user skipped AI / knowledge
		const initialSettings = decodeWorkspaceSettings({
			ai_enabled: false,
			ai_reply_mode_default: 'draft_only'
		});

		const currentlyActive = isGlobalAutoReplyActive({
			aiEnabled: initialSettings.ai_enabled === true,
			aiReplyModeDefault: initialSettings.ai_reply_mode_default === 'auto_send' ? 'auto_send' : 'draft_only'
		});
		expect(currentlyActive).toBe(false);

		// Compute toggle patch
		const patch = computeGlobalAutoReplyPatch({ currentlyEnabled: currentlyActive });

		// Must include ai_enabled: true to escape the disabled/manual onboarding state
		expect(patch.ai_reply_mode_default).toBe('auto_send');
		expect(patch.ai_enabled).toBe(true);

		// Merged settings must now evaluate isGlobalAutoReplyActive to true
		const mergedSettings = decodeWorkspaceSettings({
			...initialSettings,
			...patch
		});
		const newActiveState = isGlobalAutoReplyActive({
			aiEnabled: mergedSettings.ai_enabled === true,
			aiReplyModeDefault: mergedSettings.ai_reply_mode_default === 'auto_send' ? 'auto_send' : 'draft_only'
		});
		expect(newActiveState).toBe(true);
	});

	test('turning off auto-reply reverts to draft_only without breaking ai_enabled', () => {
		const activeSettings = decodeWorkspaceSettings({
			ai_enabled: true,
			ai_reply_mode_default: 'auto_send'
		});

		const currentlyActive = isGlobalAutoReplyActive({
			aiEnabled: activeSettings.ai_enabled === true,
			aiReplyModeDefault: 'auto_send'
		});
		expect(currentlyActive).toBe(true);

		const patch = computeGlobalAutoReplyPatch({ currentlyEnabled: currentlyActive });
		expect(patch.ai_reply_mode_default).toBe('draft_only');
		expect(patch.ai_enabled).toBeUndefined(); // do not disable AI workspace-wide, just switch mode
	});
});
