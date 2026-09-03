import { expect, test } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

test('manager global auto-reply control reports and persists the workspace mode', async ({ page }) => {
	const api = await mockWorkspaceApi(page, {
		role: 'manager',
		aiConfigured: true,
		autoReplyEnabled: true
	});

	await page.goto('/inbox?tab=knowledge');
	const toggle = page.getByRole('switch', { name: 'Global AI auto-reply default' });
	await expect(toggle).toHaveAttribute('aria-checked', 'true');
	await expect(toggle).toContainText('ON');

	await toggle.click();
	await expect(toggle).toHaveAttribute('aria-checked', 'false');
	await expect(toggle).toContainText('OFF');
	expect(api.requests).toContainEqual(expect.objectContaining({
		path: '/workspace/account/settings',
		method: 'PATCH',
		body: expect.objectContaining({ ai_reply_mode_default: 'draft_only' })
	}));
});

test('chat auto-reply can opt out and return to the global default', async ({ page }) => {
	const conversation = {
		id: 'conversation-1',
		status: 'open',
		assigned_user_ids: [],
		ai_control: { state: 'active', state_reason: null, reply_override: 'inherit', run_state: 'idle' },
		contact_name: 'Test Customer',
		channel_type: 'matrix_whatsapp',
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
			channel_type: 'matrix_whatsapp',
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
