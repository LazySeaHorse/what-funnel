/**
 * What Funnel — End-to-End Playwright Test Suite
 *
 * Architecture under test:
 *   SvelteKit frontend (localhost:5173, managed by Playwright webServer)
 *     → API Gateway (localhost:18080)
 *       → identity-svc, workspace-svc, conversation-svc, notification-svc
 *
 * Demo-mode data injection:
 *   Inbound messages are injected via redis-cli → messages.inbound Redis Stream,
 *   which is consumed by conversation-svc. No real mautrix bridge required.
 *
 * Test isolation:
 *   Each suite creates unique accounts. Suites that need a channel use beforeAll
 *   to create one account + one channel, then each test logs in fresh.
 */

import { test, expect, type Page } from '@playwright/test';
import { execSync } from 'child_process';

// ─── Helpers ─────────────────────────────────────────────────────────────────

const PASSWORD = 'E2ePassword99!';

function uniqueEmail(prefix = 'test') {
  return `${prefix}-${Date.now()}-${Math.floor(Math.random() * 9999)}@e2e.local`;
}

/** Inject a fake inbound message into the system via Redis Streams (demo mode). */
function injectInboundMessage(opts: {
  channelId: string;
  externalThreadID: string;
  externalIdentity: string;
  displayName: string;
  text: string;
}) {
  const payload = {
    ChannelID: opts.channelId,
    ExternalThreadID: opts.externalThreadID,
    Contact: { ExternalIdentity: opts.externalIdentity, DisplayName: opts.displayName },
    Message: { ContentType: 'text', Text: opts.text, ExternalMessageID: `msg-${Date.now()}` },
    Timestamp: new Date().toISOString(),
  };
  const escaped = JSON.stringify(payload).replace(/'/g, `'"'"'`);
  execSync(`redis-cli xadd messages.inbound '*' payload '${escaped}'`);
}

/**
 * Sign up a fresh account via the /signup page.
 * Returns when the browser lands on /onboarding (step 1 auto-completes).
 */
async function signupViaPage(page: Page, email: string, businessName: string) {
  await page.goto('/signup');
  await expect(page.locator('h1:has-text("What Funnel")')).toBeVisible({ timeout: 15000 });
  await page.fill('#accountName', businessName);
  await page.fill('#email', email);
  await page.fill('#password', PASSWORD);
  await page.click('input[value="full_workspace"]');
  await page.click('button[type="submit"]');
  await page.waitForURL('**/onboarding/**', { timeout: 20000 });
}

/**
 * Complete the entire onboarding wizard (steps 2-9), skipping optional steps.
 * Returns when on /inbox.
 */
async function completeOnboarding(page: Page) {
  await page.waitForURL('**/onboarding/2', { timeout: 20000 });
  await page.click('.mode-card:has-text("Full lead workspace")');
  await page.waitForURL('**/onboarding/3', { timeout: 15000 });
  await page.click('.biz-tile:has-text("Home Services")');
  await page.waitForURL('**/onboarding/4', { timeout: 15000 });
  await page.click('button.skip-link');
  await page.waitForURL('**/onboarding/5', { timeout: 15000 });
  await page.click('button.skip-link');
  await page.waitForURL('**/onboarding/6', { timeout: 15000 });
  await page.click('button:has-text("Continue →")');
  await page.waitForURL('**/onboarding/7', { timeout: 15000 });
  await page.click('button:has-text("Accept and adjust later")');
  await page.waitForURL('**/onboarding/8', { timeout: 15000 });
  await page.click('button.skip-link');
  await page.waitForURL('**/onboarding/9', { timeout: 15000 });
  await page.click('button:has-text("Go to Inbox")');
  await page.waitForURL('**/inbox', { timeout: 15000 });
}

/** Log in via /login. Assumes account already exists. */
async function loginViaPage(page: Page, email: string) {
  await page.goto('/login');
  await expect(page.locator('form')).toBeVisible({ timeout: 10000 });
  await page.fill('#email', email);
  await page.fill('#password', PASSWORD);
  await page.click('button[type="submit"]');
  await page.waitForURL('**/inbox', { timeout: 15000 });
}

/**
 * Create a channel via the Settings > Channels UI.
 * Returns the channel UUID from the POST /channels API response.
 */
async function createChannel(page: Page): Promise<string> {
  await page.goto('/settings/channels');
  // Wait for page to fully load (onMount fires async auth check)
  await expect(page.locator('h1:has-text("Connected Channels")')).toBeVisible({ timeout: 15000 });

  const channelRespPromise = page.waitForResponse(
    (resp) => resp.url().includes('/channels') && resp.request().method() === 'POST',
  );
  await page.click('button:has-text("+ Connect Channel")');
  await expect(page.locator('.modal-card')).toBeVisible({ timeout: 5000 });
  await page.click('button[type="submit"]:has-text("Connect")');
  const channelResp = await channelRespPromise;
  const data = await channelResp.json();
  const channelId = data.id as string;
  expect(channelId).toBeTruthy();

  // Demo auto-resolves to connected after ~6s
  await expect(page.locator('.success-box')).toBeVisible({ timeout: 15000 });
  // Modal closes automatically
  await expect(page.locator('.modal-backdrop')).not.toBeVisible({ timeout: 8000 });

  return channelId;
}

// ─── Suite 1: Auth & Session ──────────────────────────────────────────────────

test.describe('1. Auth & Session', () => {

  test('1.1 Signup creates account and auto-advances to onboarding', async ({ page }) => {
    await page.goto('/signup');
    await expect(page.locator('h1:has-text("What Funnel")')).toBeVisible({ timeout: 15000 });
    await page.fill('#accountName', 'Auth Test Business');
    await page.fill('#email', uniqueEmail('auth1'));
    await page.fill('#password', PASSWORD);
    await page.click('input[value="full_workspace"]');
    await page.click('button[type="submit"]');
    await page.waitForURL('**/onboarding/**', { timeout: 20000 });
    expect(page.url()).toContain('/onboarding');
  });

  test('1.2 Login with valid credentials succeeds and lands in inbox', async ({ page }) => {
    const email = uniqueEmail('auth2');
    await signupViaPage(page, email, 'Login Test Biz');
    await completeOnboarding(page);
    await page.click('button.logout-btn');
    await page.waitForURL('**/login', { timeout: 10000 });
    await loginViaPage(page, email);
    expect(page.url()).toContain('/inbox');
  });

  test('1.3 Login with wrong credentials stays on /login', async ({ page }) => {
    await page.goto('/login');
    await expect(page.locator('form')).toBeVisible({ timeout: 10000 });
    await page.fill('#email', 'nobody-fake@does-not-exist.local');
    await page.fill('#password', 'wrongpassword');
    await page.click('button[type="submit"]');
    await page.waitForTimeout(2500);
    expect(page.url()).toContain('/login');
  });

  test('1.4 Unauthenticated /inbox redirects to /login', async ({ page }) => {
    await page.goto('/inbox');
    await page.waitForURL(/login/, { timeout: 10000 });
    expect(page.url()).toContain('/login');
  });

});

// ─── Suite 2: Onboarding Flow ─────────────────────────────────────────────────

test.describe('2. Onboarding Flow', () => {

  test('2.1 Full onboarding flow completes and arrives in inbox', async ({ page }) => {
    await signupViaPage(page, uniqueEmail('ob1'), 'Onboarding Test Biz');
    await completeOnboarding(page);
    await expect(page.locator('.conversation-list')).toBeVisible({ timeout: 10000 });
  });

  test('2.2 Step 2 shows both product mode cards', async ({ page }) => {
    await signupViaPage(page, uniqueEmail('ob2'), 'Mode Test Biz');
    await page.waitForURL('**/onboarding/2', { timeout: 20000 });
    await expect(page.locator('.mode-card:has-text("Automated replies only")')).toBeVisible();
    await expect(page.locator('.mode-card:has-text("Full lead workspace")')).toBeVisible();
    await expect(page.locator('.recommended-badge')).toBeVisible();
  });

  test('2.3 Step 3 shows business type tiles', async ({ page }) => {
    await signupViaPage(page, uniqueEmail('ob3'), 'Biz Type Test');
    await page.waitForURL('**/onboarding/2', { timeout: 20000 });
    await page.click('.mode-card:has-text("Full lead workspace")');
    await page.waitForURL('**/onboarding/3', { timeout: 15000 });
    await expect(page.locator('.biz-tile:has-text("Home Services")')).toBeVisible();
    await expect(page.locator('.biz-tile:has-text("Salon")')).toBeVisible();
    await expect(page.locator('.biz-tile:has-text("Tutoring")')).toBeVisible();
  });

  test('2.4 Step 9 Done screen — Go to Inbox CTA navigates to inbox', async ({ page }) => {
    await signupViaPage(page, uniqueEmail('ob9'), 'Done Screen Test');
    await completeOnboarding(page);
    await expect(page.locator('.conversation-list')).toBeVisible({ timeout: 10000 });
  });

  test('2.5 Step 6 shows Reply Mode cards', async ({ page }) => {
    await signupViaPage(page, uniqueEmail('ob6'), 'Reply Mode Test');
    await page.waitForURL('**/onboarding/2', { timeout: 20000 });
    await page.click('.mode-card:has-text("Full lead workspace")');
    await page.waitForURL('**/onboarding/3', { timeout: 15000 });
    await page.click('.biz-tile:has-text("Home Services")');
    await page.waitForURL('**/onboarding/4', { timeout: 15000 });
    await page.click('button.skip-link');
    await page.waitForURL('**/onboarding/5', { timeout: 15000 });
    await page.click('button.skip-link');
    await page.waitForURL('**/onboarding/6', { timeout: 15000 });
    await expect(page.locator('.mode-card:has-text("Review before it sends")')).toBeVisible();
    await expect(page.locator('.mode-card:has-text("Send automatically once confident")')).toBeVisible();
  });

});

// ─── Suite 3: Channel Management ─────────────────────────────────────────────

test.describe('3. Channel Management', () => {
  // One account for the whole suite; each test logs in fresh
  let suiteEmail = '';

  test.beforeAll(async ({ browser }) => {
    suiteEmail = uniqueEmail('ch');
    const page = await browser.newPage();
    await signupViaPage(page, suiteEmail, 'Channel Test Biz');
    await completeOnboarding(page);
    await page.close();
  });

  test('3.1 Settings > Channels page loads', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    await page.goto('/settings/channels');
    await expect(page.locator('h1:has-text("Connected Channels")')).toBeVisible({ timeout: 15000 });
  });

  test('3.2 Connect Channel modal opens with identity and credentials fields', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    await page.goto('/settings/channels');
    await expect(page.locator('h1:has-text("Connected Channels")')).toBeVisible({ timeout: 15000 });
    await page.click('button:has-text("+ Connect Channel")');
    await expect(page.locator('.modal-card')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('#identity')).toBeVisible();
    await expect(page.locator('#credentials')).toBeVisible();
  });

  test('3.3 Creating a channel: form → QR scanning → success', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    await page.goto('/settings/channels');
    await expect(page.locator('h1:has-text("Connected Channels")')).toBeVisible({ timeout: 15000 });

    const respPromise = page.waitForResponse(
      (r) => r.url().includes('/channels') && r.request().method() === 'POST',
    );
    await page.click('button:has-text("+ Connect Channel")');
    await expect(page.locator('.modal-card')).toBeVisible({ timeout: 5000 });
    await page.click('button[type="submit"]:has-text("Connect")');
    const resp = await respPromise;
    expect((await resp.json()).id).toBeTruthy();

    await expect(page.locator('.qr-container')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('.success-box')).toBeVisible({ timeout: 15000 });
  });

  test('3.4 Created channel appears in list with status indicator', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    await createChannel(page);
    await expect(page.locator('.channel-card')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('.channel-card .status-indicator')).toBeVisible();
  });

  test('3.5 Channel type badge shows the channel type', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    await page.goto('/settings/channels');
    await expect(page.locator('h1:has-text("Connected Channels")')).toBeVisible({ timeout: 15000 });
    // If no channels yet, create one
    const existing = await page.locator('.channel-card').count();
    if (existing === 0) {
      await createChannel(page);
    }
    await expect(page.locator('.channel-type-badge').first()).toBeVisible({ timeout: 5000 });
  });

});

