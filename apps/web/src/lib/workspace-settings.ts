export interface WorkspaceSettings {
	timezone?: string;
	language?: string;
	date_format?: string;
	time_format?: '12' | '24';
	business_type?: string;
	business_category?: string;
	business_phone?: string;
	business_email?: string;
	business_address?: string;
	business_website?: string;
	business_hours?: string;
	lead_tracking_enabled?: boolean;
	unassigned_conversations_visible_to_members?: boolean;
	ai_enabled?: boolean;
	ai_reply_mode_default?: 'auto_send' | 'draft_only';
	ai_greeting_text?: string;
	[key: string]: unknown;
}

function asString(value: unknown): string | undefined {
	return typeof value === 'string' ? value : undefined;
}

function asBoolean(value: unknown): boolean | undefined {
	return typeof value === 'boolean' ? value : undefined;
}

// The settings blob is free-form JSON, so time_format may arrive as the string
// '12'/'24' (what the settings form writes) or as a bare number.
function asTimeFormat(value: unknown): '12' | '24' | undefined {
	const normalized = typeof value === 'number' ? String(value) : value;
	return normalized === '12' || normalized === '24' ? normalized : undefined;
}

function asReplyMode(value: unknown): 'auto_send' | 'draft_only' | undefined {
	return value === 'auto_send' || value === 'draft_only' ? value : undefined;
}

function sanitizeSettings(obj: Record<string, unknown>): WorkspaceSettings {
	const known: Record<string, unknown> = {
		timezone: asString(obj.timezone),
		language: asString(obj.language),
		date_format: asString(obj.date_format),
		time_format: asTimeFormat(obj.time_format),
		business_type: asString(obj.business_type),
		business_category: asString(obj.business_category),
		business_phone: asString(obj.business_phone),
		business_email: asString(obj.business_email),
		business_address: asString(obj.business_address),
		business_website: asString(obj.business_website),
		business_hours: asString(obj.business_hours),
		lead_tracking_enabled: asBoolean(obj.lead_tracking_enabled),
		unassigned_conversations_visible_to_members: asBoolean(obj.unassigned_conversations_visible_to_members),
		ai_enabled: asBoolean(obj.ai_enabled),
		ai_reply_mode_default: asReplyMode(obj.ai_reply_mode_default),
		ai_greeting_text: asString(obj.ai_greeting_text)
	};
	// Start from the raw object, then overwrite known keys with their validated
	// value, or drop them entirely when invalid (never leave an explicit undefined).
	const result: Record<string, unknown> = { ...obj };
	for (const [key, value] of Object.entries(known)) {
		if (value === undefined) delete result[key];
		else result[key] = value;
	}
	return result as WorkspaceSettings;
}

export function decodeWorkspaceSettings(raw: unknown): WorkspaceSettings {
	if (!raw) return {};

	// 1. Native JSON object (standard API response)
	if (typeof raw === 'object' && !Array.isArray(raw)) {
		return sanitizeSettings(raw as Record<string, unknown>);
	}

	if (typeof raw !== 'string') {
		console.warn('[WorkspaceSettings] Unexpected settings payload type:', typeof raw);
		return {};
	}

	const trimmed = raw.trim();
	if (!trimmed) return {};

	// 2. Direct JSON string
	if (trimmed.startsWith('{')) {
		try {
			const parsed = JSON.parse(trimmed);
			if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
				return sanitizeSettings(parsed as Record<string, unknown>);
			}
		} catch (err) {
			console.error('[WorkspaceSettings] Failed to parse JSON settings:', err);
			return {};
		}
	}

	// 3. Legacy base64 string fallback (for backward compatibility during migration)
	try {
		const decoded = atob(trimmed);
		const parsed = JSON.parse(decoded);
		if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
			return sanitizeSettings(parsed as Record<string, unknown>);
		}
	} catch (err) {
		console.error('[WorkspaceSettings] Failed to decode base64 settings:', err);
	}

	return {};
}
