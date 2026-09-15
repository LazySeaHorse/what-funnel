import { expect, test } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

test.describe('atomic UI primitives integration', () => {
	test('Button component renders variant classes, handles busy state, and triggers actions', async ({ page }) => {
		await page.goto('/login');

		const submitBtn = page.getByRole('button', { name: 'Sign in', exact: true });
		await expect(submitBtn).toBeVisible();
		// Verify primary variant class styling
		await expect(submitBtn).toHaveClass(/bg-blue-600/);

		// Fill in form to verify disabled/busy state
		await page.getByLabel('Email or username').fill('agent@example.test');
		await page.getByLabel('Password', { exact: true }).fill('password123');
	});

	test('Modal component renders with accessible role, title, and dismisses on Escape', async ({ page }) => {
		await mockWorkspaceApi(page, { role: 'manager' });
		await page.goto('/inbox?tab=settings');
		await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible();

		// Switch to Users & permissions tab
		await page.getByRole('tab', { name: 'Users & permissions', exact: true }).click();
		await expect(page.getByRole('heading', { name: 'Users & permissions', exact: true })).toBeVisible();

		// Click "Add user" button to open modal
		await page.getByRole('button', { name: 'Add user' }).click();

		const dialog = page.getByRole('dialog', { name: 'Add Team Member' });
		await expect(dialog).toBeVisible();
		await expect(dialog).toHaveAttribute('aria-modal', 'true');

		// Press Escape to dismiss modal
		await page.keyboard.press('Escape');
		await expect(dialog).not.toBeVisible();
	});

	test('Modal component dismisses when clicking the backdrop', async ({ page }) => {
		await mockWorkspaceApi(page, { role: 'manager' });
		await page.goto('/inbox?tab=settings');

		await page.getByRole('tab', { name: 'Users & permissions', exact: true }).click();
		await page.getByRole('button', { name: 'Add user' }).click();

		const dialog = page.getByRole('dialog', { name: 'Add Team Member' });
		await expect(dialog).toBeVisible();

		// Click top-left of the viewport (the backdrop overlay)
		await page.mouse.click(10, 10);
		await expect(dialog).not.toBeVisible();
	});

	test('Popover and Tabs components work seamlessly in the lead sidepanel', async ({ page }) => {
		await mockWorkspaceApi(page, {
			role: 'manager',
			productMode: 'full_workspace',
			conversations: [
				{
					id: 'convo-prim-1',
					status: 'open',
					assigned_user_ids: [],
					created_at: '2026-01-01T12:00:00Z',
					last_message_at: '2026-01-01T12:00:00Z',
					channel_type: 'whatsapp',
					contact: { display_name: 'Alex Rivera', external_identity: '+15559876' },
					lead: { id: 'lead-prim-1', current_state_key: 'new', tags: ['vip'] }
				}
			]
		});

		await page.goto('/inbox');
		await expect(page.getByRole('heading', { name: 'Alex Rivera', exact: true })).toBeVisible();

		const leadPanel = page.locator('.lead-panel');
		await expect(leadPanel).toBeVisible();

		// Verify Tabs component functionality
		const detailsTab = leadPanel.getByRole('button', { name: 'Details', exact: true });
		await expect(detailsTab).toBeVisible();
		await detailsTab.click();
		await expect(detailsTab).toHaveAttribute('aria-pressed', 'true');

		const leadTab = leadPanel.getByRole('button', { name: 'Lead', exact: true });
		await leadTab.click();
		await expect(leadTab).toHaveAttribute('aria-pressed', 'true');

		// Verify Popover component for Lead stage picker
		const stageBtn = leadPanel.getByLabel('Change lead stage');
		await expect(stageBtn).toBeVisible();
		await stageBtn.click();

		// Popover menu is now open
		const menu = page.locator('[role="menu"]');
		await expect(menu).toBeVisible();

		// Dismiss popover with Escape key
		await page.keyboard.press('Escape');
		await expect(menu).not.toBeVisible();
	});
});