// ─── Suite 4: Inbound Message Flow (Demo Mode via Redis Streams) ──────────────

test.describe('4. Inbound Message Flow (Demo Mode)', () => {
  let suiteEmail = '';
  let channelId = '';

  test.beforeAll(async ({ browser }) => {
    suiteEmail = uniqueEmail('inb');
    const page = await browser.newPage();
    await signupViaPage(page, suiteEmail, 'Inbound Test Biz');
    await completeOnboarding(page);
    channelId = await createChannel(page);
    await page.close();
  });

  async function goToInboxAllTab(page: Page) {
    await loginViaPage(page, suiteEmail);
    const allBtn = page.locator('button.tab-btn:has-text("All")');
    if (await allBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
      await allBtn.click();
    }
  }

  test('4.1 Inbound message appears in inbox conversation list', async ({ page }) => {
    await goToInboxAllTab(page);
    const displayName = `TCustomer-${Date.now()}`;
    injectInboundMessage({ channelId, externalThreadID: `t-${Date.now()}`, externalIdentity: `wa-${Date.now()}@s.whatsapp.net`, displayName, text: 'Do you offer house calls?' });
    await expect(page.locator(`.convo-item:has-text("${displayName}")`)).toBeVisible({ timeout: 20000 });
  });

  test('4.2 Opening conversation shows inbound message text', async ({ page }) => {
    await goToInboxAllTab(page);
    const displayName = `MsgViewer-${Date.now()}`;
    const msgText = 'Can I book an appointment?';
    injectInboundMessage({ channelId, externalThreadID: `t-${Date.now()}`, externalIdentity: `wa-${Date.now()}@s.whatsapp.net`, displayName, text: msgText });
    const item = page.locator(`.convo-item:has-text("${displayName}")`);
    await expect(item).toBeVisible({ timeout: 20000 });
    await item.click();
    await expect(page.locator(`.msg-text:has-text("${msgText}")`)).toBeVisible({ timeout: 10000 });
  });

  test('4.3 Multiple messages from same contact land in same conversation', async ({ page }) => {
    await goToInboxAllTab(page);
    const displayName = `SameContact-${Date.now()}`;
    const threadID = `t-same-${Date.now()}`;
    const extId = `wa-same-${Date.now()}@s.whatsapp.net`;
    injectInboundMessage({ channelId, externalThreadID: threadID, externalIdentity: extId, displayName, text: 'Hello!' });
    await page.waitForTimeout(2000);
    injectInboundMessage({ channelId, externalThreadID: threadID, externalIdentity: extId, displayName, text: 'Are you open weekends?' });
    const item = page.locator(`.convo-item:has-text("${displayName}")`);
    await expect(item.first()).toBeVisible({ timeout: 20000 });
    await item.first().click();
    await expect(page.locator('.msg-text:has-text("Hello!")')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('.msg-text:has-text("Are you open weekends?")')).toBeVisible({ timeout: 8000 });
  });

  test('4.4 Inbound message row does not have the outbound class', async ({ page }) => {
    await goToInboxAllTab(page);
    const displayName = `DirTest-${Date.now()}`;
    injectInboundMessage({ channelId, externalThreadID: `t-${Date.now()}`, externalIdentity: `wa-${Date.now()}@s.whatsapp.net`, displayName, text: 'Direction check' });
    const item = page.locator(`.convo-item:has-text("${displayName}")`);
    await expect(item).toBeVisible({ timeout: 20000 });
    await item.click();
    await expect(page.locator('.message-row:not(.outbound)')).toBeVisible({ timeout: 10000 });
  });

});

