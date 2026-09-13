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
			timezone: 'UTC'
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

	test('disables Continue until the saved pipeline has loaded', async ({ page }) => {
		await mockOnboardingApi(page);
		await page.route('**/api-gateway/workspace/pipelines', async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 500));
			await route.fallback();
		});

		await page.goto('/onboarding/3');
		const continueButton = page.getByRole('button', { name: 'Continue', exact: true });
		await expect(continueButton).toBeDisabled();
		await expect(continueButton).toBeEnabled();
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
		expect(api.requests).toContainEqual(expect.objectContaining({
			path: '/workspace/account/ai-config',
			method: 'PUT',
			body: expect.objectContaining({
				analysis_model: 'gemma-4-26b-a4b-it',
				reply_model: 'gemini-flash-lite-latest',
				embedding_model: 'gemini-embedding-001'
			})
		}));
	});

	test('reuses a successful unchanged provider check when continuing', async ({ page }) => {
		const api = await mockOnboardingApi(page, [], false);
		await page.goto('/onboarding/5');
		await page.getByLabel(/^API key/).fill('test-provider-key');
		await page.getByRole('button', { name: 'Test connection', exact: true }).click();

		const checkResults = page.getByRole('list', { name: 'AI provider check results' });
		await expect(checkResults.getByText('analysis', { exact: true })).toBeVisible();
		await expect(checkResults.getByText('reply', { exact: true })).toBeVisible();
		await expect(checkResults.getByText('embedding', { exact: true })).toBeVisible();
		await expect(checkResults).toContainText('gemini-flash-lite-latest → gemini-3.5-flash-lite');

		await page.getByRole('button', { name: 'Continue', exact: true }).click();
		await expect(page).toHaveURL(/\/onboarding\/6$/);
		expect(api.requests.filter((request) => request.path === '/workspace/account/ai-config/test')).toHaveLength(1);
	});

	test('checks the provider again after a verified model changes', async ({ page }) => {
		const api = await mockOnboardingApi(page, [], false);
		await page.goto('/onboarding/5');
		await page.getByLabel(/^API key/).fill('test-provider-key');
		await page.getByRole('button', { name: 'Test connection', exact: true }).click();
		await expect(page.getByText('AI provider connection verified successfully', { exact: true })).toBeVisible();

		await page.getByLabel('Customer reply model').fill('gemini-flash-latest');
		await page.getByRole('button', { name: 'Continue', exact: true }).click();
		await expect(page).toHaveURL(/\/onboarding\/6$/);
		expect(api.requests.filter((request) => request.path === '/workspace/account/ai-config/test')).toHaveLength(2);
	});

	test('shows each provider failure instead of only the first one', async ({ page }) => {
		await mockOnboardingApi(page, [], false);
		await page.route('**/api-gateway/workspace/account/ai-config/test', (route) => route.fulfill({
			status: 422,
			contentType: 'application/json',
			body: JSON.stringify({
				ok: false,
				error: 'One or more AI provider checks failed',
				checks: [
					{ role: 'analysis', model: 'gemma-4-26b-a4b-it', ok: false, kind: 'timeout', message: 'Provider did not respond before the connection-test timeout' },
					{ role: 'reply', model: 'gemini-flash-lite-latest', ok: false, kind: 'truncated', message: 'Output was truncated before structured response completed' },
					{ role: 'embedding', model: 'gemini-embedding-001', ok: true, message: 'Embedding model verified' }
				]
			})
		}));
		await page.goto('/onboarding/5');
		await page.getByLabel(/^API key/).fill('test-provider-key');
		await page.getByRole('button', { name: 'Test connection', exact: true }).click();

		await expect(page.getByText('One or more AI provider checks failed', { exact: true })).toBeVisible();
		await expect(page.getByText('Provider did not respond before the connection-test timeout', { exact: true })).toBeVisible();
		await expect(page.getByText('Output was truncated before structured response completed', { exact: true })).toBeVisible();
		await expect(page.getByText('Embedding model verified', { exact: true })).toBeVisible();
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

	test('time zone selector exposes all supported timezones and saves selected zone', async ({ page }) => {
		const api = await mockOnboardingApi(page);
		await page.goto('/onboarding/1');
		await expect(page.getByLabel('Business name')).toHaveValue('Setup Studio');

		const tzSelect = page.getByLabel('Time zone');
		const options = tzSelect.locator('option');
		const count = await options.count();
		expect(count).toBeGreaterThan(100);

		await tzSelect.selectOption('Asia/Tokyo');
		await page.getByRole('button', { name: 'Continue', exact: true }).click();
		await expect(page).toHaveURL(/\/onboarding\/2$/);
		expect(api.getSettings()).toMatchObject({
			timezone: 'Asia/Tokyo'
		});
	});

	test('normalizes legacy timezone values on onboarding step 1', async ({ page }) => {
		await mockOnboardingApi(page);
		await page.route('**/api-gateway/workspace/account', (route) => {
			if (route.request().method() === 'GET') {
				const bytes = new TextEncoder().encode(JSON.stringify({
					timezone: '(GMT+05:30) Asia / Colombo'
				}));
				const settingsB64 = btoa(Array.from(bytes, (b) => String.fromCharCode(b)).join(''));
				return route.fulfill({
					contentType: 'application/json',
					body: JSON.stringify({
						id: 'account-1',
						name: 'Setup Studio',
						product_mode: 'full_workspace',
						settings: settingsB64
					})
				});
			}
			return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ status: 'updated' }) });
		});

		await page.goto('/onboarding/1');
		await expect(page.getByLabel('Business name')).toHaveValue('Setup Studio');
		await expect(page.getByLabel('Time zone')).toHaveValue('Asia/Colombo');
	});

	test('keeps footer pinned at the bottom of the column so scrollable content does not appear below it', async ({ page }) => {
		await mockOnboardingApi(page);
		await page.setViewportSize({ width: 1280, height: 600 });
		await page.goto('/onboarding/5');

		const footer = page.locator('footer');
		await expect(footer).toBeVisible();

		const footerBox = await footer.boundingBox();
		expect(footerBox).not.toBeNull();
		const viewport = page.viewportSize();

		expect(footerBox!.y + footerBox!.height).toBeCloseTo(viewport!.height, 1);

		const scrollContainer = page.locator('.overflow-y-auto');
		const scrollBox = await scrollContainer.boundingBox();
		expect(scrollBox).not.toBeNull();

		expect(scrollBox!.y + scrollBox!.height).toBeLessThanOrEqual(footerBox!.y + 1);
	});
});
