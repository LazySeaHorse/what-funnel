import { expect, test } from '@playwright/test';
import { formatSummaryTimestamp, isMissingSummaryValue, parseSummary } from '../../src/lib/inbox/summary';
import { getUICapabilities } from '../../src/lib/ui-capabilities';

test.describe('parseSummary', () => {
	test('accepts the API shape and defaults missing labels to the key', () => {
		const parsed = parseSummary({
			fields: [{ key: 'a', label: 'A label', value: 'x' }, { key: 'b', value: 'y' }],
			generated_at: '2026-01-02T03:04:05Z',
			message_count_at_generation: 4,
			stale: true
		});
		expect(parsed).toEqual({
			fields: [{ key: 'a', label: 'A label', value: 'x' }, { key: 'b', label: 'b', value: 'y' }],
			generated_at: '2026-01-02T03:04:05Z',
			message_count_at_generation: 4,
			stale: true
		});
	});

	test('treats malformed payloads as no summary and drops bad fields', () => {
		for (const bad of [null, undefined, 'x', 5, {}, { fields: 'no', generated_at: 'x' }, { fields: [] }]) {
			expect(parseSummary(bad)).toBeNull();
		}
		const parsed = parseSummary({ fields: [null, { key: 1, value: 'x' }, { key: 'ok', value: 'v' }, { key: 'n', value: 5 }], generated_at: 't' });
		expect(parsed?.fields).toEqual([{ key: 'ok', label: 'ok', value: 'v' }]);
		expect(parsed?.stale).toBe(false);
	});
});

test('missing-value and timestamp helpers', () => {
	expect(isMissingSummaryValue(' n/a ')).toBe(true);
	expect(isMissingSummaryValue('Not sure')).toBe(false);
	expect(formatSummaryTimestamp('garbage')).toBe('');
	const now = new Date('2026-05-05T12:00:00Z');
	expect(formatSummaryTimestamp('2026-05-05T09:30:00Z', now)).not.toContain(',');
	expect(formatSummaryTimestamp('2026-05-01T09:30:00Z', now)).toContain(',');
});

test('summary capability follows the side panel (full workspace only)', () => {
	expect(getUICapabilities({ product_mode: 'full_workspace' }, { role: 'agent' }).useConversationSummary).toBe(true);
	expect(getUICapabilities({ product_mode: 'chatbot_only' }, { role: 'manager' }).useConversationSummary).toBe(false);
});