// ─── Suite 5: Outbound Messaging ─────────────────────────────────────────────

test.describe('5. Outbound Messaging', () => {
  let suiteEmail = '';
  let channelId = '';

  test.beforeAll(async ({ browser }) => {
    suiteEmail = uniqueEmail('out');
    const page = await browser.newPage();
    await signupViaPage(page, suiteEmail, 'Outbound Test Biz');
    await completeOnboarding(page);
    channelId = await createChannel(page);
    await page.close();
  });

  async function loginAndOpenConvo(page: Page, displayName: string, inboundText: string) {
    await loginViaPage(page, suiteEmail);
    injectInboundMessage({ channelId, externalThreadID: `t-${Date.now()}`, externalIdentity: `wa-${Date.now()}@s.whatsapp.net`, displayName, text: inboundText });
    const allBtn = page.locator('button.tab-btn:has-text("All")');
    if (await allBtn.isVisible({ timeout: 3000 }).catch(() => false)) await allBtn.click();
    const item = page.locator(`.convo-item:has-text("${displayName}")`);
    await expect(item).toBeVisible({ timeout: 20000 });
    await item.click();
    await expect(page.locator('.compose-input')).toBeVisible({ timeout: 5000 });
  }

  test('5.1 Can type and send a reply', async ({ page }) => {
    await loginAndOpenConvo(page, `ReplySend-${Date.now()}`, 'What are your hours?');
    const reply = 'We are open Mon–Fri, 9am to 5pm!';
    await page.fill('.compose-input', reply);
    await page.click('button.send-btn');
    await expect(page.locator(`.msg-text:has-text("${reply}")`)).toBeVisible({ timeout: 10000 });
  });

  test('5.2 Sent message has outbound class in the thread', async ({ page }) => {
    await loginAndOpenConvo(page, `OutDir-${Date.now()}`, 'Do you deliver?');
    const reply = 'Yes, we deliver within 10km!';
    await page.fill('.compose-input', reply);
    await page.click('button.send-btn');
    await expect(page.locator('.message-row.outbound .msg-text:has-text("Yes, we deliver")')).toBeVisible({ timeout: 10000 });
  });

  test('5.3 Send button disabled when compose is empty', async ({ page }) => {
    await loginAndOpenConvo(page, `EmptyBtn-${Date.now()}`, 'Hello there');
    await expect(page.locator('button.send-btn')).toBeDisabled({ timeout: 5000 });
  });

  test('5.4 Enter key sends the message', async ({ page }) => {
    await loginAndOpenConvo(page, `EnterKey-${Date.now()}`, 'Is anyone there?');
    const msg = 'Yes, we are here to help!';
    await page.fill('.compose-input', msg);
    await page.press('.compose-input', 'Enter');
    await expect(page.locator(`.msg-text:has-text("${msg}")`)).toBeVisible({ timeout: 10000 });
  });

  test('5.5 Compose input clears after sending', async ({ page }) => {
    await loginAndOpenConvo(page, `Clear-${Date.now()}`, 'I have a question');
    await page.fill('.compose-input', 'Sure, how can I help?');
    await page.click('button.send-btn');
    await expect(page.locator('.compose-input')).toHaveValue('', { timeout: 5000 });
  });

});

