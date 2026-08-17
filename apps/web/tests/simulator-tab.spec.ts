import { test, expect } from '@playwright/test';

test('left sidebar Simulate tab opens full Customer Simulation Studio and simulates full customer journey', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });

  // Sign up a fresh account
  const email = `sim-left-${Date.now()}@e2e.local`;
  await page.goto('/signup');
  await page.waitForLoadState('networkidle');
  await expect(page.locator('h1:has-text("What Funnel")')).toBeVisible({ timeout: 15000 });
  await page.fill('#accountName', 'What Funnel Studio Demo');
  await page.fill('#email', email);
  await page.fill('#password', 'E2ePassword99!');
  await page.click('button[type="submit"]');

  // Wait for onboarding
  await expect(page).toHaveURL(/\/onboarding/, { timeout: 20000 });

  // Navigate to inbox
  await page.goto('/inbox');
  await page.waitForLoadState('networkidle');

  // Click on the "Simulate" tab in the LEFT sidebar navigation
  const leftSimulateTab = page.getByRole('button', { name: 'Simulate DEV' });
  await expect(leftSimulateTab).toBeVisible();
  await leftSimulateTab.click();

  // Verify full Customer Simulation Studio view is displayed
  await expect(page.locator('h1:has-text("Customer Simulation Studio")')).toBeVisible();
  await expect(page.locator('text=Simulate Customer')).toBeVisible();
  await expect(page.locator('text=Customer Phone View')).toBeVisible();

  // Click on a preset prompt button to send as Alice Test
  const presetBtn = page.locator('button:has-text("Hi! Do you have any weekend slots available?")');
  await expect(presetBtn).toBeVisible();
  await presetBtn.click();

  // Wait for success indicator
  await expect(page.locator('text=Inbound message sent to What Funnel')).toBeVisible({ timeout: 10000 });

  // Take a screenshot of the Customer Simulation Studio view
  await page.screenshot({ path: 'test-results/simulator-studio-screenshot.png' });

  // Click "Back to Inbox"
  const backToInboxBtn = page.locator('button:has-text("Back to Inbox")');
  await backToInboxBtn.click();

  // Verify that the conversation now appears in the main Inbox list on the left
  await expect(page.locator('text=Alice Test').first()).toBeVisible({ timeout: 10000 });

  // Click on the newly created conversation in the inbox to view it in the center panel
  await page.locator('text=Alice Test').first().click();

  // Wait for message to render in chat view
  await expect(page.locator('.msg-text:has-text("Hi! Do you have any weekend slots available?")').first()).toBeVisible({ timeout: 5000 });

  // Take screenshot of the inbox after simulation
  await page.screenshot({ path: 'test-results/simulator-inbox-screenshot.png' });
});

test('simulating Telegram chat sends native webhook and displays Telegram channel badge and details in inbox', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });

  // Sign up a fresh account
  const email = `sim-tg-${Date.now()}@e2e.local`;
  await page.goto('/signup');
  await page.waitForLoadState('networkidle');
  await expect(page.locator('h1:has-text("What Funnel")')).toBeVisible({ timeout: 15000 });
  await page.fill('#accountName', 'Telegram Sim Test Studio');
  await page.fill('#email', email);
  await page.fill('#password', 'E2ePassword99!');
  await page.click('button[type="submit"]');

  // Wait for onboarding
  await expect(page).toHaveURL(/\/onboarding/, { timeout: 20000 });

  // Navigate to inbox
  await page.goto('/inbox');
  await page.waitForLoadState('networkidle');

  // Click on the "Simulate" tab in the LEFT sidebar navigation
  const leftSimulateTab = page.getByRole('button', { name: 'Simulate DEV' });
  await expect(leftSimulateTab).toBeVisible();
  await leftSimulateTab.click();

  // Click on Dana Telegram persona (or Telegram platform)
  const tgPersonaBtn = page.locator('button:has-text("Dana Telegram")');
  await expect(tgPersonaBtn).toBeVisible();
  await tgPersonaBtn.click();

  // Click on a Telegram preset prompt button
  const tgPresetBtn = page.locator('button:has-text("Hi! Interested in your services")');
  await expect(tgPresetBtn).toBeVisible();
  await tgPresetBtn.click();

  // Wait for success indicator
  await expect(page.locator('text=Inbound message sent to What Funnel')).toBeVisible({ timeout: 10000 });

  // Click "Back to Inbox"
  const backToInboxBtn = page.locator('button:has-text("Back to Inbox")');
  await backToInboxBtn.click();

  // Verify that Dana Telegram appears in the main Inbox list
  await expect(page.locator('text=Dana Telegram').first()).toBeVisible({ timeout: 10000 });

  // Click on Dana Telegram conversation
  await page.locator('text=Dana Telegram').first().click();

  // Check that the header shows Telegram
  await expect(page.locator('p:has-text("Telegram")').first()).toBeVisible({ timeout: 5000 });
});

