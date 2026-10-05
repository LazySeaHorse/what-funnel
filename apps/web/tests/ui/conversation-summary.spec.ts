import { expect, test, type Page, type WebSocketRoute } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

const conversation = {
	id: 'conversation-1',
	status: 'open',
	assigned_user_ids: ['user-1'],
	created_at: '2026-01-01T12:00:00Z',
	last_message_at: '2026-01-01T12:00:00Z',
	channel_type: 'whatsapp',
	contact: { display_name: 'Rina Patel', external_identity: '+15550122' },
	lead: { id: 'lead-1', current_state_key: 'new', tags: [] }
};

const summary = (overrides: Record<string, unknown> = {}) => ({
	fields: [
		{ key: 'customer_wants', label: 'Customer Wants', value: 'A kitchen remodel' },
		{ key: 'preferred_timeframe', label: 'Preferred Timeframe', value: 'N/A' },
		{ key: 'next_action', label: 'Next Step', value: 'Send a quote' }
	],
	generated_at: new Date().toISOString(),
	message_count_at_generation: 3,
	stale: false,
	...overrides
});

/** Captures the app's websocket so a test can push server events. */
async function captureWebSocket(page: Page) {
	const sockets: WebSocketRoute[] = [];
	await page.routeWebSocket(/\/ws(\?.*)?$/, (ws) => {
		sockets.push(ws);
	});
	return {
		async push(event: Record<string, unknown>) {
			await expect.poll(() => sockets.length).toBeGreaterThan(0);
			sockets[sockets.length - 1].send(JSON.stringify(event));
		}
	};
}

const panel = (page: Page) => page.locator('.lead-panel');

