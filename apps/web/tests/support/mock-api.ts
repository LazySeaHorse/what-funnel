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
}

export async function mockWorkspaceApi(page: Page, options: MockWorkspaceOptions = {}) {
	const role = options.role ?? 'manager';
	const failures = options.failures ?? [];
	const requests: Array<{ path: string; method: string }> = [];
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
	let aiConfigured = false;

	await page.route('**/api-gateway/**', async (route) => {
		const request = route.request();
		const path = new URL(request.url()).pathname.replace('/api-gateway', '');
		const body = request.postDataJSON?.() as Record<string, unknown> | undefined;
		requests.push({ path, method: request.method() });

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
		if (/^\/conversations\/[^/]+\/read$/.test(path)) return json({ status: 'read' });
		if (/^\/leads\/[^/]+\/(notes|history)$/.test(path)) return json([]);
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
		if (path === '/onboarding/status') {
			if (method === 'GET') return json({ completed_steps: [], skipped_steps: [], completed_at: null });
			return json({ status: 'updated' });
		}
		return json({});
	});

	return { requests, getSettings: () => accountSettings, getPipeline: () => pipeline, isAIConfigured: () => aiConfigured };
}
