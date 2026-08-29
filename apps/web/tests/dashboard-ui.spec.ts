import { test, expect } from '@playwright/test';

test('dashboard UI renders correctly with What Funnel branding and Poppins font', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });

  // Sign up a fresh account
  const email = `dash-${Date.now()}@e2e.local`;
  await page.goto('/signup');
  await page.waitForLoadState('networkidle');
  await page.fill('#account-name-input', 'What Funnel Studio');
  await page.fill('#signup-email-input', email);
  await page.fill('#signup-password-input', 'E2ePassword99!');
  await page.locator('button[type="submit"]').click();

  // Wait for signup & login to finish (redirects to onboarding)
  await page.waitForURL((url) => url.pathname.includes('/onboarding') || url.pathname.includes('/inbox'), { timeout: 20000 });

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

test('leads tab UI renders real database leads with table and detail drawer', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });

  const email = `leads-real-${Date.now()}@e2e.local`;
  await page.goto('/signup');
  await page.waitForLoadState('networkidle');
  await page.fill('#account-name-input', 'Glamour Salon');
  await page.fill('#signup-email-input', email);
  await page.fill('#signup-password-input', 'E2ePassword99!');
  await page.locator('button[type="submit"]').click();

  await page.waitForURL((url) => url.pathname.includes('/onboarding') || url.pathname.includes('/inbox'), { timeout: 20000 });
  await page.goto('/inbox');
  await page.waitForLoadState('networkidle');

  // Check Leads tab initially on empty state
  const leadsNav = page.locator('button:has-text("Leads")');
  await expect(leadsNav).toBeVisible();
  await leadsNav.click();

  // Verify Leads header and 0 count
  await expect(page.locator('h1:has-text("Leads")')).toBeVisible();
  await expect(page.locator('text=No leads in this view')).toBeVisible();

  // Send a real inbound message via Simulate Studio
  const simulateNav = page.getByRole('button', { name: 'Simulate DEV' });
  await simulateNav.click();
  await expect(page.locator('h1:has-text("Customer Simulation Studio")')).toBeVisible();

  const presetBtn = page.locator('button:has-text("Hi! Do you have any weekend slots available?")');
  await presetBtn.click();
  await expect(page.locator('text=Inbound message sent to What Funnel')).toBeVisible({ timeout: 10000 });

  // Now go back to Leads tab
  await leadsNav.click();

  // Verify that real lead Alice Test appears
  await expect(page.locator('text=Alice Test').first()).toBeVisible({ timeout: 10000 });
  await expect(page.locator('text=New Lead').first()).toBeVisible();

  // Click on the lead row
  await page.locator('text=Alice Test').first().click();

  // Verify Right Detail Drawer with real lead data
  await expect(page.locator('h2:has-text("Alice Test")')).toBeVisible();
  await expect(page.getByText('Contact info', { exact: true })).toBeVisible();

  // Add a tag
  const addTagBtn = page.locator('button[title="Add tag"]');
  await addTagBtn.click();
  await page.getByLabel('Tag name').fill('VIP-Client');
  await page.getByRole('button', { name: 'Save tag' }).click();
  await expect(page.locator('text=VIP-Client')).toBeVisible({ timeout: 5000 });

  // Add a note
  await page.getByRole('complementary').getByRole('button', { name: 'Notes', exact: true }).click();
  const addNoteBtn = page.getByRole('complementary').getByRole('button', { name: '+ Add note', exact: true });
  await addNoteBtn.click();
  await page.fill('textarea[placeholder="Add an internal note..."]', 'Customer prefers Saturday afternoon.');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(page.locator('text=Customer prefers Saturday afternoon.')).toBeVisible({ timeout: 5000 });

  // Test Leads Filters dropdown
  const leadsFiltersBtn = page.getByRole('button', { name: 'Filter leads' });
  await expect(leadsFiltersBtn).toBeVisible();
  await leadsFiltersBtn.click();
  await expect(page.locator('text=Filter by Stage').or(page.locator('label:has-text("Channel")'))).toBeVisible();

  // Filter by Telegram (should hide WhatsApp lead Alice Test)
  await page.locator('#leads-channel-filter').selectOption('telegram');
  await expect(page.locator('text=No leads in this view')).toBeVisible();

  // Reset filters
  await page.getByRole('button', { name: 'Reset filters' }).click();
  await expect(page.locator('text=Alice Test').first()).toBeVisible();

  // Test Inbox Filter Options Menu
  const inboxNav = page.locator('button:has-text("Inbox")').first();
  await inboxNav.click();
  const inboxFilterBtn = page.getByRole('button', { name: 'Filter options' });
  await expect(inboxFilterBtn).toBeVisible();
  await inboxFilterBtn.click();
  await expect(page.locator('text=Filter by Stage')).toBeVisible();

  // Select Contacted stage filter (Alice is in New Lead stage, so conversation list becomes empty)
  await page.getByRole('button', { name: 'Contacted' }).click();
  await expect(page.locator('text=Stage: Contacted')).toBeVisible();

  // Clear stage filter
  await page.getByRole('button', { name: 'Clear stage filter' }).click();
  await expect(page.locator('text=Alice Test').first()).toBeVisible();

  // Take screenshot of real Leads Tab UI
  await page.screenshot({ path: 'test-results/leads-tab-screenshot.png' });
});