test.describe('conversation summary', () => {
	test('click requests generation, shows loading, then renders the pushed summary', async ({ page }) => {
		const ws = await captureWebSocket(page);
		const api = await mockWorkspaceApi(page, { conversations: [conversation] });
		await page.goto('/inbox');

		const button = panel(page).getByRole('button', { name: 'Summarize conversation' });
		await expect(button).toBeVisible();
		await expect(panel(page).getByTestId('summary-field-customer_wants')).toHaveCount(0);

		await button.click();
		const loading = panel(page).getByRole('button', { name: 'Summarizing…' });
		await expect(loading).toBeDisabled();
		expect(api.requests.filter((r) => r.method === 'POST' && r.path === '/conversations/conversation-1/summary')).toHaveLength(1);

		// Generation finishes on the server: GET now returns it and the event arrives.
		api.setSummary('conversation-1', summary());
		await ws.push({ type: 'conversation.summary_updated', conversation_id: 'conversation-1', summary_fields: { customer_wants: 'A kitchen remodel' } });

		await expect(panel(page).getByTestId('summary-field-customer_wants')).toContainText('Customer Wants');
		await expect(panel(page).getByTestId('summary-field-customer_wants')).toContainText('A kitchen remodel');
		await expect(panel(page).getByTestId('summary-field-next_action')).toContainText('Next Step');
		await expect(panel(page).getByTestId('summary-field-preferred_timeframe')).toContainText('N/A');
		await expect(panel(page).getByTestId('summary-generated-at')).toContainText('Generated');
		await expect(panel(page).getByTestId('summary-stale')).toHaveCount(0);
		await expect(panel(page).getByRole('button', { name: 'Regenerate summary' })).toBeEnabled();
		await expect(panel(page).getByTestId('summary-error')).toHaveCount(0);
	});

	test('loads an existing summary when the conversation opens and marks it stale on new messages', async ({ page }) => {
		const ws = await captureWebSocket(page);
		await mockWorkspaceApi(page, {
			conversations: [conversation],
			summaries: { 'conversation-1': summary({ fields: [{ key: 'legacy', label: 'legacy', value: 'Old key' }] }) }
		});
		await page.goto('/inbox');

		await expect(panel(page).getByTestId('summary-field-legacy')).toContainText('Old key');
		await expect(panel(page).getByRole('button', { name: 'Regenerate summary' })).toBeVisible();
		await expect(panel(page).getByTestId('summary-stale')).toHaveCount(0);

		await ws.push({
			type: 'message.received',
			conversation_id: 'conversation-1',
			message: { id: 'm-new', direction: 'inbound', sender_type: 'contact', content_type: 'text', content: { text: 'Also a new sink' } }
		});
		await expect(panel(page).getByTestId('summary-stale')).toHaveText('Out of date');
	});

	test('a stale summary from the server is flagged and a pushed update from another user replaces it', async ({ page }) => {
		const ws = await captureWebSocket(page);
		const api = await mockWorkspaceApi(page, {
			conversations: [conversation],
			summaries: { 'conversation-1': summary({ stale: true }) }
		});
		await page.goto('/inbox');
		await expect(panel(page).getByTestId('summary-stale')).toBeVisible();

		api.setSummary('conversation-1', summary({
			fields: [{ key: 'customer_wants', label: 'Customer Wants', value: 'A bathroom instead' }]
		}));
		await ws.push({ type: 'conversation.summary_updated', conversation_id: 'conversation-1', summary_fields: {} });

		await expect(panel(page).getByTestId('summary-field-customer_wants')).toContainText('A bathroom instead');
		await expect(panel(page).getByTestId('summary-stale')).toHaveCount(0);
		await expect(panel(page).getByRole('button', { name: 'Summarizing…' })).toHaveCount(0);
	});

	test('events for other conversations do not touch the open summary', async ({ page }) => {
		const ws = await captureWebSocket(page);
		const api = await mockWorkspaceApi(page, {
			conversations: [conversation],
			summaries: { 'conversation-1': summary() }
		});
		await page.goto('/inbox');
		await expect(panel(page).getByTestId('summary-field-customer_wants')).toBeVisible();
		const before = api.requests.filter((r) => r.path.endsWith('/summary')).length;

		await ws.push({ type: 'conversation.summary_updated', conversation_id: 'some-other-conversation', summary_fields: {} });
		await ws.push({ type: 'conversation.summary_failed', conversation_id: 'some-other-conversation', message: 'nope' });
		// A marker event that is processed after the two above.
		await ws.push({ type: 'message.received', conversation_id: 'conversation-1', message: { id: 'marker', direction: 'inbound', sender_type: 'contact', content_type: 'text', content: { text: 'x' } } });
		await expect(panel(page).getByTestId('summary-stale')).toBeVisible();

		expect(api.requests.filter((r) => r.path.endsWith('/summary')).length).toBe(before);
		await expect(panel(page).getByTestId('summary-error')).toHaveCount(0);
	});

	test('generation failure from the AI service is shown and the button is usable again', async ({ page }) => {
		const ws = await captureWebSocket(page);
		await mockWorkspaceApi(page, { conversations: [conversation] });
		await page.goto('/inbox');

		await panel(page).getByRole('button', { name: 'Summarize conversation' }).click();
		await expect(panel(page).getByRole('button', { name: 'Summarizing…' })).toBeDisabled();
		await ws.push({
			type: 'conversation.summary_failed',
			conversation_id: 'conversation-1',
			error_code: 'ai_not_configured',
			message: 'AI provider is not configured for this workspace.'
		});

		await expect(panel(page).getByTestId('summary-error')).toHaveText('AI provider is not configured for this workspace.');
		await expect(panel(page).getByRole('button', { name: 'Summarize conversation' })).toBeEnabled();
	});

	test('a failed request shows an error and a retry clears it', async ({ page }) => {
		const api = await mockWorkspaceApi(page, {
			conversations: [conversation],
			summaryRequest: { status: 503, body: { error: 'internal error' } }
		});
		await page.goto('/inbox');

		const button = panel(page).getByRole('button', { name: 'Summarize conversation' });
		await button.click();
		await expect(panel(page).getByTestId('summary-error')).toHaveText('Could not request a summary. Please try again.');
		await expect(button).toBeEnabled();

		await button.click();
		await expect(panel(page).getByTestId('summary-error')).toHaveText('Could not request a summary. Please try again.');
		expect(api.requests.filter((r) => r.method === 'POST' && r.path.endsWith('/summary'))).toHaveLength(2);
	});

	test('an up-to-date answer settles immediately with the stored summary', async ({ page }) => {
		await mockWorkspaceApi(page, {
			conversations: [conversation],
			summaryRequest: { status: 200, body: { status: 'up_to_date', summary: summary() } }
		});
		await page.goto('/inbox');

		await panel(page).getByRole('button', { name: 'Summarize conversation' }).click();
		await expect(panel(page).getByTestId('summary-field-customer_wants')).toContainText('A kitchen remodel');
		await expect(panel(page).getByRole('button', { name: 'Regenerate summary' })).toBeEnabled();
		await expect(panel(page).getByTestId('summary-error')).toHaveCount(0);
	});

	test('repeated clicks while generating send one request', async ({ page }) => {
		const api = await mockWorkspaceApi(page, { conversations: [conversation], summaryRequest: { delayMs: 300 } });
		await page.goto('/inbox');

		const button = panel(page).getByRole('button', { name: 'Summarize conversation' });
		await button.dblclick();
		await expect(panel(page).getByRole('button', { name: 'Summarizing…' })).toBeDisabled();
		await page.waitForTimeout(500);
		expect(api.requests.filter((r) => r.method === 'POST' && r.path.endsWith('/summary'))).toHaveLength(1);
	});

	test('an agent can summarize a conversation they can see', async ({ page }) => {
		const api = await mockWorkspaceApi(page, { role: 'agent', conversations: [conversation] });
		await page.goto('/inbox');
		await panel(page).getByRole('button', { name: 'Summarize conversation' }).click();
		await expect(panel(page).getByRole('button', { name: 'Summarizing…' })).toBeVisible();
		expect(api.requests.some((r) => r.method === 'POST' && r.path === '/conversations/conversation-1/summary')).toBe(true);
	});

	test('chatbot-only workspaces never load or offer summaries', async ({ page }) => {
		const api = await mockWorkspaceApi(page, {
			productMode: 'chatbot_only',
			conversations: [conversation],
			summaries: { 'conversation-1': summary() }
		});
		await page.goto('/inbox');
		await expect(page.getByText('Rina Patel', { exact: true }).first()).toBeVisible();

		await expect(page.getByRole('button', { name: 'Summarize conversation' })).toHaveCount(0);
		await expect(page.getByTestId('conversation-summary')).toHaveCount(0);
		expect(api.requests.some((r) => r.path.endsWith('/summary'))).toBe(false);
	});
});
