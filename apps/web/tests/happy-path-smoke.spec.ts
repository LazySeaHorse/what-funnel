import { test, expect } from '@playwright/test';
import { cleanupAccountByEmail } from './support/db-cleanup';

const createdEmails: string[] = [];

test.afterEach(() => {
  while (createdEmails.length) {
    const email = createdEmails.pop();
    if (email) cleanupAccountByEmail(email);
  }
});

test('happy-path smoke: registration, onboarding, send message, view in timeline', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });

  // 1. Registration
  const email = `smoke-${Date.now()}@e2e.local`;
  createdEmails.push(email);

  await page.goto('/signup');
  await page.fill('#account-name-input', 'Smoke Corp');
  await page.fill('#signup-email-input', email);
  await page.fill('#signup-password-input', 'Password123!');
  await page.click('button[type="submit"]');

  // 2. Onboarding redirect
  await page.waitForURL('**/onboarding/**', { timeout: 20000 });
  expect(page.url()).toContain('/onboarding');

  // 3. Navigate to Inbox
  await page.goto('/inbox');
  await page.waitForLoadState('networkidle');

  // 4. Use Simulator to create incoming customer conversation
  const simTab = page.getByRole('button', { name: 'Simulate DEV' });
  await expect(simTab).toBeVisible({ timeout: 10000 });
  await simTab.click();

  const simInput = page.locator('input[placeholder="Send message as customer..."]');
  await expect(simInput).toBeVisible({ timeout: 10000 });
  const testMessage = `Hello from customer ${Date.now()}`;
  await simInput.fill(testMessage);
  await simInput.press('Enter');

  // 5. Open conversation in Inbox and view in timeline
  const inboxTab = page.locator('button:has-text("Inbox")').first();
  await inboxTab.click();

  const convoItem = page.locator('.convo-item').first();
  await expect(convoItem).toBeVisible({ timeout: 10000 });
  await convoItem.click();

  // Verify inbound message in timeline
  await expect(
    page.locator(`.message-row:not(.outbound) .msg-text:has-text("${testMessage}")`)
  ).toBeVisible({ timeout: 10000 });

  // 6. Send operator message and verify it appears in the timeline
  const operatorInput = page.locator('.compose-input');
  await expect(operatorInput).toBeVisible();
  const replyMessage = `Operator reply ${Date.now()}`;
  await operatorInput.fill(replyMessage);
  await operatorInput.press('Enter');

  // Verify outbound message in timeline
  await expect(
    page.locator(`.message-row.outbound .msg-text:has-text("${replyMessage}")`)
  ).toBeVisible({ timeout: 10000 });
});
