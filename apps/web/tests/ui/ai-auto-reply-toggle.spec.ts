import { expect, test } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

test('manager global auto-reply control reports and persists the workspace mode', async ({ page }) => {
	const api = await mockWorkspaceApi(page, {
		role: 'manager',
		aiConfigured: true,
		autoReplyEnabled: true
	});

	await page.goto('/inbox');
	const toggle = page.getByRole('switch', { name: 'Global AI auto-reply' });
	await expect(toggle).toHaveAttribute('aria-checked', 'true');
	await expect(toggle).toContainText('ON');

	await toggle.click();
	await expect(toggle).toHaveAttribute('aria-checked', 'false');
	await expect(toggle).toContainText('OFF');
	expect(api.requests).toContainEqual(expect.objectContaining({
		path: '/workspace/account/settings',
		method: 'PATCH',
		body: expect.objectContaining({ ai_enabled: false })
	}));
});

test('chat auto-reply can opt out and return to the global default', async ({ page }) => {
	const conversation = {
		id: 'conversation-1',
		status: 'open',
		assigned_user_ids: [],
		ai_mode_active: true,
		ai_auto_reply_enabled: null,
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
	const toggle = page.getByRole('switch', { name: 'Auto-reply for this chat' });
	await expect(toggle).toHaveText(/AI on/);

	await toggle.click();
	await expect(toggle).toHaveText(/AI off/);
	await toggle.click();
	await expect(toggle).toHaveText(/AI on/);

	const updates = api.requests.filter((request) => request.path === '/conversations/conversation-1/ai-auto-reply');
	expect(updates.map((request) => request.body)).toEqual([{ enabled: false }, { enabled: null }]);
});
