export interface SummaryField {
	key: string;
	label: string;
	value: string;
}

export interface ConversationSummary {
	fields: SummaryField[];
	generated_at: string;
	message_count_at_generation: number;
	stale: boolean;
}

export interface SummaryUIState {
	/** Fetching the stored summary. */
	loading: boolean;
	/** A generation was requested and its websocket result has not arrived yet. */
	generating: boolean;
	error: string;
}

export const EMPTY_SUMMARY_STATE: SummaryUIState = { loading: false, generating: false, error: '' };

/** How long to wait for conversation.summary_updated before telling the user it is slow. */
export const SUMMARY_TIMEOUT_MS = 60_000;

export const SUMMARY_TIMEOUT_MESSAGE = 'The summary is taking longer than expected. Please try again.';
export const SUMMARY_REQUEST_FAILED_MESSAGE = 'Could not request a summary. Please try again.';
export const SUMMARY_LOAD_FAILED_MESSAGE = 'Could not load the summary. Please try again.';

/**
 * Validates a summary from the API. Anything malformed is treated as "no
 * summary" instead of crashing the panel.
 */
export function parseSummary(raw: unknown): ConversationSummary | null {
	if (!raw || typeof raw !== 'object') return null;
	const value = raw as Record<string, unknown>;
	if (!Array.isArray(value.fields) || typeof value.generated_at !== 'string') return null;
	const fields: SummaryField[] = [];
	for (const item of value.fields) {
		if (!item || typeof item !== 'object') continue;
		const field = item as Record<string, unknown>;
		if (typeof field.key !== 'string' || typeof field.value !== 'string') continue;
		fields.push({
			key: field.key,
			label: typeof field.label === 'string' && field.label ? field.label : field.key,
			value: field.value
		});
	}
	return {
		fields,
		generated_at: value.generated_at,
		message_count_at_generation: typeof value.message_count_at_generation === 'number' ? value.message_count_at_generation : 0,
		stale: value.stale === true
	};
}

export function isMissingSummaryValue(value: string): boolean {
	return value.trim().toUpperCase() === 'N/A';
}

export function formatSummaryTimestamp(iso: string, now: Date = new Date()): string {
	const date = new Date(iso);
	if (Number.isNaN(date.getTime())) return '';
	const time = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	return date.toDateString() === now.toDateString()
		? time
		: `${date.toLocaleDateString([], { day: 'numeric', month: 'short' })}, ${time}`;
}
