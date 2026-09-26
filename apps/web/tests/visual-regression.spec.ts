import { test, expect } from '@playwright/test';
import { cleanupAccountByEmail } from './support/db-cleanup';

const createdEmails: string[] = [];

test.afterEach(() => {
  while (createdEmails.length) {
    const email = createdEmails.pop();
    if (email) cleanupAccountByEmail(email);
  }
});

test.describe('Visual Regression Snapshots', () => {
  test('login page visual snapshot', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto('/login');
    await page.waitForLoadState('networkidle');

    await expect(page.locator('#identifier-input')).toBeVisible();
    await expect(page).toHaveScreenshot('login-screen.png', {
      maxDiffPixelRatio: 0.1,
      animations: 'disabled',
    });
  });

  test('inbox screen visual snapshot', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });

    const email = `visual-${Date.now()}@e2e.local`;
    createdEmails.push(email);

    await page.goto('/signup');
    await page.fill('#account-name-input', 'Visual Studio');
    await page.fill('#signup-email-input', email);
    await page.fill('#signup-password-input', 'Password123!');
    await page.click('button[type="submit"]');

    await page.waitForURL('**/onboarding/**', { timeout: 20000 });
    await page.goto('/inbox');
    await page.waitForLoadState('networkidle');

    // Wait for the main inbox elements to be rendered
    await expect(page.locator('button:has-text("Inbox")').first()).toBeVisible({ timeout: 10000 });

    // Snapshot the inbox page
    await expect(page).toHaveScreenshot('inbox-screen.png', {
      maxDiffPixelRatio: 0.1,
      animations: 'disabled',
      mask: [page.locator('.text-slate-500:has-text("@")')], // mask dynamic email/dates if any
    });
  });
});
