import { apiRequest } from '$lib/api';

export interface AuditLogEntry {
	id: string;
	actor_user_id: string | null;
	actor_name: string | null;
	actor_email: string | null;
	action: string;
	target_type: string;
	target_id: string | null;
	metadata: Record<string, unknown>;
	created_at: string;
}

export interface AuditLogPage {
	items: AuditLogEntry[];
	next_cursor: string | null;
}

export interface AuditLogQuery {
	limit?: number;
	cursor?: string | null;
	actorUserID?: string;
	action?: string;
	/** RFC3339 timestamps. */
	from?: string;
	to?: string;
}

export const AUDIT_LOG_PAGE_SIZE = 50;

/** GET /workspace/audit-logs (admin only). Empty filters are omitted. */
export async function fetchAuditLogs(query: AuditLogQuery = {}, signal?: AbortSignal): Promise<AuditLogPage> {
	const params = new URLSearchParams();
	params.set('limit', String(query.limit ?? AUDIT_LOG_PAGE_SIZE));
	if (query.cursor) params.set('cursor', query.cursor);
	if (query.actorUserID) params.set('actor_user_id', query.actorUserID);
	if (query.action?.trim()) params.set('action', query.action.trim());
	if (query.from) params.set('from', query.from);
	if (query.to) params.set('to', query.to);
	const res = await apiRequest(`/workspace/audit-logs?${params}`, { signal });
	return {
		items: Array.isArray(res?.items) ? res.items : [],
		next_cursor: typeof res?.next_cursor === 'string' && res.next_cursor ? res.next_cursor : null
	};
}

/** "lead.state_changed" -> "Lead state changed". */
export function auditActionLabel(action: string): string {
	const text = action.replace(/[._-]+/g, ' ').trim();
	return text ? text.charAt(0).toUpperCase() + text.slice(1) : 'Unknown action';
}

/** Who performed the entry: a deleted actor keeps its id but loses name/email. */
export function auditActorLabel(entry: Pick<AuditLogEntry, 'actor_user_id' | 'actor_name' | 'actor_email'>): string {
	if (!entry.actor_user_id) return 'System';
	return entry.actor_name?.trim() || entry.actor_email?.trim() || 'Deleted user';
}

function summarizeValue(value: unknown): string {
	if (value === null || value === undefined) return '';
	if (typeof value === 'object') return JSON.stringify(value);
	return String(value);
}

/** One-line "key: value" summary of the first few metadata fields. */
export function auditMetadataSummary(metadata: Record<string, unknown> | null | undefined, max = 3): string {
	const entries = Object.entries(metadata ?? {}).filter(([, v]) => v !== null && v !== undefined && v !== '');
	if (entries.length === 0) return '';
	const shown = entries.slice(0, max).map(([k, v]) => `${k.replace(/_/g, ' ')}: ${summarizeValue(v)}`);
	const more = entries.length - shown.length;
	return shown.join(', ') + (more > 0 ? ` +${more} more` : '');
}
