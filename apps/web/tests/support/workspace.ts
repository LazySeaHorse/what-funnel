import { expect, type Page } from '@playwright/test';

export const PASSWORD = 'E2ePassword99!';

export function uniqueEmail(prefix: string) {
	return `${prefix}-${Date.now()}-${Math.floor(Math.random() * 10_000)}@e2e.local`;
}

export async function signupAndOpenSettings(page: Page, workspaceName = 'UI Safety Test Workspace') {
	const email = uniqueEmail('ui');

	await page.goto('/signup');
	await page.getByLabel('Business Name').fill(workspaceName);
	await page.fill('#signup-email-input', email);
	await page.fill('#signup-password-input', 'Password123!');
	await expect(page.getByRole('radio', { name: /Full workspace/i })).toBeChecked();
	await page.click('button[type="submit"]');
	await page.waitForURL('**/onboarding/**', { timeout: 20_000 });

	await page.goto('/inbox?tab=settings');
	await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible({ timeout: 20_000 });

	return { email, workspaceName };
}
