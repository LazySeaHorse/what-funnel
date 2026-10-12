import { expect, test } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

test('manager can switch the workspace AI mode in general settings', async ({ page }) => {
	const api = await mockWorkspaceApi(page, {
		role: 'manager',
		aiConfigured: true,
		autoReplyEnabled: true
	});

	await page.goto('/inbox?tab=settings');
	await expect(page.getByRole('radio', { name: /Full AI/ })).toBeChecked();

	await page.getByRole('radio', { name: /Suggestions only/ }).check();
	await page.getByRole('button', { name: 'Save changes' }).click();
	await expect.poll(() => api.requests.filter((r) => r.path === '/workspace/account/settings' && r.method === 'PATCH').at(-1)?.body)
		.toEqual(expect.objectContaining({ ai_enabled: true, ai_reply_mode_default: 'draft_only' }));

	await page.getByRole('radio', { name: /No AI replies/ }).check();
	await page.getByRole('button', { name: 'Save changes' }).click();
	await expect.poll(() => api.requests.filter((r) => r.path === '/workspace/account/settings' && r.method === 'PATCH').at(-1)?.body)
		.toEqual(expect.objectContaining({ ai_enabled: false }));
});

test('enabling AI replies without a configured provider shows the error and saves nothing', async ({ page }) => {
	const api = await mockWorkspaceApi(page, { role: 'manager', aiConfigured: false });
	await page.goto('/inbox?tab=settings');

	await page.getByRole('radio', { name: /No AI replies/ }).check();
	await page.getByRole('button', { name: 'Save changes' }).click();
	await expect.poll(() => api.requests.filter((r) => r.path === '/workspace/account/settings' && r.method === 'PATCH').length).toBe(1);

	await page.getByRole('radio', { name: /Full AI/ }).check();
	await page.getByRole('button', { name: 'Save changes' }).click();
	await expect(page.getByText(/Configure an AI provider/)).toBeVisible();
	expect(api.requests.filter((r) => r.path === '/workspace/account/settings' && r.method === 'PATCH')).toHaveLength(1);
});

test('the knowledge page no longer carries a global auto-reply switch', async ({ page }) => {
	await mockWorkspaceApi(page, { role: 'manager', aiConfigured: true, autoReplyEnabled: true });
	await page.goto('/inbox?tab=knowledge');
	await expect(page.getByRole('navigation', { name: 'Knowledge sections' })).toBeVisible();
	await expect(page.getByRole('switch', { name: 'Global AI auto-reply default' })).toHaveCount(0);
});

test('chat auto-reply can opt out and return to the global default', async ({ page }) => {
	const conversation = {
		id: 'conversation-1',
		status: 'open',
		assigned_user_ids: [],
		ai_control: { state: 'active', state_reason: null, reply_override: 'inherit', run_state: 'idle' },
		contact_name: 'Test Customer',
		channel_type: 'whatsapp',
		last_message_at: '2026-08-30T00:00:00Z'
	};
	const api = await mockWorkspaceApi(page, {
		role: 'manager',
		aiConfigured: true,
		autoReplyEnabled: true,
		conversations: [conversation]
	});

	await page.goto('/inbox');
	await page.getByText('Test Customer').first().click();
	const toggle = page.getByRole('switch', { name: 'AI replies for this chat' });
	await expect(toggle).toHaveText(/AI replies on/);

	await toggle.click();
	await expect(toggle).toHaveText(/AI replies off/);
	await toggle.click();
	await expect(toggle).toHaveText(/AI replies on/);

	const updates = api.requests.filter((request) => request.path === '/conversations/conversation-1/ai-control');
	expect(updates.map((request) => request.body)).toEqual([
		{ reply_override: 'disabled' },
		{ reply_override: 'enabled' }
	]);
});

test('AI ownership locks the composer and offers an immediate pause', async ({ page }) => {
	const api = await mockWorkspaceApi(page, {
		role: 'manager',
		aiConfigured: true,
		autoReplyEnabled: true,
		conversations: [{
			id: 'conversation-replying',
			status: 'open',
			assigned_user_ids: [],
			ai_control: { state: 'active', state_reason: null, reply_override: 'inherit', run_state: 'replying' },
			contact_name: 'Replying Customer',
			channel_type: 'whatsapp',
			last_message_at: '2026-08-30T00:00:00Z'
		}]
	});

	await page.goto('/inbox');
	await page.getByText('Replying Customer').first().click();
	await expect(page.getByText('AI is replying...')).toBeVisible();
	await expect(page.getByPlaceholder('Enter a message...')).not.toBeVisible();

	await page.getByRole('button', { name: 'Pause AI' }).click();
	await expect(page.getByPlaceholder('Enter a message...')).toBeVisible();
	expect(api.requests).toContainEqual(expect.objectContaining({
		path: '/conversations/conversation-replying/ai-control',
		method: 'PATCH',
		body: { action: 'pause' }
	}));
});

const closableConversation = {
	id: 'conversation-close',
	status: 'open',
	assigned_user_ids: [],
	ai_control: { state: 'active', state_reason: null, reply_override: 'inherit', run_state: 'idle' },
	contact_name: 'Closable Customer',
	channel_type: 'whatsapp',
	last_message_at: '2026-08-30T00:00:00Z'
};

test('the X closes the conversation and returns to the empty state', async ({ page }) => {
	const api = await mockWorkspaceApi(page, { role: 'manager', conversations: [{ ...closableConversation }] });
	await page.goto('/inbox');
	await page.getByText('Closable Customer').first().click();

	await page.getByRole('button', { name: 'Close conversation' }).click();
	await expect(page.getByText('Select a conversation from the list')).toBeVisible();
	expect(api.requests).toContainEqual(expect.objectContaining({
		path: '/conversations/conversation-close/close',
		method: 'POST'
	}));
});

test('a failed close keeps the chat open and shows the error', async ({ page }) => {
	await mockWorkspaceApi(page, {
		role: 'manager',
		conversations: [{ ...closableConversation }],
		failures: ['/conversations/conversation-close/close']
	});
	await page.goto('/inbox');
	await page.getByText('Closable Customer').first().click();

	await page.getByRole('button', { name: 'Close conversation' }).click();
	await expect(page.getByText('Failed to close the conversation. Please try again.')).toBeVisible();
	await expect(page.getByRole('button', { name: 'Close conversation' })).toBeVisible();
});
