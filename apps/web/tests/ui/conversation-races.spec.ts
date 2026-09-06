import { expect, test, type Page, type Route } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

export function conversations() {
	return ['Alice', 'Bob'].map((name, index) => ({
		id: `conversation-${index + 1}`, status: 'open', assigned_user_ids: [] as string[],
		channel_type: 'matrix_whatsapp', last_message_at: '2026-01-01T12:00:00Z',
		contact: { display_name: name, external_identity: name.toLowerCase() },
		lead: { id: `lead-${index + 1}`, current_state_key: 'new', tags: [] as string[] },
		ai_control: { state: 'active', reply_override: 'inherit', run_state: 'idle' }
	}));
}

async function holdRequest(page: Page, pattern: string) {
	let held: Route | undefined;
	let count = 0;
	await page.route(pattern, (route) => { held = route; count++; });
	return {
		get count() { return count; },
		async wait() { await expect.poll(() => Boolean(held)).toBe(true); return held!; },
		async release(body: unknown, status = 200) {
			const route = await this.wait();
			const response = page.waitForResponse((response) => response.request() === route.request());
			await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
			await (await response).finished();
			await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))));
		}
	};
}

test('a delayed send stays in its conversation and preserves the next conversation draft', async ({ page }) => {
	await mockWorkspaceApi(page, { conversations: conversations() });
	const send = await holdRequest(page, '**/api-gateway/internal/conversations/*/send');
	await page.goto('/inbox');
	const composer = page.getByPlaceholder('Enter a message...');
	await composer.fill('For Alice');
	await composer.press('Enter');
	const request = await send.wait();
	expect(request.request().url()).toContain('/conversation-1/send');
	await composer.press('Enter');
	await page.getByText('Bob', { exact: true }).first().click();
	await expect(page.getByRole('heading', { name: 'Bob', exact: true })).toBeVisible();
	await expect(composer).toHaveValue('');
	await composer.fill('Unsent reply for Bob');
	await send.release({ id: 'sent-A', conversation_id: 'conversation-1', content_type: 'text', content: { text: 'For Alice' }, direction: 'outbound', sender_type: 'human' });
	await expect(page.getByText('For Alice', { exact: true })).toHaveCount(0);
	await expect(composer).toHaveValue('Unsent reply for Bob');
	expect(send.count).toBe(1);
	await page.getByText('Alice', { exact: true }).first().click();
	await expect(page.getByRole('heading', { name: 'Alice', exact: true })).toBeVisible();
	await expect(composer).toHaveValue('');
	await page.getByText('Bob', { exact: true }).first().click();
	await expect(composer).toHaveValue('Unsent reply for Bob');
});

test('a failed send preserves its draft and scopes its error to that conversation', async ({ page }) => {
	await mockWorkspaceApi(page, { conversations: conversations() });
	const send = await holdRequest(page, '**/api-gateway/internal/conversations/*/send');
	await page.goto('/inbox');
	const composer = page.getByPlaceholder('Enter a message...');
	await composer.fill('Retry Alice');
	await composer.press('Enter');
	await send.wait();
	await page.getByText('Bob', { exact: true }).first().click();
	await expect(page.getByRole('heading', { name: 'Bob', exact: true })).toBeVisible();
	await send.release({ error: 'Unavailable' }, 503);
	await expect(page.getByRole('alert')).toHaveCount(0);
	await page.getByText('Alice', { exact: true }).first().click();
	await expect(composer).toHaveValue('Retry Alice');
	await expect(page.getByRole('alert')).toContainText('Failed to send message');
});

test('a delayed assignment updates Alice without changing Bob', async ({ page }) => {
	const records = conversations();
	await mockWorkspaceApi(page, { conversations: records });
	const assignment = await holdRequest(page, '**/api-gateway/conversations/*/assign');
	await page.goto('/inbox');
	const panel = page.locator('.lead-panel');
	await panel.getByTitle('Assign conversation').click();
	await panel.getByRole('button', { name: /manager/i }).click();
	const request = await assignment.wait();
	expect(request.request().url()).toContain('/conversation-1/assign');
	expect(request.request().postDataJSON()).toEqual({ user_ids: ['user-1'] });
	await page.getByText('Bob', { exact: true }).first().click();
	await expect(page.getByRole('heading', { name: 'Bob', exact: true })).toBeVisible();
	records[0].assigned_user_ids = ['user-1'];
	await assignment.release({ status: 'assigned' });
	await expect(panel.getByText('Unassigned', { exact: true })).toBeVisible();
	await page.getByText('Alice', { exact: true }).first().click();
	await expect(panel.getByText('Unassigned', { exact: true })).toHaveCount(0);
});

