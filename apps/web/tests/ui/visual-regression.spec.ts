import { expect, test } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

test.describe('visual regression', () => {
	test('login page remains visually stable', async ({ page }) => {
		await page.setViewportSize({ width: 1440, height: 900 });
		await page.goto('/login');
		await expect(page.getByRole('heading', { name: 'Sign in', exact: true })).toBeVisible();
		await expect(page).toHaveScreenshot('login-desktop.png');
	});

	test('settings desktop layout remains visually stable', async ({ page }) => {
		await page.setViewportSize({ width: 1440, height: 900 });
		await mockWorkspaceApi(page);
		await page.goto('/inbox?tab=settings');
		await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible({ timeout: 15000 });
		await expect(page).toHaveScreenshot('settings-desktop.png', { fullPage: true });
	});

	test('settings mobile layout remains visually stable', async ({ page }) => {
		await page.setViewportSize({ width: 375, height: 812 });
		await mockWorkspaceApi(page);
		await page.goto('/inbox?tab=settings');
		await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible({ timeout: 15000 });
		await expect(page).toHaveScreenshot('settings-mobile.png', { fullPage: true });
	});

	test('inbox desktop layout remains visually stable', async ({ browser }) => {
		// Fixed timezone/locale so the rendered message times are deterministic.
		const context = await browser.newContext({
			viewport: { width: 1440, height: 900 },
			timezoneId: 'UTC',
			locale: 'en-US'
		});
		const page = await context.newPage();
		try {
			await mockWorkspaceApi(page, {
				role: 'manager',
				productMode: 'full_workspace',
				conversations: [
					{
						id: 'convo-visual-1',
						status: 'open',
						assigned_user_ids: ['user-1'],
						channel_type: 'whatsapp',
						last_message_at: '2026-01-01T12:00:00Z',
						contact: { display_name: 'Alice Cooper', external_identity: '+15550101' },
						lead: { id: 'lead-visual-1', current_state_key: 'new', tags: [] },
						ai_control: { state: 'active', reply_override: 'inherit', run_state: 'idle' }
					},
					{
						id: 'convo-visual-2',
						status: 'open',
						assigned_user_ids: [],
						channel_type: 'telegram',
						last_message_at: '2026-01-01T09:30:00Z',
						contact: { display_name: 'Bob Marley', external_identity: '@bob_tele' },
						lead: { id: 'lead-visual-2', current_state_key: 'qualified', tags: [] },
						ai_control: { state: 'active', reply_override: 'inherit', run_state: 'idle' }
					}
				],
				aiConfigured: true,
				autoReplyEnabled: true
			});
			await page.goto('/inbox');
			await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible({ timeout: 15000 });
			await expect(page.getByText('Alice Cooper').first()).toBeVisible();
			await expect(page).toHaveScreenshot('inbox-desktop.png');
		} finally {
			await context.close();
		}
	});
});
