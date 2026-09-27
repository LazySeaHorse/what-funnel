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
	[key: string]: unknown;
}

function sanitizeSettings(obj: Record<string, unknown>): WorkspaceSettings {
	return {
		...obj,
		timezone: typeof obj.timezone === 'string' ? obj.timezone : undefined,
		language: typeof obj.language === 'string' ? obj.language : undefined,
		date_format: typeof obj.date_format === 'string' ? obj.date_format : undefined,
		time_format: obj.time_format === '12' || obj.time_format === '24' ? obj.time_format : undefined,
		business_type: typeof obj.business_type === 'string' ? obj.business_type : undefined,
		business_category: typeof obj.business_category === 'string' ? obj.business_category : undefined,
		business_phone: typeof obj.business_phone === 'string' ? obj.business_phone : undefined,
		business_email: typeof obj.business_email === 'string' ? obj.business_email : undefined,
		business_address: typeof obj.business_address === 'string' ? obj.business_address : undefined,
		business_website: typeof obj.business_website === 'string' ? obj.business_website : undefined,
		business_hours: typeof obj.business_hours === 'string' ? obj.business_hours : undefined,
		lead_tracking_enabled:
			typeof obj.lead_tracking_enabled === 'boolean' ? obj.lead_tracking_enabled : undefined,
		unassigned_conversations_visible_to_members:
			typeof obj.unassigned_conversations_visible_to_members === 'boolean'
				? obj.unassigned_conversations_visible_to_members
				: undefined,
		ai_enabled: typeof obj.ai_enabled === 'boolean' ? obj.ai_enabled : undefined,
		ai_reply_mode_default:
			obj.ai_reply_mode_default === 'auto_send' || obj.ai_reply_mode_default === 'draft_only'
				? obj.ai_reply_mode_default
				: undefined
	};
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