test('a delayed lead state update cannot change the next conversation', async ({ page }) => {
	await mockWorkspaceApi(page, { conversations: conversations() });
	const stateUpdate = await holdRequest(page, '**/api-gateway/leads/*/state');
	await page.goto('/inbox');
	const panel = page.locator('.lead-panel');
	await panel.getByRole('button', { name: 'Change lead stage' }).click();
	await panel.getByRole('button', { name: 'Set lead stage to New lead' }).click();
	const request = await stateUpdate.wait();
	expect(request.request().url()).toContain('/leads/lead-1/state');
	await page.getByText('Bob', { exact: true }).first().click();
	await expect(page.getByRole('heading', { name: 'Bob', exact: true })).toBeVisible();
	await stateUpdate.release({ id: 'lead-1', current_state_key: 'interested' });
	await expect(panel.getByRole('button', { name: 'Change lead stage' })).toContainText('new');
});

test('a delayed tag update cannot change the next conversation', async ({ page }) => {
	await mockWorkspaceApi(page, { conversations: conversations() });
	const tagUpdate = await holdRequest(page, '**/api-gateway/leads/*/tags');
	await page.goto('/inbox');
	const panel = page.locator('.lead-panel');
	await panel.getByTitle('Add tag').click();
	await panel.getByLabel('Tag name').fill('priority');
	await panel.getByRole('button', { name: 'Save tag' }).click();
	const request = await tagUpdate.wait();
	expect(request.request().url()).toContain('/leads/lead-1/tags');
	await page.getByText('Bob', { exact: true }).first().click();
	await expect(page.getByRole('heading', { name: 'Bob', exact: true })).toBeVisible();
	await tagUpdate.release({ id: 'lead-1', tags: ['priority'] });
	await expect(panel.getByText('No tags', { exact: true })).toBeVisible();
	await expect(panel.getByText('priority', { exact: true })).toHaveCount(0);
});

test('back to conversations cancels a pending conversation selection', async ({ page }) => {
	await mockWorkspaceApi(page, { conversations: conversations() });
	const selection = await holdRequest(page, '**/api-gateway/conversations/conversation-2');
	await page.goto('/inbox');
	await expect(page.getByRole('heading', { name: 'Alice', exact: true })).toBeVisible();
	await page.getByText('Bob', { exact: true }).first().click();
	const request = await selection.wait();

	await page.setViewportSize({ width: 375, height: 812 });
	await page.getByRole('button', { name: 'Back to conversations' }).click();
	await request.fulfill({
		status: 200,
		contentType: 'application/json',
		body: JSON.stringify(conversations()[1])
	}).catch(() => {});
	await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))));

	await expect(page.getByPlaceholder('Search conversations', { exact: true })).toBeVisible();
	await expect(page.getByRole('heading', { name: 'Bob', exact: true })).toHaveCount(0);
});

test('a delayed AI control update does not change or lock the next chat', async ({ page }) => {
	const records = conversations();
	await mockWorkspaceApi(page, { conversations: records, aiConfigured: true, autoReplyEnabled: false });
	const update = await holdRequest(page, '**/api-gateway/conversations/*/ai-control');
	await page.goto('/inbox');
	const toggle = page.getByRole('switch', { name: 'AI replies for this chat' });
	await expect(toggle).toHaveAttribute('aria-checked', 'false');
	await toggle.click();
	const request = await update.wait();
	expect(request.request().url()).toContain('/conversation-1/ai-control');
	await page.getByText('Bob', { exact: true }).first().click();
	await expect(page.getByRole('heading', { name: 'Bob', exact: true })).toBeVisible();
	await expect(toggle).toBeEnabled();
	records[0].ai_control.reply_override = 'enabled';
	await update.release({ ai_control: records[0].ai_control });
	await expect(toggle).toHaveAttribute('aria-checked', 'false');
	await expect(page.getByPlaceholder('Enter a message...')).toBeVisible();
	await page.getByText('Alice', { exact: true }).first().click();
	await expect(toggle).toHaveAttribute('aria-checked', 'true');
});
