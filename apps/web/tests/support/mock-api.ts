import type { Page } from '@playwright/test';

function encodeSettings(value: unknown) {
	const bytes = new TextEncoder().encode(JSON.stringify(value));
	return btoa(Array.from(bytes, (byte) => String.fromCharCode(byte)).join(''));
}

function decodeSettings(value: string): Record<string, unknown> {
	return JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(value), (char) => char.charCodeAt(0))));
}

const settings = encodeSettings({
	timezone: '(GMT+00:00) UTC',
	language: 'English',
	date_format: 'DD MMM YYYY',
	time_format: '12',
	business_category: 'Studio',
	business_phone: '+1 555 0100',
	business_email: 'studio@example.test',
	business_address: '1 Test Street',
	business_hours: 'Mon–Fri, 9am–5pm',
	lead_tracking_enabled: true,
	unassigned_conversations_visible_to_members: true
});

export interface MockWorkspaceOptions {
	role?: 'manager' | 'agent';
	productMode?: 'full_workspace' | 'chatbot_only';
	failures?: string[];
	conversations?: any[];
	replyDraft?: any | null;
	knowledge?: { concepts?: any[]; patterns?: any[] };
	aiConfigured?: boolean;
	autoReplyEnabled?: boolean;
}

export async function mockWorkspaceApi(page: Page, options: MockWorkspaceOptions = {}) {
	const role = options.role ?? 'manager';
	const failures = options.failures ?? [];
	const requests: Array<{ path: string; method: string; body?: Record<string, unknown> }> = [];
	let accountName = 'Test Workspace';
	let accountSlug = 'test-slug';
	let productMode: string = options.productMode ?? 'full_workspace';
	let accountSettings = settings;
	let users: Array<{ id: string; email: string; username: string; role: string; password?: string }> = [
		{ id: 'user-1', email: `${role}@example.test`, username: role, role }
	];
	let replyMode: string | null = null;
	let channels: Array<{ id: string; type: string; status: string; bridge_identity?: string }> = [];
	let bridgeConnections: Array<{ channel_id: string; platform: string; state: string; detail: string }> = [];
	let pipeline = { id: 'pipeline-1', name: 'Default pipeline', states: [{ key: 'new', label: 'New lead', color: '#0B6E99' }] };
	let aiConfigured = options.aiConfigured ?? false;
	if (options.autoReplyEnabled !== undefined) {
		accountSettings = encodeSettings({
			...decodeSettings(accountSettings),
			ai_enabled: true,
			ai_reply_mode_default: options.autoReplyEnabled ? 'auto_send' : 'draft_only'
		});
	}
	let ingestion: any = null;
	let knowledgeConcepts = options.knowledge?.concepts ?? [];
	let knowledgePatterns = options.knowledge?.patterns ?? [];

	await page.route('**/api-gateway/**', async (route) => {
		const request = route.request();
		const path = new URL(request.url()).pathname.replace('/api-gateway', '');
		const body = request.postDataJSON?.() as Record<string, unknown> | undefined;
		requests.push({ path, method: request.method(), body });

		if (failures.includes(path)) {
			await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'Service unavailable' }) });
			return;
		}

		const json = (body: unknown) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(body) });
		if (path === '/auth/me') return json({ id: 'user-1', user_id: 'user-1', email: `${role}@example.test`, username: role, role });
		if (path === '/workspace/account') {
			if (request.method() === 'GET') return json({ id: 'account-1', name: accountName, product_mode: productMode, settings: accountSettings });
			if (request.method() === 'PATCH') {
				accountName = String(body?.name || accountName);
				return json({ status: 'updated' });
			}
			return json({ status: 'updated' });
		}
		if (path === '/workspace/account/slug') {
			if (request.method() === 'GET') return json({ slug: accountSlug });
			if (request.method() === 'PUT') {
				accountSlug = String(body?.slug || accountSlug);
				return json({ slug: accountSlug });
			}
		}
		if (path === '/workspace/account/settings') {
			accountSettings = encodeSettings(request.method() === 'PATCH'
				? { ...decodeSettings(accountSettings), ...body }
				: body);
			return json({ status: 'updated' });
		}
		if (path === '/workspace/account/product-mode') {
			productMode = String(body?.product_mode || productMode);
			return json({ status: 'updated' });
		}
		if (path === '/workspace/account/ai-config/status') return json({ configured: aiConfigured });
		if (path === '/workspace/account/ai-config/test') {
			return json({ ok: true, message: 'AI provider connection verified successfully' });
		}
		if (path === '/workspace/account/ai-config' && request.method() === 'PUT') {
			aiConfigured = true;
			return json({ status: 'updated' });
		}
		if (path === '/workspace/users') {
			if (request.method() === 'GET') return json(users);
			if (request.method() === 'POST') {
				const username = String(body?.username || 'user');
				const newUser = {
					id: `user-${users.length + 1}`,
					email: String(body?.email || `${username}@example.test`),
					username,
					role: String(body?.role || 'agent'),
					password: String(body?.password || '')
				};
				users = [...users, newUser];
				return json(newUser);
			}
		}
		if (path === '/workspace/users/me/reply-mode') {
			if (request.method() === 'GET') {
				return json({
					reply_mode: replyMode,
					workspace_default: 'draft_only',
					effective_reply_mode: replyMode || 'draft_only',
					override_allowed: true
				});
			}
			replyMode = typeof body?.reply_mode === 'string' ? body.reply_mode : null;
			return json({ status: 'updated' });
		}
		if (path.startsWith('/workspace/users/') && path.endsWith('/role')) {
			users = users.map((user) => user.id === path.split('/')[3] ? { ...user, role: String(body?.role) } : user);
			return json({ status: 'updated' });
		}
		if (path.startsWith('/workspace/users/') && path.endsWith('/password')) {
			return json({ status: 'updated' });
		}
		if (path.startsWith('/workspace/users/') && request.method() === 'DELETE') {
			users = users.filter((u) => u.id !== path.split('/')[3]);
			return json({ status: 'deleted' });
		}
		if (path === '/channels') {
			return json(channels);
		}
		if (path.startsWith('/channels/') && path.endsWith('/disconnect')) {
			channels = channels.filter((channel) => channel.id !== path.split('/')[2]);
			bridgeConnections = bridgeConnections.filter((connection) => connection.channel_id !== path.split('/')[2]);
			return json({ status: 'disconnected' });
		}
		if (path === '/bridge-connections') {
			if (request.method() === 'POST') {
				const platform = String(body?.platform || 'whatsapp');
				const channel = { id: `channel-${channels.length + 1}`, type: `matrix_${platform}`, status: 'pending' };
				const connection = { channel_id: channel.id, platform, state: platform === 'telegram' ? 'awaiting_code' : 'awaiting_scan', detail: 'Complete the provider sign-in to finish connecting.' };
				channels = [...channels, channel];
				bridgeConnections = [...bridgeConnections, connection];
				return json(connection);
			}
			return json(bridgeConnections);
		}
		if (path === '/workspace/pipelines') return json([pipeline]);
		if (path.startsWith('/workspace/pipelines/') && request.method() === 'PUT') {
			pipeline = { ...pipeline, name: String(body?.name || pipeline.name), states: Array.isArray(body?.states) ? body.states as typeof pipeline.states : pipeline.states };
			return json(pipeline);
		}
		if (path === '/conversations') return json(options.conversations ?? []);
		if (/^\/conversations\/[^/]+$/.test(path)) {
			const conversationID = path.split('/')[2];
			return json((options.conversations ?? []).find((conversation) => conversation.id === conversationID) ?? {});
		}
		if (/^\/conversations\/[^/]+\/messages$/.test(path)) return json({ messages: [], next_cursor: null });
		if (/^\/conversations\/[^/]+\/reply-draft$/.test(path)) return json({ draft: options.replyDraft ?? null });
		if (/^\/conversations\/[^/]+\/ai-auto-reply$/.test(path) && request.method() === 'PATCH') {
			const conversation = (options.conversations ?? []).find((item) => item.id === path.split('/')[2]);
			if (conversation) conversation.ai_auto_reply_enabled = body?.enabled ?? null;
			return json({ ai_auto_reply_enabled: body?.enabled ?? null });
		}
		if (/^\/conversations\/[^/]+\/read$/.test(path)) return json({ status: 'read' });
		if (/^\/leads\/[^/]+\/(notes|history)$/.test(path)) return json([]);
		if (path === '/api/kb/concepts') return json({ concepts: knowledgeConcepts });
		if (path === '/api/kb/patterns') return json({ patterns: knowledgePatterns });
		if (path === '/api/kb/purge' && request.method() === 'DELETE') {
			const result = { cleared_concepts: knowledgeConcepts.length, cleared_patterns: knowledgePatterns.length };
			knowledgeConcepts = [];
			knowledgePatterns = [];
			return json({ success: true, ...result });
		}
		if (path === '/api/kb/suggestions') return json({ suggestions: [] });
		if (path === '/api/kb/mining-runs/latest') return json({ last_run: null });
		if (path === '/api/kb/ingestions/latest') return json({ ingestion });
		if (path === '/api/kb/ingestions' && request.method() === 'POST') {
			ingestion = { id: '81111111-1111-4111-8111-111111111111', status: 'queued', concepts: [], patterns: [] };
			return json(ingestion);
		}
		if (/^\/api\/kb\/ingestions\/[^/]+\/publish$/.test(path) && request.method() === 'POST') {
			ingestion = { ...ingestion, status: 'complete', concepts: body?.concepts ?? [], patterns: body?.patterns ?? [] };
			return json(ingestion);
		}
		if (/^\/api\/kb\/ingestions\/[^/]+$/.test(path)) {
			if (ingestion?.status === 'queued') {
				ingestion = {
					...ingestion,
					status: 'review_required',
					concepts: [{ id: '82111111-1111-4111-8111-111111111111', type: 'pricing', title: 'Pricing', tags: [], body_markdown: '$100 per hour.', status: 'draft' }],
					patterns: [{ id: '83111111-1111-4111-8111-111111111111', canonical_question: 'What does it cost?', answer_markdown: '$100 per hour.', trigger_phrases: ['pricing', 'how much', 'what does it cost'], status: 'draft' }]
				};
			}
			return json(ingestion);
		}
		if (path === '/onboarding/status') return json({ completed_at: '2026-01-01T00:00:00Z', skipped_steps: [] });
		return json({});
	});

	return { requests };
}

