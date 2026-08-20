import { expect, test } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

test.describe('in-app settings safety net', () => {
	async function openMockedSettings(page: Parameters<typeof mockWorkspaceApi>[0]) {
		await mockWorkspaceApi(page);
		await page.goto('/inbox?tab=settings');
		await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible();
	}

	test('every section is reachable and exposes the matching accessible panel', async ({ page }) => {
		await openMockedSettings(page);

		for (const section of [
			['General', 'General'],
			['Business profile', 'Business profile'],
			['Users & permissions', 'Users & permissions'],
			['Channels', 'Connected channels'],
			[/Lead pipeline/, 'Lead pipeline']
		] as const) {
			const [tabName, heading] = section;
			const tab = page.getByRole('tab', { name: tabName, exact: true });
			await tab.click();
			await expect(tab).toHaveAttribute('aria-selected', 'true');
			await expect(page.getByRole('tabpanel').getByRole('heading', { name: heading, exact: true })).toBeVisible();
		}
	});

	test('general settings persist through a full page refresh', async ({ page }) => {
		const workspaceName = `Persisted Workspace ${Date.now()}`;
		await openMockedSettings(page);

		await page.getByLabel('Workspace name').fill(workspaceName);
		await page.getByLabel('Default time zone').selectOption('(GMT+00:00) UTC');
		await page.getByRole('button', { name: 'Save changes' }).click();
		await expect(page.getByText('Settings saved successfully', { exact: true })).toBeVisible();

		await page.reload();
		await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible({ timeout: 20_000 });
		await expect(page.getByLabel('Workspace name')).toHaveValue(workspaceName);
		await expect(page.getByLabel('Default time zone')).toHaveValue('(GMT+00:00) UTC');
	});

	test('a failed save preserves edits and gives the user a recoverable error', async ({ page }) => {
		await openMockedSettings(page);
		await page.route('**/api-gateway/workspace/account/settings', (route) =>
			route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'Settings service is unavailable' }) })
		);

		await page.getByLabel('Default time zone').selectOption('(GMT+00:00) UTC');
		await page.getByRole('button', { name: 'Save changes' }).click();

		await expect(page.getByText('Settings service is unavailable', { exact: true })).toBeVisible();
		await expect(page.getByLabel('Default time zone')).toHaveValue('(GMT+00:00) UTC');
	});

	test('invite modal can be dismissed by keyboard without changing workspace state', async ({ page }) => {
		await openMockedSettings(page);
		await page.getByRole('tab', { name: 'Users & permissions', exact: true }).click();
		await page.getByRole('button', { name: 'Invite user' }).click();

		const dialog = page.getByRole('dialog', { name: 'Invite Team Member' });
		await expect(dialog).toBeVisible();
		await page.getByLabel('Email Address').fill('new-member@example.test');
		await page.keyboard.press('Escape');
		await expect(dialog).not.toBeVisible();
		await expect(page.getByText('Invitation sent', { exact: true })).not.toBeVisible();
	});

	test('channel connection dialog can be cancelled with Escape', async ({ page }) => {
		await openMockedSettings(page);
		await page.getByRole('tab', { name: 'Channels', exact: true }).click();
		await page.getByRole('button', { name: 'Connect channel' }).click();
		await expect(page.getByRole('dialog', { name: 'Connect a channel' })).toBeVisible();
		await page.keyboard.press('Escape');
		await expect(page.getByRole('dialog', { name: 'Connect a channel' })).not.toBeVisible();
	});

	test('starting and disconnecting a provider connection updates the rendered state', async ({ page }) => {
		await openMockedSettings(page);
		await page.getByRole('tab', { name: 'Channels', exact: true }).click();
		await page.getByRole('button', { name: 'Connect channel' }).click();
		const dialog = page.getByRole('dialog', { name: 'Connect a channel' });
		await dialog.getByRole('combobox', { name: 'Channel' }).selectOption('whatsapp');
		await dialog.getByRole('button', { name: 'Continue' }).click();
		await expect(page.getByRole('dialog', { name: 'Connect WhatsApp' })).toBeVisible();
		await expect(page.getByAltText('QR code for WhatsApp connection')).toBeVisible();
		await page.getByRole('button', { name: 'Close channel dialog' }).click();
		await expect(page.getByText('WhatsApp', { exact: true })).toBeVisible();

		page.once('dialog', (dialog) => dialog.accept());
		await page.getByRole('button', { name: 'Disconnect' }).click();
		await expect(page.getByText('Channel disconnected.', { exact: true })).toBeVisible();
		await expect(page.getByText('No channels connected yet.', { exact: true })).toBeVisible();
	});

	test('a failed provider connection keeps the selected channel ready to retry', async ({ page }) => {
		await openMockedSettings(page);
		await page.getByRole('tab', { name: 'Channels', exact: true }).click();
		await page.route('**/api-gateway/bridge-connections', (route) => {
			if (route.request().method() === 'POST') {
				return route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'Bridge service is unavailable' }) });
			}
			return route.fallback();
		});
		await page.getByRole('button', { name: 'Connect channel' }).click();
		const dialog = page.getByRole('dialog', { name: 'Connect a channel' });
		await dialog.getByRole('combobox', { name: 'Channel' }).selectOption('telegram');
		await dialog.getByRole('button', { name: 'Continue' }).click();

		await expect(page.getByText('Bridge service is unavailable', { exact: true })).toBeVisible();
		await expect(dialog).toBeVisible();
		await expect(dialog.getByRole('combobox', { name: 'Channel' })).toHaveValue('telegram');
	});

	test('workspace type changes hide lead-pipeline controls when lead tracking is unavailable', async ({ page }) => {
		await openMockedSettings(page);
		await expect(page.getByRole('tab', { name: /Lead pipeline/ })).toBeVisible();
		await page.getByRole('radio', { name: /Chatbot only/ }).check();
		await expect(page.getByText('Workspace type updated.', { exact: true })).toBeVisible();
		await expect(page.getByRole('tab', { name: /Lead pipeline/ })).not.toBeVisible();
	});

	test('pipeline editor rejects duplicate stage keys before it can create an invalid pipeline', async ({ page }) => {
		await openMockedSettings(page);
		await page.getByRole('tab', { name: /Lead pipeline/ }).click();
		await page.getByPlaceholder('Stage key').fill('new');
		await page.getByRole('button', { name: 'Add stage' }).click();
		await expect(page.getByText('Stage keys must be unique.', { exact: true })).toBeVisible();
		await expect(page.getByRole('button', { name: 'Save pipeline' })).toBeVisible();
	});

	test('workspace deletion cannot be submitted until the exact workspace name is entered', async ({ page }) => {
		await openMockedSettings(page);
		await page.getByRole('button', { name: 'Delete' }).click();
		const dialog = page.getByRole('dialog', { name: 'Delete Workspace' });
		const confirmation = dialog.getByPlaceholder('Test Workspace');
		await expect(dialog.getByRole('button', { name: 'Delete permanently' })).toBeDisabled();
		await confirmation.fill('test workspace');
		await expect(dialog.getByRole('button', { name: 'Delete permanently' })).toBeDisabled();
		await confirmation.fill('Test Workspace');
		await expect(dialog.getByRole('button', { name: 'Delete permanently' })).toBeEnabled();
		await page.keyboard.press('Escape');
		await expect(dialog).not.toBeVisible();
	});

	test('the settings view remains operable without horizontal overflow on mobile', async ({ page }) => {
		await page.setViewportSize({ width: 375, height: 812 });
		await openMockedSettings(page);

		await page.getByRole('tab', { name: 'Business profile', exact: true }).click();
		await expect(page.getByLabel('Business category')).toBeVisible();
		const hasHorizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
		expect(hasHorizontalOverflow).toBe(false);
	});

	test('visible settings buttons always have an accessible name', async ({ page }) => {
		await mockWorkspaceApi(page);
		await page.goto('/inbox?tab=settings');
		await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible();

		const unnamedButtons = await page.locator('button').evaluateAll((buttons) =>
			buttons
				.filter((button) => {
					const name = button.getAttribute('aria-label') || button.getAttribute('aria-labelledby') || button.textContent?.trim();
					return !name;
				})
				.map((button) => button.outerHTML)
		);
		expect(unnamedButtons).toEqual([]);
	});
});
