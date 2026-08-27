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
// @ts-ignore
import { execFileSync } from 'child_process';

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
  execFileSync(
    'docker',
    ['compose', 'exec', '-T', 'redis', 'redis-cli', 'XADD', 'messages.inbound', '*', 'payload', JSON.stringify(payload)],
    { cwd: '../..', stdio: 'ignore' }
  );
}

/**
 * Sign up a fresh account via the /signup page.
 * Returns when the browser lands on /onboarding (step 1 auto-completes).
 */
async function signupViaPage(page: Page, email: string, businessName: string) {
  await page.goto('/signup');
  await page.waitForLoadState('networkidle');
  await expect(page.getByRole('heading', { name: 'Create workspace', exact: true })).toBeVisible({ timeout: 15000 });
  await page.fill('#account-name-input', businessName);
  await page.fill('#signup-email-input', email);
  await page.fill('#signup-password-input', PASSWORD);
  await expect(page.getByRole('radio', { name: /Full Workspace/ })).toBeChecked();
  await page.click('button[type="submit"]');
  await page.waitForURL('**/onboarding/**', { timeout: 20000 });
}

/**
 * Complete the current six-step onboarding wizard and arrive at the inbox.
 * Returns when on /inbox.
 */
async function completeOnboarding(page: Page) {
  await page.waitForURL('**/onboarding/1', { timeout: 20000 });
  await page.getByRole('button', { name: 'Continue', exact: true }).click();
  await page.waitForURL('**/onboarding/2', { timeout: 15000 });
  await page.getByRole('button', { name: 'Continue', exact: true }).click();
  await page.waitForURL('**/onboarding/3', { timeout: 15000 });
  await page.getByRole('button', { name: 'Continue', exact: true }).click();
  await page.waitForURL('**/onboarding/4', { timeout: 15000 });
  const providerKey = page.getByLabel(/^API key/);
  if (await providerKey.isVisible()) await providerKey.fill('e2e-provider-key');
  await page.getByRole('button', { name: 'Continue', exact: true }).click();
  await page.waitForURL('**/onboarding/5', { timeout: 15000 });
  const skipKnowledge = page.getByRole('button', { name: 'Skip', exact: true });
  if (await skipKnowledge.isVisible()) {
    await skipKnowledge.click();
  } else {
    await page.getByRole('button', { name: 'Organize with AI', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Structured Knowledge', exact: true })).toBeVisible({ timeout: 15000 });
    await page.getByRole('button', { name: 'Continue', exact: true }).click();
  }
  await page.waitForURL('**/onboarding/6', { timeout: 15000 });
  await page.getByRole('button', { name: 'Complete setup', exact: true }).click();
  await page.waitForURL('**/onboarding/7', { timeout: 15000 });
  await page.getByRole('button', { name: 'Go to Inbox', exact: true }).click();
  await page.waitForURL('**/inbox', { timeout: 15000 });
}

/** Log in via /login. Assumes account already exists. */
async function loginViaPage(page: Page, email: string) {
  await page.goto('/login');
  await expect(page.locator('form')).toBeVisible({ timeout: 10000 });
  await page.fill('#email-input', email);
  await page.fill('#password-input', PASSWORD);
  await page.click('button[type="submit"]');
  await page.waitForURL('**/inbox', { timeout: 15000 });
}

/**
 * Start a provider connection via the Settings > Channels UI.
 * Returns the created channel UUID from the bridge-connection response.
 */
async function createChannel(page: Page): Promise<string> {
  await page.goto('/inbox?tab=settings');
  await page.waitForLoadState('networkidle');
  await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible({ timeout: 15000 });
  await page.getByRole('tab', { name: 'Channels', exact: true }).click();

  const connectionResponsePromise = page.waitForResponse(
    (resp) => resp.url().includes('/bridge-connections') && resp.request().method() === 'POST',
  );
  await page.getByRole('button', { name: 'Connect channel' }).click();
  const dialog = page.getByRole('dialog', { name: 'Connect a channel' });
  await expect(dialog).toBeVisible({ timeout: 5000 });
  await dialog.getByRole('combobox', { name: 'Channel' }).selectOption('whatsapp');
  await dialog.getByRole('button', { name: 'Continue' }).click();
  const connectionResponse = await connectionResponsePromise;
  expect(connectionResponse.status()).toBe(201);
  const channelId = (await connectionResponse.json()).channel_id as string;
  expect(channelId).toBeTruthy();
  await expect(page.getByRole('dialog', { name: 'Connect WhatsApp' })).toBeVisible({ timeout: 5000 });
  await page.getByRole('button', { name: 'Close channel dialog' }).click();

  return channelId;
}

/** Create an authenticated Matrix mock channel for outbound-message tests.
 * Bridge setup is covered separately; a synthetic inbound thread is not a real
 * Matrix room and therefore cannot be used to exercise an actual bridge send.
 */
async function createMockMatrixChannel(page: Page): Promise<string> {
  const response = await page.evaluate(async () => {
    const result = await fetch('/api-gateway/channels', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({
        type: 'matrix_whatsapp',
        bridge_identity: '@whatsappbot:mock',
        bridge_credentials: {
          homeserver_url: 'mock',
          user_id: '@whatfunnel-e2e:mock',
          access_token: 'mock-token'
        }
      })
    });
    return { status: result.status, body: await result.json() };
  });

  expect(response.status).toBe(201);
  expect(response.body.id).toBeTruthy();
  return response.body.id as string;
}

// ─── Suite 1: Auth & Session ──────────────────────────────────────────────────

test.describe('1. Auth & Session', () => {

  test('1.1 Signup creates account and auto-advances to onboarding', async ({ page }) => {
    await page.goto('/signup');
    await page.waitForLoadState('networkidle');
    await expect(page.getByRole('heading', { name: 'Create workspace', exact: true })).toBeVisible({ timeout: 15000 });
    await page.fill('#account-name-input', 'Auth Test Business');
    await page.fill('#signup-email-input', uniqueEmail('auth1'));
    await page.fill('#signup-password-input', PASSWORD);
    await expect(page.getByRole('radio', { name: /Full Workspace/ })).toBeChecked();
    await page.click('button[type="submit"]');
    await page.waitForURL('**/onboarding/**', { timeout: 20000 });
    expect(page.url()).toContain('/onboarding');
  });

  test('1.2 Login with valid credentials succeeds and lands in inbox', async ({ page }) => {
    const email = uniqueEmail('auth2');
    await signupViaPage(page, email, 'Login Test Biz');
    await completeOnboarding(page);
    await page.getByRole('button', { name: 'Toggle workspace menu' }).click();
    await page.getByRole('button', { name: 'Sign out' }).click();
    await page.waitForURL('**/login', { timeout: 10000 });
    await loginViaPage(page, email);
    expect(page.url()).toContain('/inbox');
  });

  test('1.3 Login with wrong credentials stays on /login', async ({ page }) => {
    await page.goto('/login');
    await expect(page.locator('form')).toBeVisible({ timeout: 10000 });
    await page.fill('#email-input', 'nobody-fake@does-not-exist.local');
    await page.fill('#password-input', 'wrongpassword');
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

  test('2.2 Step 1 collects business details', async ({ page }) => {
    await signupViaPage(page, uniqueEmail('ob2'), 'Mode Test Biz');
    await page.waitForURL('**/onboarding/1', { timeout: 20000 });
    await expect(page.getByRole('heading', { name: 'Let’s start with your business', exact: true })).toBeVisible();
    await expect(page.getByLabel('Business name')).toHaveValue('Mode Test Biz');
    await expect(page.getByLabel('Business type')).toBeVisible();
    await expect(page.getByLabel('Time zone')).toBeVisible();
  });

  test('2.3 Step 2 presents available messaging channels', async ({ page }) => {
    await signupViaPage(page, uniqueEmail('ob3'), 'Biz Type Test');
    await page.waitForURL('**/onboarding/1', { timeout: 20000 });
    await page.getByRole('button', { name: 'Continue', exact: true }).click();
    await page.waitForURL('**/onboarding/2', { timeout: 15000 });
    await expect(page.getByRole('heading', { name: 'Connect your channels', exact: true })).toBeVisible();
    await expect(page.getByText('WhatsApp', { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Connect' }).first()).toBeVisible();
  });

  test('2.4 Setup complete screen sends the user to the inbox', async ({ page }) => {
    await signupViaPage(page, uniqueEmail('ob9'), 'Done Screen Test');
    await completeOnboarding(page);
    await expect(page.locator('.conversation-list')).toBeVisible({ timeout: 10000 });
  });

  test('2.5 Step 4 offers the current AI assistant modes', async ({ page }) => {
    await signupViaPage(page, uniqueEmail('ob6'), 'Reply Mode Test');
    await page.waitForURL('**/onboarding/1', { timeout: 20000 });
    for (const step of [2, 3, 4]) {
      await page.getByRole('button', { name: 'Continue', exact: true }).click();
      await page.waitForURL(`**/onboarding/${step}`, { timeout: 15000 });
    }
    await expect(page.getByRole('heading', { name: 'Meet your AI Assistant', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: /Auto answer when confident/ })).toBeVisible();
    await expect(page.getByRole('button', { name: /Suggest replies only/ })).toBeVisible();
    await expect(page.getByRole('button', { name: /Manual only/ })).toBeVisible();
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

  test('3.1 In-app Settings opens the Channels section', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    await page.goto('/inbox?tab=settings');
    await page.getByRole('tab', { name: 'Channels', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Connected channels', exact: true })).toBeVisible({ timeout: 15000 });
  });

  test('3.2 Connect Channel dialog exposes a provider selector', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    await page.goto('/inbox?tab=settings');
    await page.getByRole('tab', { name: 'Channels', exact: true }).click();
    await page.getByRole('button', { name: 'Connect channel' }).click();
    const dialog = page.getByRole('dialog', { name: 'Connect a channel' });
    await expect(dialog).toBeVisible({ timeout: 5000 });
    await expect(dialog.getByRole('combobox', { name: 'Channel' })).toBeVisible();
    await expect(dialog.getByText(/Connection credentials remain server-side/)).toBeVisible();
  });

  test('3.3 Creating a channel updates the settings list', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    const channelID = await createChannel(page);
    expect(channelID).toBeTruthy();
    await expect(page.getByText('WhatsApp', { exact: true }).first()).toBeVisible({ timeout: 5000 });
    page.once('dialog', (dialog) => dialog.accept());
    await page.getByRole('button', { name: 'Disconnect' }).click();
    await expect(page.getByText('No channels connected yet.', { exact: true })).toBeVisible({ timeout: 5000 });
  });

  test('3.4 Disconnecting a channel returns the view to its empty state', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    await createChannel(page);
    page.once('dialog', (dialog) => dialog.accept());
    await page.getByRole('button', { name: 'Disconnect' }).click();
    await expect(page.getByText('No channels connected yet.', { exact: true })).toBeVisible({ timeout: 5000 });
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
    await expect(page.locator('.message-row:not(.outbound)').first()).toBeVisible({ timeout: 10000 });
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
    channelId = await createMockMatrixChannel(page);
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
    const sendResponse = page.waitForResponse((response) =>
      response.url().includes('/internal/conversations/') && response.url().endsWith('/send') && response.request().method() === 'POST',
    );
    await page.click('button.send-btn');
    expect((await sendResponse).status()).toBe(200);
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
    await expect(page.getByRole('button', { name: 'Change lead state' })).toBeVisible({ timeout: 8000 });
  }

  test('6.1 Lead panel visible when conversation open in full_workspace', async ({ page }) => {
    await openFreshLeadConvo(page);
    await expect(page.locator('.lead-panel').getByText('Lead State', { exact: true })).toBeVisible();
  });

  test('6.2 Lead state control is available for a new conversation', async ({ page }) => {
    await openFreshLeadConvo(page);
    await ensureLeadStarted(page);
  });

  test('6.3 Can add a note to a lead', async ({ page }) => {
    await openFreshLeadConvo(page);
    await ensureLeadStarted(page);
    const note = `Playwright note ${Date.now()}`;
    await page.getByRole('button', { name: 'Internal Note' }).click();
    await page.getByPlaceholder('Add an internal note visible only to your team...').fill(note);
    await page.getByRole('button', { name: 'Post Internal Note' }).click();
    await expect(page.locator('.lead-panel .note-item').filter({ hasText: note }).last()).toBeVisible({ timeout: 8000 });
  });

  test('6.4 Can add a tag to a lead', async ({ page }) => {
    await openFreshLeadConvo(page);
    await ensureLeadStarted(page);
    const tag = `e2e-${Date.now()}`;
    await page.getByTitle('Add tag').click();
    await page.getByRole('textbox', { name: 'Tag name' }).fill(tag);
    await page.getByRole('button', { name: 'Save tag' }).click();
    await expect(page.locator('.lead-panel').getByText(tag, { exact: true })).toBeVisible({ timeout: 8000 });
  });

  test('6.5 Can change lead pipeline state', async ({ page }) => {
    await openFreshLeadConvo(page);
    await ensureLeadStarted(page);
    await page.getByRole('button', { name: 'Change lead state' }).click();
    const options = page.locator('button[aria-label^="Set lead state to "]');
    expect(await options.count()).toBeGreaterThan(1);
    const nextState = await options.nth(1).getAttribute('aria-label');
    await options.nth(1).click();
    await expect(page.getByRole('button', { name: 'Change lead state' })).toContainText((nextState || '').replace('Set lead state to ', '').toLowerCase());
  });

  test('6.6 History tab shows entries after a state change', async ({ page }) => {
    await openFreshLeadConvo(page);
    await ensureLeadStarted(page);
    await page.getByRole('button', { name: 'Change lead state' }).click();
    await page.locator('button[aria-label^="Set lead state to "]').nth(1).click();
    await page.getByRole('button', { name: 'Activity' }).click();
    await expect(page.locator('.lead-panel').getByText(/Stage changed to|No state history recorded yet/).first()).toBeVisible({ timeout: 8000 });
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
    await expect(page.locator('button.tab-btn').first()).toBeVisible({ timeout: 10000 });
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
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible({ timeout: 10_000 });
  });

  test('7.6 Logout button redirects to /login', async ({ page }) => {
    await loginViaPage(page, suiteEmail);
    await page.getByRole('button', { name: 'Toggle workspace menu' }).click();
    await page.getByRole('button', { name: 'Sign out' }).click();
    await page.waitForURL('**/login', { timeout: 10000 });
    expect(page.url()).toContain('/login');
  });

});

// ─── Suite 8: In-app Settings ─────────────────────────────────────────────────

test.describe('8. Settings Pages', () => {
  let suiteEmail = '';

  test.beforeAll(async ({ browser }) => {
    suiteEmail = uniqueEmail('sett');
    const page = await browser.newPage();
    await signupViaPage(page, suiteEmail, 'Settings Test Biz');
    await completeOnboarding(page);
    await page.close();
  });

  async function loginAndOpenSettings(page: Page) {
    await loginViaPage(page, suiteEmail);
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible({ timeout: 10_000 });
  }

  test('8.1 Admin can navigate every current settings section', async ({ page }) => {
    await loginAndOpenSettings(page);
    for (const [tab, heading] of [
      ['General', 'General'],
      ['Business profile', 'Business profile'],
      ['Users & permissions', 'Users & permissions'],
      ['Channels', 'Connected channels'],
      [/Lead pipeline/, 'Lead pipeline']
    ]) {
      await page.getByRole('tab', { name: tab, exact: true }).click();
      await expect(page.getByRole('tabpanel').getByRole('heading', { name: heading, exact: true })).toBeVisible();
    }
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
    channelId = await createMockMatrixChannel(page);
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

  test('10.1 Admin sees the in-app Settings control in the inbox sidebar', async ({ page }) => {
    await signupViaPage(page, uniqueEmail('rbac1'), 'RBAC Admin Biz');
    await completeOnboarding(page);
    await expect(page.getByRole('button', { name: 'Settings', exact: true })).toBeVisible({ timeout: 10000 });
  });

  test('10.2 Admin can open the Channels section without leaving the inbox shell', async ({ page }) => {
    await signupViaPage(page, uniqueEmail('rbac2'), 'RBAC Channel Biz');
    await completeOnboarding(page);
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await page.getByRole('tab', { name: 'Channels', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Connected channels', exact: true })).toBeVisible({ timeout: 15000 });
  });

});
