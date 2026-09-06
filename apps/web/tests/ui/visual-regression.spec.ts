import { expect, test } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

test.describe('visual regression', () => {
	test('login page remains visually stable', async ({ page }) => {
		await page.setViewportSize({ width: 1440, height: 900 });
		await page.goto('/login');
		await expect(page.getByRole('heading', { name: 'Sign in', exact: true })).toBeVisible();
		await expect(page).toHaveScreenshot('login-desktop.png', { animations: 'disabled', fullPage: true });
	});

	test('settings desktop layout remains visually stable', async ({ page }) => {
		await page.setViewportSize({ width: 1440, height: 900 });
		await mockWorkspaceApi(page);
		await page.goto('/inbox?tab=settings');
		await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible();
		await expect(page).toHaveScreenshot('settings-desktop.png', { animations: 'disabled', fullPage: true });
	});

	test('settings mobile layout remains visually stable', async ({ page }) => {
		await page.setViewportSize({ width: 375, height: 812 });
		await mockWorkspaceApi(page);
		await page.goto('/inbox?tab=settings');
		await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible();
		await expect(page).toHaveScreenshot('settings-mobile.png', { animations: 'disabled', fullPage: true });
	});
});
