import { test, expect } from '@playwright/test';

test.describe('Form Validation Rendering', () => {
  test('submitting empty login form displays visual validation indicators', async ({ page }) => {
    await page.goto('/login');
    await page.waitForLoadState('networkidle');

    const identifierInput = page.locator('#identifier-input');
    const passwordInput = page.locator('#password-input');
    const submitBtn = page.locator('button[type="submit"]');

    await expect(identifierInput).toBeVisible();
    await expect(passwordInput).toBeVisible();
    await expect(submitBtn).toBeVisible();

    // Check required attribute
    await expect(identifierInput).toHaveAttribute('required', '');
    await expect(passwordInput).toHaveAttribute('required', '');

    // Submit form empty
    await submitBtn.click();

    // Browser HTML5 validation activates
    const isIdentifierInvalid = await identifierInput.evaluate(
      (el: HTMLInputElement) => !el.checkValidity()
    );
    expect(isIdentifierInvalid).toBe(true);

    // Now submit invalid credentials to test server-side visual error banner rendering
    await identifierInput.fill('nonexistent-user@invalid.local');
    await passwordInput.fill('WrongPassword123!');
    await submitBtn.click();

    // Assert visual error banner appears
    const errorAlert = page.locator('.wf-alert-error');
    await expect(errorAlert).toBeVisible({ timeout: 10000 });
    await expect(errorAlert).toContainText(/incorrect|invalid|error/i);
  });
});
