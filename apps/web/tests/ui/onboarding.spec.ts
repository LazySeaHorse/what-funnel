import { expect, test } from '@playwright/test';
import { mockOnboardingApi } from '../support/mock-api';

test.describe('onboarding persistence', () => {
	test('saves business, pipeline, team slug, and AI choices through their real API contracts', async ({ page }) => {
		const api = await mockOnboardingApi(page);
		await page.goto('/onboarding/1');
		await expect(page.getByLabel('Business name')).toHaveValue('Setup Studio');

		await page.getByLabel('Business name').fill('Honest Studio');
		await page.getByLabel('Business type').selectOption('Consulting / Agency');
		await page.getByLabel('Time zone').selectOption('(GMT+00:00) UTC');
		await page.getByRole('button', { name: 'Continue', exact: true }).click();
		await expect(page).toHaveURL(/\/onboarding\/2$/);
		expect(api.getSettings()).toMatchObject({
			business_type: 'Consulting / Agency',
			timezone: '(GMT+00:00) UTC'
		});

		await page.getByRole('button', { name: 'Continue', exact: true }).click();
		await expect(page).toHaveURL(/\/onboarding\/3$/);
		await page.getByPlaceholder('Stage name').fill('Qualified');
		await page.getByRole('button', { name: 'Continue', exact: true }).click();
		await expect(page).toHaveURL(/\/onboarding\/4$/);
		expect(api.getPipeline().states[0].label).toBe('Qualified');

		// Step 4: Team members & slug
		await page.getByPlaceholder('company-name').fill('honest-studio');
		await page.getByRole('button', { name: 'Continue', exact: true }).click();
		await expect(page).toHaveURL(/\/onboarding\/5$/);

		// Step 5: AI Assistant
		await page.getByRole('button', { name: /Suggest replies only/ }).click();
		await page.getByRole('button', { name: 'Continue', exact: true }).click();
		await expect(page).toHaveURL(/\/onboarding\/6$/);
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

	test('requires a real provider configuration before enabling AI', async ({ page }) => {
		const api = await mockOnboardingApi(page, [], false);
		await page.goto('/onboarding/5');
		await expect(page.getByLabel(/^API key/)).toBeVisible();
		await expect(page.getByRole('button', { name: 'Continue', exact: true })).toBeDisabled();
		await expect(page.getByText('Add your AI provider API key, or choose Manual only.', { exact: true })).toBeVisible();
		await page.getByLabel(/^API key/).fill('test-provider-key');
		await expect(page.getByRole('button', { name: 'Continue', exact: true })).toBeEnabled();
		await page.getByRole('button', { name: 'Continue', exact: true }).click();

		await expect(page).toHaveURL(/\/onboarding\/6$/);
		expect(api.isAIConfigured()).toBe(true);
		expect(api.requests).toContainEqual(expect.objectContaining({ path: '/workspace/account/ai-config', method: 'PUT' }));
	});
});
