import { expect, test } from '@playwright/test';
import { aiModeFromSettings, aiModePatch, isGlobalAutoReplyActive } from '../../src/lib/ai-reply-mode';
import { decodeWorkspaceSettings } from '../../src/lib/workspace-settings';

test.describe('AI mode mapping', () => {
	test('derives the mode from stored settings', () => {
		expect(aiModeFromSettings({ ai_enabled: false, ai_reply_mode_default: 'auto_send' })).toBe('off');
		expect(aiModeFromSettings({})).toBe('off');
		expect(aiModeFromSettings({ ai_enabled: true, ai_reply_mode_default: 'draft_only' })).toBe('suggest');
		expect(aiModeFromSettings({ ai_enabled: true })).toBe('suggest');
		expect(aiModeFromSettings({ ai_enabled: true, ai_reply_mode_default: 'auto_send' })).toBe('auto');
	});

	test('every mode round-trips through its patch', () => {
		for (const mode of ['off', 'suggest', 'auto'] as const) {
			// Start from the opposite state so the patch has to do real work.
			const initial = {
				ai_enabled: mode === 'off',
				ai_reply_mode_default: mode === 'auto' ? 'draft_only' : 'auto_send'
			};
			const merged = decodeWorkspaceSettings({ ...initial, ...aiModePatch(mode) });
			expect(aiModeFromSettings(merged)).toBe(mode);
		}
	});

	test('leaving manual mode re-enables AI', () => {
		expect(aiModePatch('suggest')).toEqual({ ai_enabled: true, ai_reply_mode_default: 'draft_only' });
		expect(aiModePatch('auto')).toEqual({ ai_enabled: true, ai_reply_mode_default: 'auto_send' });
	});

	test('turning AI off does not touch the reply mode default', () => {
		expect(aiModePatch('off')).toEqual({ ai_enabled: false });
	});

	test('auto-reply is only active when AI is enabled and in auto_send', () => {
		expect(isGlobalAutoReplyActive({ aiEnabled: true, aiReplyModeDefault: 'auto_send' })).toBe(true);
		expect(isGlobalAutoReplyActive({ aiEnabled: false, aiReplyModeDefault: 'auto_send' })).toBe(false);
		expect(isGlobalAutoReplyActive({ aiEnabled: true, aiReplyModeDefault: 'draft_only' })).toBe(false);
	});
});
