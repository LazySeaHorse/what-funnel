import { expect, test } from '@playwright/test';
import { decodeWorkspaceSettings } from '../../src/lib/workspace-settings';

test.describe('workspace-settings utility', () => {
	test('decodes native JSON object directly', () => {
		const input = {
			timezone: 'America/New_York',
			language: 'English',
			lead_tracking_enabled: true,
			ai_enabled: false,
			time_format: '24' as const,
			custom_key: 123
		};
		const result = decodeWorkspaceSettings(input);
		expect(result.timezone).toBe('America/New_York');
		expect(result.lead_tracking_enabled).toBe(true);
		expect(result.ai_enabled).toBe(false);
		expect(result.time_format).toBe('24');
		expect(result.custom_key).toBe(123);
	});

	test('decodes direct JSON string without throwing base64 DOMException', () => {
		const jsonString = JSON.stringify({
			timezone: 'Asia/Tokyo',
			language: 'Japanese',
			lead_tracking_enabled: false
		});
		const result = decodeWorkspaceSettings(jsonString);
		expect(result.timezone).toBe('Asia/Tokyo');
		expect(result.language).toBe('Japanese');
		expect(result.lead_tracking_enabled).toBe(false);
	});

	test('decodes legacy base64 encoded JSON string', () => {
		const original = {
			timezone: 'Europe/London',
			business_category: 'Consulting',
			lead_tracking_enabled: true
		};
		const base64Str = btoa(JSON.stringify(original));
		const result = decodeWorkspaceSettings(base64Str);
		expect(result.timezone).toBe('Europe/London');
		expect(result.business_category).toBe('Consulting');
		expect(result.lead_tracking_enabled).toBe(true);
	});

	test('handles null, undefined, empty string, and non-object inputs gracefully', () => {
		expect(decodeWorkspaceSettings(null)).toEqual({});
		expect(decodeWorkspaceSettings(undefined)).toEqual({});
		expect(decodeWorkspaceSettings('')).toEqual({});
		expect(decodeWorkspaceSettings('   ')).toEqual({});
		expect(decodeWorkspaceSettings(12345)).toEqual({});
		expect(decodeWorkspaceSettings([])).toEqual({});
	});

	test('handles corrupted JSON and base64 strings gracefully without throwing', () => {
		expect(decodeWorkspaceSettings('{corrupt-json')).toEqual({});
		expect(decodeWorkspaceSettings('not-valid-base64-or-json!@#$%')).toEqual({});
	});

	test('sanitizes field types safely', () => {
		const messyInput = {
			timezone: 12345, // invalid type
			lead_tracking_enabled: 'not-a-bool', // invalid type
			time_format: '48', // invalid enum
			ai_reply_mode_default: 'invalid_mode', // invalid enum
			language: 'French'
		};
		const result = decodeWorkspaceSettings(messyInput);
		expect(result.timezone).toBeUndefined();
		expect(result.lead_tracking_enabled).toBeUndefined();
		expect(result.time_format).toBeUndefined();
		expect(result.ai_reply_mode_default).toBeUndefined();
		expect(result.language).toBe('French');
	});
});