export async function mockOnboardingApi(
	page: Page,
	failures: string[] = [],
	configured = true,
	productMode: 'full_workspace' | 'chatbot_only' = 'full_workspace'
) {
	let accountName = 'Setup Studio';
	let accountSlug = 'setup-studio';
	let accountSettings: Record<string, unknown> = {};
	let aiConfigured = configured;
	let users: Array<{ id: string; username: string; role: string }> = [];
	let pipeline = {
		id: 'pipeline-setup',
		name: 'Default Pipeline',
		states: [{ key: 'new', label: 'New lead', color: '#3B82F6' }]
	};
	let ingestion: any = null;
	const requests: Array<{ path: string; method: string; body?: Record<string, unknown> }> = [];

	await page.route('**/api-gateway/**', async (route) => {
		const request = route.request();
		const path = new URL(request.url()).pathname.replace('/api-gateway', '');
		const method = request.method();
		const body = request.postDataJSON?.() as Record<string, unknown> | undefined;
		requests.push({ path, method, body });

		if (failures.includes(`${method} ${path}`)) {
			return route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'Setup service is unavailable' }) });
		}

		const json = (value: unknown) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(value) });
		if (path === '/auth/me') return json({ id: 'user-setup', role: 'manager' });
		if (path === '/workspace/account') {
			if (method === 'GET') return json({ id: 'account-setup', name: accountName, product_mode: productMode, settings: encodeSettings(accountSettings) });
			accountName = String(body?.name || accountName);
			return json({ status: 'updated' });
		}
		if (path === '/workspace/account/slug') {
			if (method === 'GET') return json({ slug: accountSlug });
			accountSlug = String(body?.slug || accountSlug);
			return json({ slug: accountSlug });
		}
		if (path === '/workspace/users') {
			if (method === 'GET') return json(users);
			if (method === 'POST') {
				const newUser = { id: `user-${users.length + 1}`, username: String(body?.username), role: String(body?.role || 'agent'), password: String(body?.password) };
				users = [...users, newUser];
				return json(newUser);
			}
		}
		if (path.startsWith('/workspace/users/') && method === 'DELETE') {
			users = users.filter(u => u.id !== path.split('/')[3]);
			return json({ status: 'deleted' });
		}
		if (path === '/workspace/account/settings' && method === 'PATCH') {
			accountSettings = { ...accountSettings, ...body };
			return json({ status: 'updated' });
		}
		if (path === '/workspace/account/ai-config/status') return json({ configured: aiConfigured });
		if (path === '/workspace/account/ai-config/test') return json({ ok: true, message: 'AI provider connection verified successfully' });
		if (path === '/workspace/account/ai-config' && method === 'PUT') {
			aiConfigured = true;
			return json({ status: 'updated' });
		}
		if (path === '/workspace/pipelines' && method === 'GET') return json([pipeline]);
		if (path === '/workspace/pipelines/pipeline-setup' && method === 'PUT') {
			pipeline = { ...pipeline, name: String(body?.name || pipeline.name), states: Array.isArray(body?.states) ? body.states as typeof pipeline.states : pipeline.states };
			return json({ status: 'updated' });
		}
		if (path === '/channels') return json([]);
		if (path === '/api/kb/ingestions/latest') return json({ ingestion });
		if (path === '/api/kb/ingestions' && method === 'POST') {
			ingestion = {
				id: '11111111-1111-4111-8111-111111111111',
				status: 'queued',
				concepts: [],
				patterns: []
			};
			return json(ingestion);
		}
		if (/^\/api\/kb\/ingestions\/[^/]+\/publish$/.test(path) && method === 'POST') {
			ingestion = { ...ingestion, status: 'complete', concepts: body?.concepts ?? ingestion.concepts, patterns: body?.patterns ?? ingestion.patterns };
			return json(ingestion);
		}
		if (/^\/api\/kb\/ingestions\/[^/]+$/.test(path)) {
			if (ingestion?.status === 'queued') {
				ingestion = {
					...ingestion,
					status: 'review_required',
					concepts: [
						{ id: '21111111-1111-4111-8111-111111111111', position: 0, type: 'service', title: 'Consulting', tags: [], body_markdown: 'Strategy consulting.', status: 'draft' },
						{ id: '31111111-1111-4111-8111-111111111111', position: 1, type: 'pricing', title: 'Pricing', tags: [], body_markdown: '$100 per hour.', status: 'draft' },
						{ id: '41111111-1111-4111-8111-111111111111', position: 2, type: 'hours', title: 'Hours', tags: [], body_markdown: 'Weekdays.', status: 'draft' },
						{ id: '51111111-1111-4111-8111-111111111111', position: 3, type: 'policy', title: 'Cancellation', tags: [], body_markdown: '24 hours notice.', status: 'draft' }
					],
					patterns: [
						{ id: '61111111-1111-4111-8111-111111111111', position: 0, canonical_question: 'What does consulting cost?', answer_markdown: '$100 per hour.', trigger_phrases: ['consulting price', 'what do you charge', 'hourly rate'], status: 'draft' },
						{ id: '71111111-1111-4111-8111-111111111111', position: 1, canonical_question: 'When are you open?', answer_markdown: 'We are open weekdays.', trigger_phrases: ['opening hours', 'when are you open', 'business hours'], status: 'draft' }
					]
				};
			}
			return json(ingestion);
		}
		if (path === '/onboarding/status') {
			if (method === 'GET') return json({ completed_steps: [], skipped_steps: [], completed_at: null });
			return json({ status: 'updated' });
		}
		return json({});
	});

	return { requests, getSettings: () => accountSettings, getPipeline: () => pipeline, isAIConfigured: () => aiConfigured };
}
