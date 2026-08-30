import { test, expect } from '@playwright/test';

test('sending messages in inbox and simulator tabs renders correctly in both views', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });

  // Sign up fresh account
  const email = `msg-flow-${Date.now()}@e2e.local`;
  await page.goto('/signup');
  await page.fill('#account-name-input', 'Realtime Sync Studio');
  await page.fill('#signup-email-input', email);
  await page.fill('#signup-password-input', 'Password123!');
  await expect(page.getByRole('radio', { name: /Full workspace/i })).toBeChecked();
  await page.click('button[type="submit"]');

  await page.waitForURL('**/onboarding/**', { timeout: 20000 });
  await page.goto('/inbox');
  await page.waitForLoadState('networkidle');

  // 1. Go to Simulator tab
  const simTab = page.getByRole('button', { name: 'Simulate DEV' });
  await expect(simTab).toBeVisible();
  await simTab.click();

  // 2. Send custom customer message from simulator
  const simInput = page.locator('input[placeholder="Send message as customer..."]');
  await expect(simInput).toBeVisible();
  await simInput.fill('Hey, what are your opening hours today?');
  await simInput.press('Enter');

  // Verify message appears in Customer Phone View
  await expect(page.locator('.sim-bubble:has-text("Hey, what are your opening hours today?")')).toBeVisible({ timeout: 10000 });

  // 3. Navigate to Inbox tab
  const inboxTab = page.locator('button:has-text("Inbox")').first();
  await inboxTab.click();

  // Select Alice Test conversation
  const aliceItem = page.locator('.convo-item:has-text("Alice Test")').first();
  await expect(aliceItem).toBeVisible({ timeout: 10000 });
  await aliceItem.click();

  // Verify customer message appears in white bubble on the left
  await expect(page.locator('.message-row:not(.outbound) .msg-text:has-text("Hey, what are your opening hours today?")')).toBeVisible({ timeout: 10000 });

  // 4. Send operator response from Inbox
  const operatorInput = page.locator('.compose-input');
  await expect(operatorInput).toBeVisible();
  await operatorInput.fill('We are open until 8 PM today! Feel free to drop by.');
  await operatorInput.press('Enter');

  // Verify operator message immediately appears in blue bubble on the right
  await expect(page.locator('.message-row.outbound .msg-text:has-text("We are open until 8 PM today! Feel free to drop by.")')).toBeVisible({ timeout: 10000 });

  // 5. Switch back to Simulator tab and verify operator message also synced back to customer phone view!
  await simTab.click();
  await expect(page.locator('.sim-bubble:has-text("We are open until 8 PM today! Feel free to drop by.")')).toBeVisible({ timeout: 10000 });

  // Take screenshot of fully synchronized customer live chat
  await page.screenshot({ path: 'test-results/messaging-flow-synced.png' });
});
