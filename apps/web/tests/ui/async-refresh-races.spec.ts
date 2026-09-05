import { expect, test, type Route } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

function conversation(id: string, name: string, externalIdentity: string) {
	return {
		id,
		status: 'open',
		assigned_user_ids: ['user-1'],
		channel_type: 'matrix_whatsapp',
		last_message_at: '2026-01-01T12:00:00Z',
		contact: { display_name: name, external_identity: externalIdentity },
		lead: { id: `lead-${id}`, current_state_key: 'new', tags: [] },
		ai_control: { state: 'active', reply_override: 'inherit', run_state: 'idle' }
	};
}

test('an old conversation filter response cannot replace the current filter', async ({ page }) => {
	const allConversation = conversation('all-only', 'All Only', 'all-only');
	const mineConversation = conversation('mine-only', 'Mine Only', 'mine-only');
	let heldAllRequest: Route | null = null;

	await mockWorkspaceApi(page);
	await page.route('**/api-gateway/conversations?*', async (route) => {
		const filter = new URL(route.request().url()).searchParams.get('filter');
		if (filter === 'all' && !heldAllRequest) {
			heldAllRequest = route;
			return;
		}
		await route.fulfill({
			contentType: 'application/json',
			body: JSON.stringify(filter === 'mine' ? [mineConversation] : [allConversation])
		});
	});

	await page.goto('/inbox');
	await expect.poll(() => Boolean(heldAllRequest)).toBe(true);
	await page.getByRole('button', { name: /^Mine/ }).click();

	// Give the newer request a chance to finish first in an implementation that
	// incorrectly allows overlapping refreshes, then release the stale response.
	await page.waitForTimeout(100);
	await heldAllRequest!.fulfill({
		contentType: 'application/json',
		body: JSON.stringify([allConversation])
	});

	await expect(page.getByText('Mine Only', { exact: true }).first()).toBeVisible();
	await expect(page.getByText('All Only', { exact: true })).toHaveCount(0);
});

test('an old simulator refresh cannot replace the selected contact snapshot', async ({ page }) => {
	const alice = conversation('alice-conversation', 'Alice Test', 'test-alice-001');
	const bob = conversation('bob-conversation', 'Bob Demo', 'test-bob-002');
	let holdNextConversationRequest = false;
	let heldAliceRequest: Route | null = null;

	await mockWorkspaceApi(page, { conversations: [alice, bob] });
	await page.route('**/api-gateway/conversations?*', async (route) => {
		if (holdNextConversationRequest && !heldAliceRequest) {
			heldAliceRequest = route;
			return;
		}
		await route.fulfill({ contentType: 'application/json', body: JSON.stringify([alice, bob]) });
	});
	await page.route('**/api-gateway/conversations/*/messages?*', async (route) => {
		const isAlice = route.request().url().includes('/alice-conversation/');
		await route.fulfill({
			contentType: 'application/json',
			body: JSON.stringify({
				messages: [{
					id: isAlice ? 'alice-message' : 'bob-message',
					sender_type: 'contact',
					direction: 'inbound',
					content: { text: isAlice ? 'Alice stale message' : 'Bob current message' }
				}]
			})
		});
	});
	await page.route('**/api-gateway/conversations/*/reply-draft', async (route) => {
		const isAlice = route.request().url().includes('/alice-conversation/');
		await route.fulfill({
			contentType: 'application/json',
			body: JSON.stringify({
				draft: {
					stage_matched: isAlice ? 'pattern' : 'llm_grounded',
					confidence: 0.9,
					draft_text: isAlice ? 'Alice stale draft' : 'Bob current draft',
					status: 'pending'
				}
			})
		});
	});

	await page.goto('/inbox');
	await expect(page.getByRole('button', { name: 'Simulate DEV' })).toBeVisible();
	holdNextConversationRequest = true;
	await page.getByRole('button', { name: 'Simulate DEV' }).click();
	await expect.poll(() => Boolean(heldAliceRequest)).toBe(true);

	await page.getByRole('button', { name: /Bob Demo/ }).click();
	await expect(page.getByText('Bob current message', { exact: true })).toBeVisible();
	await expect(page.getByText('Bob current draft', { exact: true })).toBeVisible();
	await heldAliceRequest!.fulfill({ contentType: 'application/json', body: JSON.stringify([alice, bob]) });

	await expect(page.getByText('Bob current message', { exact: true })).toBeVisible();
	await expect(page.getByText('Bob current draft', { exact: true })).toBeVisible();
	await expect(page.getByText('Alice stale message', { exact: true })).toHaveCount(0);
	await expect(page.getByText('Alice stale draft', { exact: true })).toHaveCount(0);
});
