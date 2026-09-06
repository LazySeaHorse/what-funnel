type IntlWithTimeZones = typeof Intl & { supportedValuesOf?: (key: 'timeZone') => string[] };

export const supportedTimeZones = (() => {
	const zones = (Intl as IntlWithTimeZones).supportedValuesOf?.('timeZone') ?? [];
	return ['UTC', ...zones.filter((zone) => zone !== 'UTC')];
})();

function normalizeGMTOffset(offset: string): string {
	if (offset === 'GMT' || offset === 'UTC') return 'GMT+00:00';
	const match = offset.match(/^(?:GMT|UTC)([+-])(\d{1,2})(?::(\d{2}))?$/);
	if (!match) return offset;
	return `GMT${match[1]}${match[2].padStart(2, '0')}:${match[3] ?? '00'}`;
}

export function formatTimeZoneLabel(zone: string): string {
	try {
		const offset = new Intl.DateTimeFormat('en-US', { timeZone: zone, timeZoneName: 'shortOffset' })
			.formatToParts(new Date()).find((part) => part.type === 'timeZoneName')?.value ?? 'GMT';
		return `(${normalizeGMTOffset(offset)}) ${zone.replaceAll('_', ' ').replaceAll('/', ' / ')}`;
	} catch {
		return zone;
	}
}

export function normalizeSavedTimeZone(value: string): string {
	if (value === 'UTC' || value.includes('UTC')) return 'UTC';
	const compactValue = value.replace(/\s*\/\s*/g, '/');
	return supportedTimeZones.find((zone) => compactValue.includes(zone)) ?? value;
}

