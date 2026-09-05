import { expect, test, type Route } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

const records = ['Alice', 'Bob'].map((name, index) => ({
	id: `conversation-${index + 1}`,
	status: 'open',
	assigned_user_ids: [] as string[],
	channel_type: 'matrix_whatsapp',
	last_message_at: '2026-01-01T12:00:00Z',
	contact: { display_name: name, external_identity: name.toLowerCase() },
	lead: { id: `lead-${index + 1}`, current_state_key: 'new', tags: [] as string[] }
}));

test('drawer mutations target the displayed lead while its conversation is still loading', async ({ page }) => {
	await mockWorkspaceApi(page, { conversations: records });

	let heldSelection: Route | undefined;
	await page.route('**/api-gateway/conversations/conversation-2', (route) => {
		heldSelection = route;
	});

	const mutations: Array<{ path: string; body: unknown }> = [];
	await page.route('**/api-gateway/leads/**', async (route) => {
		const request = route.request();
		const path = new URL(request.url()).pathname.replace('/api-gateway', '');
		if (request.method() === 'GET') {
			return route.fulfill({ contentType: 'application/json', body: '[]' });
		}
		mutations.push({ path, body: request.postDataJSON() });
		if (path.endsWith('/tags')) {
			return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: request.postDataJSON().tags }) });
		}
		return route.fulfill({ contentType: 'application/json', body: '{}' });
	});

	await page.goto('/inbox');
	await page.getByRole('button', { name: 'Leads', exact: true }).click();
	await page.getByText('Bob', { exact: true }).first().click();
	await expect.poll(() => Boolean(heldSelection)).toBe(true);

	const drawer = page.locator('aside.lead-panel');
	await expect(drawer.getByRole('heading', { name: 'Bob', exact: true })).toBeVisible();
	await drawer.getByTitle('Add tag').click();
	await drawer.getByRole('textbox', { name: 'Tag name' }).fill('priority');
	await drawer.getByRole('button', { name: 'Save tag' }).click();
	await drawer.getByRole('button', { name: 'Notes', exact: true }).click();
	await drawer.getByRole('button', { name: '+ Add note', exact: true }).click();
	await drawer.getByPlaceholder('Add an internal note...').fill('Bob note');
	await drawer.getByRole('button', { name: 'Save', exact: true }).click();

	await expect.poll(() => mutations.length).toBe(2);
	expect(mutations).toEqual([
		{ path: '/leads/lead-2/tags', body: { tags: ['priority'] } },
		{ path: '/leads/lead-2/notes', body: { body: 'Bob note' } }
	]);

	await heldSelection!.fulfill({ contentType: 'application/json', body: JSON.stringify(records[1]) });
});

test('late lead detail responses cannot replace the selected lead details', async ({ page }) => {
	await mockWorkspaceApi(page, { conversations: records });
	let aliceNotes: Route | undefined;
	await page.route('**/api-gateway/leads/lead-1/notes', (route) => { aliceNotes = route; });
	await page.route('**/api-gateway/leads/lead-2/notes', (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify([{ body: 'Bob note' }]) }));
	await page.route('**/api-gateway/leads/*/history', (route) => route.fulfill({ contentType: 'application/json', body: '[]' }));

	await page.goto('/inbox');
	await expect.poll(() => Boolean(aliceNotes)).toBe(true);
	await page.getByRole('button', { name: 'Leads', exact: true }).click();
	await page.getByText('Bob', { exact: true }).first().click();
	const drawer = page.locator('aside.lead-panel');
	await expect(drawer.getByText('Bob note', { exact: true })).toBeVisible();
	await aliceNotes!.fulfill({ contentType: 'application/json', body: JSON.stringify([{ body: 'Alice note' }]) });
	await expect(drawer.getByText('Bob note', { exact: true })).toBeVisible();
	await expect(drawer.getByText('Alice note', { exact: true })).toHaveCount(0);
});
