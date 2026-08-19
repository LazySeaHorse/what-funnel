import { test, expect } from '@playwright/test';

test('dashboard UI renders correctly with What Funnel branding and Poppins font', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });

  // Sign up a fresh account
  const email = `dash-${Date.now()}@e2e.local`;
  await page.goto('/signup');
  await page.fill('#account-name-input', 'What Funnel Studio');
  await page.fill('#signup-email-input', email);
  await page.fill('#signup-password-input', 'E2ePassword99!');
  await page.click('button[type="submit"]');

  // Wait for signup & login to finish (redirects to onboarding)
  await page.waitForURL('**/onboarding/**', { timeout: 20000 });

  // Navigate to inbox
  await page.goto('/inbox');
  await page.waitForLoadState('networkidle');

  // Verify page title
  await expect(page).toHaveTitle(/What Funnel/);

  // Check branding and navigation
  const brandText = page.locator('text=What Funnel');
  await expect(brandText.first()).toBeVisible();

  const inboxNav = page.locator('button:has-text("Inbox")');
  await expect(inboxNav).toBeVisible();

  // Check 3-column layout components
  const inboxHeader = page.locator('h1:has-text("Inbox")');
  await expect(inboxHeader).toBeVisible();

  // Check font family is Poppins
  const bodyFont = await page.evaluate(() => {
    return window.getComputedStyle(document.body).fontFamily;
  });
  console.log('Body font family:', bodyFont);
  expect(bodyFont.toLowerCase()).toContain('poppins');

  // Take screenshot of the entire dashboard at 1440x900
  await page.screenshot({ path: 'test-results/dashboard-screenshot.png' });
});
