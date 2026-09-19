import { test, expect } from '@playwright/test';
import { mockOnboardingApi } from '../support/mock-api';
import { DeterministicMonkeyFuzzer } from './monkey';

test.describe('Onboarding Wizard UI Monkey Fuzzing', () => {
	test.beforeEach(async ({ page }) => {
		test.setTimeout(90000);
		await page.setViewportSize({ width: 1440, height: 900 });
		await mockOnboardingApi(page);
	});

	test('fuzzing onboarding step 1 (Business Info) with extreme inputs and transitions', async ({ page }) => {
		await page.goto('/onboarding/1');
		await page.waitForLoadState('networkidle');
		await expect(page.getByLabel('Business name')).toBeVisible({ timeout: 10000 });

		const fuzzer = new DeterministicMonkeyFuzzer(page, {
			seed: 63124,
			maxActions: 50,
			actionDelayMs: 25,
			ignoredConsoleErrors: [/favicon\.ico/i, /Failed to load resource/i]
		});

		await fuzzer.run();
		fuzzer.assertNoErrors();
	});

	test('fuzzing pipeline and team steps with rapid stage mutations and inputs', async ({ page }) => {
		await page.goto('/onboarding/3');
		await page.waitForLoadState('networkidle');

		const fuzzer = new DeterministicMonkeyFuzzer(page, {
			seed: 74215,
			maxActions: 50,
			actionDelayMs: 25,
			ignoredConsoleErrors: [/favicon\.ico/i, /Failed to load resource/i]
		});

		await fuzzer.run();
		fuzzer.assertNoErrors();
	});

	test('fuzzing AI assistant and knowledge base steps', async ({ page }) => {
		await page.goto('/onboarding/5');
		await page.waitForLoadState('networkidle');

		const fuzzer = new DeterministicMonkeyFuzzer(page, {
			seed: 85326,
			maxActions: 50,
			actionDelayMs: 25,
			ignoredConsoleErrors: [/favicon\.ico/i, /Failed to load resource/i]
		});

		await fuzzer.run();
		fuzzer.assertNoErrors();
	});
});
