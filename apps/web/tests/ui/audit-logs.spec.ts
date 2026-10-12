import { expect, test } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

const entry = (n: number, extra: Record<string, unknown> = {}) => ({
	id: `log-${n}`,
	actor_user_id: 'user-1',
	actor_name: 'Maya Manager',
	actor_email: 'manager@example.test',
	action: 'lead.state_changed',
	target_type: 'lead',
	target_id: `lead-${n}`,
	metadata: { from: 'new', to: 'won' },
	created_at: `2026-03-${String(30 - n).padStart(2, '0')}T10:00:00Z`,
	...extra
});

const logs = [
	entry(1),
	entry(2, { actor_user_id: null, actor_name: null, actor_email: null, action: 'knowledge.purged', target_type: 'knowledge', target_id: null, metadata: {} }),
	entry(3, { actor_user_id: 'gone-user', actor_name: null, actor_email: null, action: 'user.deleted', target_type: 'user' }),
	...Array.from({ length: 50 }, (_, i) => entry(i + 4, { action: 'conversation.assigned', target_type: 'conversation' }))
];

test.describe('logs tab', () => {
	test('manager sees the audit trail with labels, fallbacks, details and paging', async ({ page }) => {
		const api = await mockWorkspaceApi(page, { role: 'manager', auditLogs: logs });
		await page.goto('/inbox?tab=logs');
		await expect(page.getByRole('heading', { name: 'Logs', exact: true })).toBeVisible();

		const rows = page.getByTestId('log-entry');
		await expect(rows).toHaveCount(50);
		await expect(rows.first()).toContainText('Lead state changed');
		await expect(rows.first()).toContainText('Maya Manager');
		await expect(rows.first()).toContainText('from: new, to: won');
		await expect(rows.nth(1)).toContainText('System');
		await expect(rows.nth(2)).toContainText('Deleted user');

		await rows.first().getByRole('button', { name: 'Show details' }).click();
		await expect(rows.first().locator('pre')).toContainText('"to": "won"');

		await page.getByRole('button', { name: 'Load more' }).click();
		await expect(rows).toHaveCount(53);
		await expect(page.getByRole('button', { name: 'Load more' })).not.toBeVisible();
		expect(api.requests.some((r) => r.path === '/workspace/audit-logs')).toBe(true);
	});

	test('filters query the server and an empty result shows a message', async ({ page }) => {
		await mockWorkspaceApi(page, { role: 'manager', auditLogs: logs });
		await page.goto('/inbox?tab=logs');
		await expect(page.getByTestId('log-entry')).toHaveCount(50);

		const request = page.waitForRequest((r) => r.url().includes('action=knowledge.purged'));
		await page.locator('#logs-action-filter').fill('knowledge.purged');
		await request;
		await expect(page.getByTestId('log-entry')).toHaveCount(1);

		await page.locator('#logs-action-filter').fill('nope.nothing');
		await expect(page.getByTestId('logs-empty')).toContainText('No log entries match');
		await page.getByRole('button', { name: 'Clear filters' }).click();
		await expect(page.getByTestId('log-entry')).toHaveCount(50);

		await page.locator('#logs-actor-filter').selectOption('user-1');
		await expect(page.getByTestId('log-entry')).toHaveCount(50);
		await page.locator('#logs-actor-filter').selectOption('');
		await page.locator('#logs-from-filter').fill('2026-03-28');
		await page.locator('#logs-to-filter').fill('2026-03-28');
		await expect(page.getByTestId('log-entry')).toHaveCount(1);
	});

	test('shows an error with retry when loading fails', async ({ page }) => {
		await mockWorkspaceApi(page, { role: 'manager', auditLogs: logs, failures: ['/workspace/audit-logs'] });
		await page.goto('/inbox?tab=logs');
		await expect(page.getByTestId('logs-error')).toContainText('Service unavailable');
	});

	test('agents do not get the Logs tab', async ({ page }) => {
		const api = await mockWorkspaceApi(page, { role: 'agent', auditLogs: logs });
		await page.goto('/inbox?tab=logs');
		await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
		await expect(page.getByRole('button', { name: 'Logs', exact: true })).not.toBeVisible();
		expect(api.requests.some((r) => r.path === '/workspace/audit-logs')).toBe(false);
	});
});
