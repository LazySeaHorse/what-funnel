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

export function decodeWorkspaceSettings(raw: unknown): WorkspaceSettings {
	if (!raw) return {};
	if (typeof raw === 'object') return raw as WorkspaceSettings;
	if (typeof raw !== 'string') return {};

	for (const candidate of [() => atob(raw), () => raw]) {
		try {
			const parsed = JSON.parse(candidate());
			if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) return parsed as WorkspaceSettings;
		} catch {
			// Accounts from older deployments may contain plain JSON or base64 JSON.
		}
	}
	return {};
}
