import { test, expect } from '@playwright/test';
import { DeterministicMonkeyFuzzer } from './monkey';

test.describe('Live Backend UI Monkey Fuzzing', () => {
	test('runs deterministic monkey testing against real backend services and database', async ({ page }) => {
		test.setTimeout(90000);
		await page.setViewportSize({ width: 1440, height: 900 });

		const email = `monkey-live-${Date.now()}@e2e.local`;

		// 1. Sign up a fresh isolated tenant workspace
		await page.goto('/signup');
		await page.waitForLoadState('networkidle');

		await expect(page.locator('#account-name-input')).toBeVisible({ timeout: 15000 });
		await page.fill('#account-name-input', 'Chaos Monkey Lab');
		await page.fill('#signup-email-input', email);
		await page.fill('#signup-password-input', 'SecureChaosP@ss123!');
		await page.locator('button[type="submit"]').click();

		// 2. Wait for onboarding or direct redirect to inbox
		await page.waitForURL(
			(url) => url.pathname.includes('/onboarding') || url.pathname.includes('/inbox'),
			{ timeout: 25000 }
		);

		await page.goto('/inbox');
		await page.waitForLoadState('networkidle');
		await expect(page.locator('h1:has-text("Inbox")')).toBeVisible({ timeout: 15000 });

		// 3. Inject simulated inbound conversation via Simulate Studio to seed live data
		const simulateNav = page.getByRole('button', { name: 'Simulate DEV' });
		if (await simulateNav.isVisible().catch(() => false)) {
			await simulateNav.click();
			await expect(page.locator('h1:has-text("Simulate")')).toBeVisible({ timeout: 10000 });

			const presetBtn = page.locator('button:has-text("Hi! Do you have any weekend slots available?")');
			if (await presetBtn.isVisible().catch(() => false)) {
				await presetBtn.click();
				await page.waitForTimeout(1000);
			}

			const backToInboxBtn = page.locator('button:has-text("Back to Inbox")');
			if (await backToInboxBtn.isVisible().catch(() => false)) {
				await backToInboxBtn.click();
				await page.waitForTimeout(500);
			}
		}

		// 4. Run the deterministic monkey against the live stack
		const fuzzer = new DeterministicMonkeyFuzzer(page, {
			maxActions: 70,
			actionDelayMs: 40
		});

		await fuzzer.run();
		fuzzer.assertNoErrors();
	});
});
