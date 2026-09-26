import { test, expect } from '@playwright/test';

test.describe('Route Guard Protection', () => {
  test('unauthenticated visit to /inbox redirects to /login', async ({ page }) => {
    // Navigate directly without cookies/session
    await page.goto('/inbox');
    await page.waitForURL('**/login', { timeout: 15000 });
    expect(page.url()).toContain('/login');
    await expect(page.locator('#identifier-input')).toBeVisible();
  });

  test('unauthenticated visit to /onboarding redirects to /login', async ({ page }) => {
    await page.goto('/onboarding');
    await page.waitForURL('**/login', { timeout: 15000 });
    expect(page.url()).toContain('/login');
  });
});
