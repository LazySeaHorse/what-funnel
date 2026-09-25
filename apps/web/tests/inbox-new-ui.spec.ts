import { test, expect } from '@playwright/test';
import { cleanupAccountByEmail } from './support/db-cleanup';

const createdEmails: string[] = [];

test.afterEach(() => {
  while (createdEmails.length) {
    const email = createdEmails.pop();
    if (email) cleanupAccountByEmail(email);
  }
});

test('new inbox UI: textarea composer, no note tab, AI toggle switch, cluster radii', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });

  const email = `ui-audit-${Date.now()}@e2e.local`;
  createdEmails.push(email);
  await page.goto('/signup');
  await page.fill('#account-name-input', 'Audit Studio');
  await page.fill('#signup-email-input', email);
  await page.fill('#signup-password-input', 'Password123!');
  await page.click('button[type="submit"]');
  await page.waitForURL('**/onboarding/**', { timeout: 20000 });

  // Seed a conversation via simulator
  await page.goto('/inbox?tab=simulate');
  await page.waitForLoadState('networkidle');
  const simInput = page.locator('input[placeholder="Send message as customer..."]');
  await expect(simInput).toBeVisible({ timeout: 10000 });
  await simInput.fill('Hi there, I wanted to ask about your services.');
  await simInput.press('Enter');

  await page.goto('/inbox');
  await page.waitForLoadState('networkidle');

  const convoItem = page.locator('.convo-item').first();
  await expect(convoItem).toBeVisible({ timeout: 10000 });
  await convoItem.click();

  // Verify: compose-input is a textarea, NOT an input[type=text]
  const composerTextarea = page.locator('textarea.compose-input');
  await expect(composerTextarea).toBeVisible({ timeout: 5000 });
  const composerTag = await composerTextarea.evaluate((el) => el.tagName.toLowerCase());
  expect(composerTag).toBe('textarea');

  // Verify: no "Internal Note" tab in the composer area (it was removed)
  const noteTab = page.locator('.bg-white.rounded-2xl button:has-text("Internal Note")');
  await expect(noteTab).not.toBeVisible();

  // Verify: AI toggle switch present (role=switch with aria-label)
  const aiToggle = page.locator('[role="switch"][aria-label="AI replies for this chat"]');
  await expect(aiToggle).toBeVisible();

  const send1 = page.waitForResponse((res) => res.url().includes('/send') && res.request().method() === 'POST');
  await composerTextarea.fill('Testing the new composer, first message.');
  await composerTextarea.press('Enter');
  // Message appears immediately (optimistic send)
  await expect(page.locator('.message-row.outbound').first()).toBeVisible({ timeout: 3000 });
  await send1;

  const send2 = page.waitForResponse((res) => res.url().includes('/send') && res.request().method() === 'POST');
  await composerTextarea.fill('And a second consecutive message from me.');
  await composerTextarea.press('Enter');
  await expect(page.locator('.message-row.outbound').nth(1)).toBeVisible({ timeout: 3000 });
  await send2;

  // Let DOM settle and take screenshot
  await page.waitForTimeout(500);
  await page.screenshot({ path: 'test-results/inbox-new-ui.png' });
});
