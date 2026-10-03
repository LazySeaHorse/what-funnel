import { test, expect } from '@playwright/test';
import { mockIdleWebSocket, mockWorkspaceApi } from '../support/mock-api';
import { DeterministicMonkeyFuzzer } from './monkey';

test.describe('Auth UI Monkey Fuzzing', () => {
	test.beforeEach(async ({ page }) => {
		test.setTimeout(60000);
		await page.setViewportSize({ width: 1440, height: 900 });
		await mockIdleWebSocket(page);
		await mockWorkspaceApi(page);
	});

	test('fuzzing /login form inputs, toggles, and rapid submissions', async ({ page }) => {
		await page.goto('/login');
		await page.waitForLoadState('networkidle');
		await expect(page.locator('h2:has-text("Sign in")')).toBeVisible({ timeout: 10000 });

		const fuzzer = new DeterministicMonkeyFuzzer(page, {
			seed: 41235,
			maxActions: 50,
			actionDelayMs: 25,
			ignoredConsoleErrors: [/Failed to load resource: the server responded with a status of 401/i, /The username or password is incorrect/i]
		});

		await fuzzer.run();
		fuzzer.assertNoErrors();
	});

	test('fuzzing /signup form inputs, mode selectors, and submissions', async ({ page }) => {
		await page.goto('/signup');
		await page.waitForLoadState('networkidle');
		await expect(page.locator('h2:has-text("Create workspace")')).toBeVisible({ timeout: 10000 });

		const fuzzer = new DeterministicMonkeyFuzzer(page, {
			seed: 52341,
			maxActions: 50,
			actionDelayMs: 25,
			ignoredConsoleErrors: [/Failed to load resource: the server responded with a status of (400|409|422)/i, /Failed to create workspace/i]
		});

		await fuzzer.run();
		fuzzer.assertNoErrors();
	});
});