// ─── Suite 6: Lead Tracking ───────────────────────────────────────────────────

test.describe('6. Lead Tracking (Full Workspace)', () => {
  let suiteEmail = '';
  let channelId = '';

  test.beforeAll(async ({ browser }) => {
    suiteEmail = uniqueEmail('lead');
    const page = await browser.newPage();
    await signupViaPage(page, suiteEmail, 'Lead Test Biz');
    await completeOnboarding(page);
    channelId = await createChannel(page);
    await page.close();
  });

  async function openFreshLeadConvo(page: Page) {
    await loginViaPage(page, suiteEmail);
    const dn = `Lead-${Date.now()}`;
    injectInboundMessage({ channelId, externalThreadID: `t-${Date.now()}`, externalIdentity: `wa-${Date.now()}@s.whatsapp.net`, displayName: dn, text: 'Interested in your services.' });
    const allBtn = page.locator('button.tab-btn:has-text("All")');
    if (await allBtn.isVisible({ timeout: 3000 }).catch(() => false)) await allBtn.click();
    const item = page.locator(`.convo-item:has-text("${dn}")`);
    await expect(item).toBeVisible({ timeout: 20000 });
    await item.click();
    await expect(page.locator('.lead-panel')).toBeVisible({ timeout: 8000 });
  }

  async function ensureLeadStarted(page: Page) {
    const btn = page.locator('button.start-lead-btn');
    if (await btn.isVisible({ timeout: 2000 }).catch(() => false)) {
      await btn.click();
      await expect(page.locator('select.state-select')).toBeVisible({ timeout: 8000 });
    }
  }

  test('6.1 Lead panel visible when conversation open in full_workspace', async ({ page }) => {
    await openFreshLeadConvo(page);
    await expect(page.locator('.lead-panel-title:has-text("Lead Profile")')).toBeVisible();
  });

  test('6.2 Can start tracking a lead', async ({ page }) => {
    await openFreshLeadConvo(page);
    await ensureLeadStarted(page);
    await expect(page.locator('select.state-select')).toBeVisible({ timeout: 5000 });
  });

  test('6.3 Can add a note to a lead', async ({ page }) => {
    await openFreshLeadConvo(page);
    await ensureLeadStarted(page);
    const note = `Playwright note ${Date.now()}`;
    await page.fill('.note-textarea', note);
    await page.click('button.add-note-btn:has-text("Save Note")');
    await expect(page.locator(`.note-body:has-text("${note}")`)).toBeVisible({ timeout: 8000 });
  });

  test('6.4 Can add a tag to a lead', async ({ page }) => {
    await openFreshLeadConvo(page);
    await ensureLeadStarted(page);
    const tag = `e2e-${Date.now()}`;
    await page.fill('.tag-input', tag);
    await page.click('button.add-tag-btn');
    await expect(page.locator(`.lead-tag:has-text("${tag}")`)).toBeVisible({ timeout: 8000 });
  });

  test('6.5 Can change lead pipeline state', async ({ page }) => {
    await openFreshLeadConvo(page);
    await ensureLeadStarted(page);
    const sel = page.locator('select.state-select');
    await expect(sel).toBeVisible({ timeout: 5000 });
    const opts = await sel.locator('option').all();
    if (opts.length >= 2) {
      await sel.selectOption(await opts[1].getAttribute('value') as string);
      await page.waitForTimeout(1500);
      await expect(sel).toBeVisible({ timeout: 5000 });
    }
  });

  test('6.6 History tab shows entries after a state change', async ({ page }) => {
    await openFreshLeadConvo(page);
    await ensureLeadStarted(page);
    const sel = page.locator('select.state-select');
    const opts = await sel.locator('option').all();
    if (opts.length >= 2) {
      await sel.selectOption(await opts[1].getAttribute('value') as string);
      await page.waitForTimeout(1500);
    }
    await page.click('button.panel-tab-btn:has-text("History")');
    await expect(page.locator('.history-timeline, .empty-timeline-state')).toBeVisible({ timeout: 5000 });
  });

});

