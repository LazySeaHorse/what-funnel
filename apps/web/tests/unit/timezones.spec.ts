import { expect, test } from '@playwright/test';
import { formatTimeZoneLabel, normalizeSavedTimeZone, supportedTimeZones } from '../../src/lib/timezones';

test.describe('timezones utility', () => {
	test('supportedTimeZones provides UTC first followed by all IANA timezones', () => {
		expect(supportedTimeZones.length).toBeGreaterThan(100);
		expect(supportedTimeZones[0]).toBe('UTC');
		expect(supportedTimeZones).toContain('America/New_York');
		expect(supportedTimeZones).toContain('Europe/London');
		expect(supportedTimeZones).toContain('Asia/Tokyo');
		expect(supportedTimeZones).toContain('Asia/Colombo');
	});

	test('formatTimeZoneLabel produces formatted GMT offset and spaced slash names', () => {
		expect(formatTimeZoneLabel('UTC')).toBe('(GMT+00:00) UTC');
		const tokyo = formatTimeZoneLabel('Asia/Tokyo');
		expect(tokyo).toContain('Asia / Tokyo');
		expect(tokyo).toMatch(/\(GMT\+[0-9]{2}:[0-9]{2}\)/);
	});

	test('normalizeSavedTimeZone converts legacy formatted strings to canonical IANA IDs', () => {
		expect(normalizeSavedTimeZone('')).toBe('UTC');
		expect(normalizeSavedTimeZone('UTC')).toBe('UTC');
		expect(normalizeSavedTimeZone('(GMT+00:00) UTC')).toBe('UTC');
		expect(normalizeSavedTimeZone('(GMT+05:30) Asia / Colombo')).toBe('Asia/Colombo');
		expect(normalizeSavedTimeZone('Asia/Tokyo')).toBe('Asia/Tokyo');
		expect(normalizeSavedTimeZone('Unknown/Timezone')).toBe('Unknown/Timezone');
	});
});
