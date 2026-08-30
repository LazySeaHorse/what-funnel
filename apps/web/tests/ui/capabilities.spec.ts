import { expect, test } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

const conversation = {
	id: 'conversation-1',
	status: 'open',
	assigned_user_ids: ['user-1'],
	created_at: '2026-01-01T12:00:00Z',
	last_message_at: '2026-01-01T12:00:00Z',
	channel_type: 'matrix_whatsapp',
	contact: {
		display_name: 'Rina Patel',
		external_identity: '+15550122'
	},
	lead: {
		id: 'lead-1',
		current_state_key: 'new',
		tags: ['priority']
	}
};

const replyDraft = {
	id: 'draft-1',
	conversation_id: conversation.id,
	source_message_id: 'message-1',
	draft_text: 'Here is the prepared response.',
	stage_matched: 'pattern',
	status: 'pending',
	created_at: '2026-01-01T12:00:00Z',
	updated_at: '2026-01-01T12:00:00Z'
};

test.describe('effective UI capabilities', () => {
	test('chatbot-only manager gets a lean inbox without CRM or workforce requests', async ({ page }) => {
		const api = await mockWorkspaceApi(page, {
			role: 'manager',
			productMode: 'chatbot_only',
			conversations: [conversation],
			replyDraft
		});

		await page.goto('/inbox?tab=leads');
		await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
		await expect(page.getByText('Rina Patel', { exact: true }).first()).toBeVisible();

		await expect(page.getByRole('button', { name: 'Leads', exact: true })).not.toBeVisible();
		await expect(page.getByRole('button', { name: 'Contacts', exact: true })).not.toBeVisible();
		await expect(page.getByRole('button', { name: 'Automations', exact: true })).toBeVisible();
		await expect(page.getByRole('button', { name: 'Knowledge', exact: true })).toBeVisible();
		await expect(page.getByTestId('operator-identity')).not.toBeVisible();
		await expect(page.getByTitle('Assign conversation')).not.toBeVisible();
		await expect(page.locator('.lead-panel')).not.toBeVisible();
		await expect(page.getByRole('button', { name: /Internal note/i })).not.toBeVisible();
		await expect(page.getByText(replyDraft.draft_text, { exact: true })).not.toBeVisible();
		await expect(page.getByPlaceholder('Enter a message...')).toBeVisible();

		await expect.poll(() => api.requests.map((request) => request.path)).not.toContain('/workspace/pipelines');
		expect(api.requests.some((request) => request.path === '/workspace/users')).toBe(false);
		expect(api.requests.some((request) => request.path.endsWith('/reply-draft'))).toBe(false);
		expect(api.requests.some((request) => request.path.startsWith('/leads/'))).toBe(false);
	});

	test('full-workspace agent sees operator tools and personal preferences only', async ({ page }) => {
		const api = await mockWorkspaceApi(page, {
			role: 'agent',
			productMode: 'full_workspace',
			conversations: [conversation],
			replyDraft
		});

		await page.goto('/inbox?tab=knowledge');
		await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
		await expect(page.getByRole('button', { name: 'Leads', exact: true })).toBeVisible();
		await expect(page.getByRole('button', { name: 'Contacts', exact: true })).toBeVisible();
		await expect(page.getByRole('button', { name: 'Automations', exact: true })).not.toBeVisible();
		await expect(page.getByRole('button', { name: 'Knowledge', exact: true })).not.toBeVisible();
		await expect(page.getByRole('button', { name: 'Simulate', exact: true })).not.toBeVisible();
		await expect(page.getByRole('button', { name: 'Preferences', exact: true })).toBeVisible();
		await expect(page.getByTestId('operator-identity')).not.toBeVisible();
		await expect(page.getByTitle('Assign conversation')).not.toBeVisible();
		await expect(page.locator('.lead-panel')).toBeVisible();
		await expect(page.getByRole('button', { name: /Internal note/i })).toBeVisible();
		await expect(page.getByText(replyDraft.draft_text, { exact: true })).toBeVisible();

		await page.getByRole('button', { name: 'Preferences', exact: true }).click();
		await expect(page.getByRole('heading', { name: 'Preferences', exact: true })).toBeVisible();
		await expect(page.getByText('test-slug-agent', { exact: true })).toBeVisible();
		await expect(page.getByRole('heading', { name: 'AI reply mode', exact: true })).toBeVisible();
		await page.getByRole('radio', { name: /Auto-send/ }).check();
		await page.getByRole('button', { name: 'Save preference' }).click();
		await expect(page.getByText('Reply preference saved.', { exact: true })).toBeVisible();

		expect(api.requests.some((request) => request.path === '/channels')).toBe(false);
		expect(api.requests.some((request) => request.path === '/bridge-connections')).toBe(false);
		expect(api.requests.some((request) => request.path === '/workspace/users')).toBe(false);
	});

	test('full-workspace manager retains management controls', async ({ page }) => {
		await mockWorkspaceApi(page, {
			role: 'manager',
			productMode: 'full_workspace',
			conversations: [conversation]
		});

		await page.goto('/inbox');
		await expect(page.getByRole('button', { name: 'Leads', exact: true })).toBeVisible();
		await expect(page.getByRole('button', { name: 'Automations', exact: true })).toBeVisible();
		await expect(page.getByRole('button', { name: 'Knowledge', exact: true })).toBeVisible();
		await expect(page.getByRole('button', { name: 'Contacts', exact: true })).toBeVisible();
		await expect(page.getByRole('button', { name: 'Settings', exact: true })).toBeVisible();
		await expect(page.getByTestId('operator-identity')).toBeVisible();
		await expect(page.getByTitle('Assign conversation')).toBeVisible();
	});
});