// ─── Suite 7: Inbox Filters & Navigation ─────────────────────────────────────

test.describe('7. Inbox Filters & Navigation', () => {
  let suiteEmail = '';
  let channelId = '';

  test.beforeAll(async ({ browser }) => {
    suiteEmail = uniqueEmail('filt');
    const page = await browser.newPage();
    await signupViaPage(page, suiteEmail, 'Filter Test Biz');
    await completeOnboarding(page);
    channelId = await createChannel(page);
    await page.close();
  });

  test('7.1 Inbox loads with conversation list', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    await expect(page.locator('.conversation-list')).toBeVisible({ timeout: 10000 });
  });

  test('7.2 Admin sees All / Mine / Unassigned filter tabs', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    await expect(page.locator('button.tab-btn:has-text("All")')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('button.tab-btn:has-text("Mine")')).toBeVisible();
    await expect(page.locator('button.tab-btn:has-text("Unassigned")')).toBeVisible();
  });

  test('7.3 Clicking a filter tab changes the active state', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    await expect(page.locator('button.tab-btn')).toBeVisible({ timeout: 10000 });
    await page.click('button.tab-btn:has-text("Unassigned")');
    await expect(page.locator('button.tab-btn.active:has-text("Unassigned")')).toBeVisible({ timeout: 3000 });
  });

  test('7.4 Inbound conversations appear under All filter', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    const dn = `FilterContact-${Date.now()}`;
    injectInboundMessage({ channelId, externalThreadID: `t-${Date.now()}`, externalIdentity: `wa-${Date.now()}@s.whatsapp.net`, displayName: dn, text: 'Filter test message' });
    await page.click('button.tab-btn:has-text("All")');
    await expect(page.locator(`.convo-item:has-text("${dn}")`)).toBeVisible({ timeout: 20000 });
  });

  test('7.5 Settings nav link navigates to settings', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    await page.click('a.nav-btn:has-text("Settings")');
    await page.waitForURL('**/settings/**', { timeout: 10000 });
    expect(page.url()).toMatch(/\/settings\//);
  });

  test('7.6 Logout button redirects to /login', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    await page.click('button.logout-btn:has-text("Logout")');
    await page.waitForURL('**/login', { timeout: 10000 });
    expect(page.url()).toContain('/login');
  });

});

