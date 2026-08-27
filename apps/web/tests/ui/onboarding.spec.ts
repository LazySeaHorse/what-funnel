import { expect, test } from '@playwright/test';
import { mockOnboardingApi } from '../support/mock-api';

test.describe('onboarding persistence', () => {
	test('saves business, pipeline, and AI choices through their real API contracts', async ({ page }) => {
		const api = await mockOnboardingApi(page);
		await page.goto('/onboarding/1');
		await expect(page.getByLabel('Business name')).toHaveValue('Setup Studio');

		await page.getByLabel('Business name').fill('Honest Studio');
		await page.getByLabel('Business type').selectOption('Consulting / Agency');
		await page.getByLabel('Time zone').selectOption('(GMT+00:00) UTC / London');
		await page.getByRole('button', { name: 'Continue', exact: true }).click();
		await expect(page).toHaveURL(/\/onboarding\/2$/);
		expect(api.getSettings()).toMatchObject({
			business_type: 'Consulting / Agency',
			timezone: '(GMT+00:00) UTC / London'
		});

		await page.getByRole('button', { name: 'Continue', exact: true }).click();
		await expect(page).toHaveURL(/\/onboarding\/3$/);
		await page.getByPlaceholder('Stage name').fill('Qualified');
		await page.getByRole('button', { name: 'Continue', exact: true }).click();
		await expect(page).toHaveURL(/\/onboarding\/4$/);
		expect(api.getPipeline().states[0].label).toBe('Qualified');

		await page.getByRole('button', { name: /Suggest replies only/ }).click();
		await page.getByRole('button', { name: 'Continue', exact: true }).click();
		await expect(page).toHaveURL(/\/onboarding\/5$/);
		expect(api.getSettings()).toMatchObject({ ai_enabled: true, ai_reply_mode_default: 'draft_only' });
	});

	test('does not advance when a required save fails', async ({ page }) => {
		await mockOnboardingApi(page, ['PATCH /workspace/account/settings']);
		await page.goto('/onboarding/1');
		await expect(page.getByLabel('Business name')).toHaveValue('Setup Studio');
		await page.getByRole('button', { name: 'Continue', exact: true }).click();

		await expect(page).toHaveURL(/\/onboarding\/1$/);
		await expect(page.getByText('Setup service is unavailable', { exact: true })).toBeVisible();
	});
});
