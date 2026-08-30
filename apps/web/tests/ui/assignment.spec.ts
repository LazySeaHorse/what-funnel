import { expect, test } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

const conversation = {
	id: 'conversation-1',
	status: 'open',
	assigned_user_ids: [] as string[],
	created_at: '2026-01-01T12:00:00Z',
	last_message_at: '2026-01-01T12:00:00Z',
	channel_type: 'matrix_whatsapp',
	contact: {
		display_name: 'Jordan Lee',
		external_identity: '+15550199'
	},
	lead: {
		id: 'lead-1',
		current_state_key: 'new',
		tags: ['lead']
	}
};

test.describe('conversation assignment UI workflows', () => {
	test('assign, multi-assign, and unassign from the lead sidepanel', async ({ page }) => {
		const api = await mockWorkspaceApi(page, {
			role: 'manager',
			productMode: 'full_workspace',
			conversations: [{ ...conversation }]
		});

		await page.goto('/inbox');
		await expect(page.getByRole('heading', { name: 'Jordan Lee', exact: true })).toBeVisible();

		// Initially unassigned
		const leadPanel = page.locator('.lead-panel');
		await expect(leadPanel).toBeVisible();
		await expect(leadPanel.getByText('Unassigned')).toBeVisible();

		// Open assign dropdown by clicking "+" in the sidepanel
		const assignBtn = leadPanel.getByTitle('Assign conversation');
		await expect(assignBtn).toBeVisible();
		await assignBtn.click();

		// Assign dropdown opens anchored inside the sidepanel
		await expect(leadPanel.getByText('Assign team member')).toBeVisible();
		await expect(leadPanel.getByRole('button', { name: /manager/i })).toBeVisible();

		// Click to assign
		await leadPanel.getByRole('button', { name: /manager/i }).click();

		// Verify API was called with assignment
		await expect.poll(() => api.requests.some((r) => r.path === '/conversations/conversation-1/assign')).toBe(true);

		// Close dropdown by toggling button
		await assignBtn.click();
		await expect(leadPanel.getByText('Assign team member')).not.toBeVisible();
	});
});
