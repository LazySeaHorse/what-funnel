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
		await expect(page.getByText('Enter your AI provider API key, or select Manual only.', { exact: true })).toBeVisible();
		await page.getByLabel(/^API key/).fill('test-provider-key');
		await expect(page.getByRole('button', { name: 'Continue', exact: true })).toBeEnabled();
		await page.getByRole('button', { name: 'Continue', exact: true }).click();

		await expect(page).toHaveURL(/\/onboarding\/6$/);
		expect(api.isAIConfigured()).toBe(true);
		expect(api.requests).toContainEqual(expect.objectContaining({ path: '/workspace/account/ai-config', method: 'PUT' }));
	});

	test('chatbot-only onboarding skips lead and team setup without requesting their APIs', async ({ page }) => {
		const api = await mockOnboardingApi(page, [], true, 'chatbot_only');
		await page.goto('/onboarding/1');
		await expect(page.getByText('Step 1 of 5', { exact: true })).toBeVisible();

		await page.getByRole('button', { name: 'Continue', exact: true }).click();
		await expect(page).toHaveURL(/\/onboarding\/2$/);
		await expect(page.getByText('Step 2 of 5', { exact: true })).toBeVisible();
		await page.getByRole('button', { name: 'Continue', exact: true }).click();
		await expect(page).toHaveURL(/\/onboarding\/5$/);
		await expect(page.getByText('Step 3 of 5', { exact: true })).toBeVisible();

		expect(api.requests.some((request) => request.path === '/workspace/pipelines')).toBe(false);
		expect(api.requests.some((request) => request.path === '/workspace/users')).toBe(false);
		expect(api.requests).toContainEqual(expect.objectContaining({
			path: '/onboarding/status',
			method: 'PATCH',
			body: { step: 'pipeline_setup', action: 'skip' }
		}));
		expect(api.requests).toContainEqual(expect.objectContaining({
			path: '/onboarding/status',
			method: 'PATCH',
			body: { step: 'team_setup', action: 'skip' }
		}));
	});

	test('chatbot-only review omits lead and team summaries', async ({ page }) => {
		await mockOnboardingApi(page, [], true, 'chatbot_only');
		await page.goto('/onboarding/7');
		await expect(page.getByText('Step 5 of 5', { exact: true })).toBeVisible();
		await expect(page.getByText('Lead pipeline', { exact: true })).not.toBeVisible();
		await expect(page.getByText('Team', { exact: true })).not.toBeVisible();
		await expect(page.getByText('AI Assistant', { exact: true }).last()).toBeVisible();
		await expect(page.getByText('Knowledge Base', { exact: true }).last()).toBeVisible();
	});

	test('manual only AI mode auto-skips knowledge base paste step', async ({ page }) => {
		const api = await mockOnboardingApi(page, [], false);
		await page.goto('/onboarding/5');
		await page.getByRole('button', { name: /Manual only/ }).click();
		await page.getByRole('button', { name: 'Continue', exact: true }).click();

		// Directly advances to Step 7 (Review & Finish), skipping Step 6
		await expect(page).toHaveURL(/\/onboarding\/7$/);
		expect(api.getSettings()).toMatchObject({ ai_enabled: false, ai_reply_mode_default: 'draft_only' });
		expect(api.requests).toContainEqual(expect.objectContaining({
			path: '/onboarding/status',
			method: 'PATCH',
			body: { step: 'kb_setup', action: 'skip' }
		}));

		// Back navigation from Step 7 returns to Step 5
		await page.getByRole('button', { name: 'Back', exact: true }).click();
		await expect(page).toHaveURL(/\/onboarding\/5$/);
	});

	test('reviews a rich paste inline and publishes it before completing KB setup', async ({ page }) => {
		const api = await mockOnboardingApi(page);
		await page.goto('/onboarding/6');
		await page.getByPlaceholder(/Paste raw business info/).fill('Consulting, pricing, hours, cancellation rules, and several FAQs.');
		await page.getByRole('button', { name: 'Organize with AI', exact: true }).click();

		await expect(page.getByText('Structured Knowledge', { exact: true })).toBeVisible();
		await expect(page.getByLabel('Concept title')).toHaveCount(4);
		await expect(page.getByLabel('Canonical question')).toHaveCount(2);
		await page.getByLabel('Concept title').first().fill('Advisory consulting');
		await page.getByLabel('Include Cancellation').uncheck();
		await page.getByLabel('Canonical question').first().fill('How much does advisory consulting cost?');
		await page.getByRole('button', { name: 'Add to Knowledge Base', exact: true }).click();

		await expect(page).toHaveURL(/\/onboarding\/7$/);
		const publish = api.requests.find((request) => request.path.endsWith('/publish'));
		expect(publish?.body?.concepts).toEqual(expect.arrayContaining([
			expect.objectContaining({ title: 'Advisory consulting', approved: true }),
			expect.objectContaining({ title: 'Cancellation', approved: false })
		]));
		expect(publish?.body?.patterns).toEqual(expect.arrayContaining([
			expect.objectContaining({ canonical_question: 'How much does advisory consulting cost?', approved: true })
		]));
		expect(api.requests).toContainEqual(expect.objectContaining({
			path: '/onboarding/status',
			method: 'PATCH',
			body: { step: 'kb_setup', action: 'complete' }
		}));
	});

	test('can skip a running KB ingestion to the next onboarding page', async ({ page }) => {
		const api = await mockOnboardingApi(page);
		await page.route(/\/api-gateway\/api\/kb\/ingestions\/[0-9a-f-]+$/, async (route) => {
			return route.fulfill({
				contentType: 'application/json',
				body: JSON.stringify({ id: '11111111-1111-4111-8111-111111111111', status: 'processing', concepts: [], patterns: [] })
			});
		});
		await page.goto('/onboarding/6');
		await page.getByPlaceholder(/Paste raw business info/).fill('Enough information to start compiling.');
		await page.getByRole('button', { name: 'Organize with AI', exact: true }).click();

		await page.getByRole('button', { name: /Skip waiting and go to next page/ }).click();
		await expect(page).toHaveURL(/\/onboarding\/7$/);
		expect(api.requests).not.toContainEqual(expect.objectContaining({
			path: '/onboarding/status',
			body: { step: 'done', action: 'complete' }
		}));
	});
});