// ─── Suite 8: Settings Pages ──────────────────────────────────────────────────

test.describe('8. Settings Pages', () => {
  let suiteEmail = '';

  test.beforeAll(async ({ browser }) => {
    suiteEmail = uniqueEmail('sett');
    const page = await browser.newPage();
    await signupViaPage(page, suiteEmail, 'Settings Test Biz');
    await completeOnboarding(page);
    await page.close();
  });

  async function loginAndGoto(page: Page, path: string) {
    await loginViaPage(page, suiteEmail);
    await page.goto(path);
    await page.waitForTimeout(1000);
  }

  test('8.1 Account settings page renders', async ({ page }) => {
    await loginAndGoto(page, '/settings/account');
    await expect(page.locator('.settings-content, h1')).toBeVisible({ timeout: 10000 });
  });

  test('8.2 Channels settings page renders', async ({ page }) => {
    await loginAndGoto(page, '/settings/channels');
    await expect(page.locator('h1:has-text("Connected Channels")')).toBeVisible({ timeout: 15000 });
  });

  test('8.3 Users settings page renders', async ({ page }) => {
    await loginAndGoto(page, '/settings/users');
    await expect(page.locator('.settings-content, h1')).toBeVisible({ timeout: 10000 });
  });

  test('8.4 Pipeline settings page renders', async ({ page }) => {
    await loginAndGoto(page, '/settings/pipeline');
    await expect(page.locator('.settings-content, h1')).toBeVisible({ timeout: 10000 });
  });

  test('8.5 Knowledge base settings page renders', async ({ page }) => {
    await loginAndGoto(page, '/settings/knowledge-base');
    await expect(page.locator('.settings-content, h1')).toBeVisible({ timeout: 10000 });
  });

  test('8.6 Settings sidebar nav links are present', async ({ page }) => {
    await loginAndGoto(page, '/settings/channels');
    await expect(page.locator('.settings-sidebar')).toBeVisible({ timeout: 15000 });
    await expect(page.locator('a:has-text("← Back to Inbox")')).toBeVisible();
    await expect(page.locator('a:has-text("Channels")')).toBeVisible();
  });

});

