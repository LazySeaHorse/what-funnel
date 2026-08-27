import { expect, test } from '@playwright/test';
import { mockWorkspaceApi } from './support/mock-api';

test('loads an AI reply draft and only sends it after agent review', async ({ page }) => {
	await mockWorkspaceApi(page);

	const conversation = {
		id: 'conversation-1',
		status: 'open',
		assigned_user_ids: ['user-1'],
		created_at: '2026-08-27T12:00:00Z',
		last_message_at: '2026-08-27T12:01:00Z',
		channel_type: 'matrix_whatsapp',
		contact: { display_name: 'Alice', external_identity: 'alice@example.test' },
		last_message: {
			id: 'message-1',
			direction: 'inbound',
			sender_type: 'contact',
			content_type: 'text',
			content: { text: 'Are you open today?' },
			created_at: '2026-08-27T12:01:00Z'
		}
	};
	const draft = {
		id: 'draft-1',
		conversation_id: conversation.id,
		source_message_id: 'message-1',
		draft_text: 'Yes, we are open until 8 PM today.',
		stage_matched: 'pattern',
		confidence: 1,
		status: 'pending',
		created_at: '2026-08-27T12:01:01Z',
		updated_at: '2026-08-27T12:01:01Z'
	};
	const sends: Record<string, unknown>[] = [];

	await page.route('**/api-gateway/conversations**', async (route) => {
		const request = route.request();
		const path = new URL(request.url()).pathname.replace('/api-gateway', '');
		const json = (value: unknown) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(value) });

		if (path === '/conversations') return json([conversation]);
		if (path === `/conversations/${conversation.id}`) return json(conversation);
		if (path === `/conversations/${conversation.id}/messages`) {
			return json({ messages: [conversation.last_message], next_cursor: null });
		}
		if (path === `/conversations/${conversation.id}/reply-draft`) return json({ draft });
		if (path === `/conversations/${conversation.id}/read`) return json({ status: 'read' });
		return json({});
	});
	await page.route('**/api-gateway/internal/conversations/**/send', async (route) => {
		sends.push(route.request().postDataJSON());
		return route.fulfill({
			contentType: 'application/json',
			body: JSON.stringify({
				id: 'message-2',
				conversation_id: conversation.id,
				direction: 'outbound',
				sender_type: 'human',
				content_type: 'text',
				content: { text: 'Yes — we are open until 8 PM.' },
				created_at: '2026-08-27T12:02:00Z'
			})
		});
	});

	await page.goto('/inbox');
	await expect(page.getByText('Yes, we are open until 8 PM today.')).toBeVisible();

	await page.getByRole('button', { name: 'Use this' }).click();
	const composer = page.locator('.compose-input');
	await expect(composer).toHaveValue(draft.draft_text);
	await expect(page.getByText(draft.draft_text)).toBeHidden();
	expect(sends).toHaveLength(0);

	await composer.fill('Yes — we are open until 8 PM.');
	await composer.press('Enter');
	await expect.poll(() => sends.length).toBe(1);
	expect(sends[0]).toMatchObject({
		text: 'Yes — we are open until 8 PM.',
		ai_reply_draft_id: draft.id,
		sender_type: 'human'
	});
});