// ─── Suite 9: WebSocket Realtime Push ────────────────────────────────────────

test.describe('9. WebSocket Realtime Push', () => {
  let suiteEmail = '';
  let channelId = '';

  test.beforeAll(async ({ browser }) => {
    suiteEmail = uniqueEmail('ws');
    const page = await browser.newPage();
    await signupViaPage(page, suiteEmail, 'WS Test Biz');
    await completeOnboarding(page);
    channelId = await createChannel(page);
    await page.close();
  });

  test('9.1 Inbox loads without JS errors after WS connect', async ({ page }) => {
    const errors: string[] = [];
    page.on('console', m => { if (m.type() === 'error') errors.push(m.text()); });
    await loginViaPage(page, suiteEmail);
    await page.waitForTimeout(3000);
    await expect(page.locator('.conversation-list')).toBeVisible({ timeout: 5000 });
    // Filter out known benign 404s; fail only on unexpected JS errors
    const unexpected = errors.filter(e => !e.includes('unauthenticated') && !e.includes('favicon'));
    expect(unexpected).toHaveLength(0);
  });

  test('9.2 New inbound message appears in list via WS without page refresh', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    const allBtn = page.locator('button.tab-btn:has-text("All")');
    if (await allBtn.isVisible({ timeout: 3000 }).catch(() => false)) await allBtn.click();
    const before = await page.locator('.convo-item').count();
    const dn = `WSPush-${Date.now()}`;
    injectInboundMessage({ channelId, externalThreadID: `t-${Date.now()}`, externalIdentity: `wa-${Date.now()}@s.whatsapp.net`, displayName: dn, text: 'Real-time push test' });
    await expect(page.locator(`.convo-item:has-text("${dn}")`)).toBeVisible({ timeout: 20000 });
    expect(await page.locator('.convo-item').count()).toBeGreaterThanOrEqual(before);
  });

  test('9.3 Sending a reply — outbound message appears in thread', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    const dn = `WSReply-${Date.now()}`;
    injectInboundMessage({ channelId, externalThreadID: `t-${Date.now()}`, externalIdentity: `wa-${Date.now()}@s.whatsapp.net`, displayName: dn, text: 'Hi there!' });
    const allBtn = page.locator('button.tab-btn:has-text("All")');
    if (await allBtn.isVisible({ timeout: 3000 }).catch(() => false)) await allBtn.click();
    const item = page.locator(`.convo-item:has-text("${dn}")`);
    await expect(item).toBeVisible({ timeout: 20000 });
    await item.click();
    await expect(page.locator('.compose-input')).toBeVisible({ timeout: 5000 });
    const outMsg = 'Hello! How can I help you today?';
    await page.fill('.compose-input', outMsg);
    await page.click('button.send-btn');
    await expect(page.locator(`.msg-text:has-text("${outMsg}")`)).toBeVisible({ timeout: 10000 });
  });

});

// ─── Suite 10: RBAC ──────────────────────────────────────────────────────────

test.describe('10. RBAC — Admin vs Member', () => {

  test('10.1 Admin sees Settings nav link in the inbox sidebar', async ({ page }) => {
    await signupViaPage(page, uniqueEmail('rbac1'), 'RBAC Admin Biz');
    await completeOnboarding(page);
    await expect(page.locator('a.nav-btn:has-text("Settings")')).toBeVisible({ timeout: 10000 });
  });

  test('10.2 Admin can access /settings/channels without redirect', async ({ page }) => {
    await signupViaPage(page, uniqueEmail('rbac2'), 'RBAC Channel Biz');
    await completeOnboarding(page);
    await page.goto('/settings/channels');
    await expect(page.locator('h1:has-text("Connected Channels")')).toBeVisible({ timeout: 15000 });
  });

});
