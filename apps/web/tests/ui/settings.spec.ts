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
			['AI provider', 'AI provider'],
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

	test('a manager can save a BYOK provider configuration without the key being displayed again', async ({ page }) => {
		await openMockedSettings(page);
		await page.getByRole('tab', { name: 'AI provider', exact: true }).click();

		await expect(page.getByText('Not configured', { exact: true })).toBeVisible();
		await page.getByLabel('API key', { exact: true }).fill('sk-test-private-key');
		await page.getByLabel('OpenAI-compatible base URL').fill('https://provider.example.test/v1/');
		await page.getByLabel('Completion model').fill('provider-chat-model');
		await page.getByLabel('Embedding model').fill('provider-embedding-model');
		await page.getByRole('button', { name: 'Save provider' }).click();

		await expect(page.getByText('AI provider configuration saved.', { exact: true })).toBeVisible();
		await expect(page.getByText('Configured', { exact: true })).toBeVisible();
		await expect(page.getByLabel('New API key', { exact: true })).toHaveValue('');
		await expect(page.getByText('sk-test-private-key')).not.toBeVisible();
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
		await expect(page.getByLabel('Default time zone')).toHaveValue('UTC');
	});

	test('a failed save preserves edits and gives the user a recoverable error', async ({ page }) => {
		await openMockedSettings(page);
		await page.route('**/api-gateway/workspace/account/settings', (route) =>
			route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'Settings service is unavailable' }) })
		);

		await page.getByLabel('Default time zone').selectOption('(GMT+00:00) UTC');
		await page.getByRole('button', { name: 'Save changes' }).click();

		await expect(page.getByText('Settings service is unavailable', { exact: true })).toBeVisible();
		await expect(page.getByLabel('Default time zone')).toHaveValue('UTC');
	});

	test('add user modal can be dismissed by keyboard without changing workspace state', async ({ page }) => {
		await openMockedSettings(page);
		await page.getByRole('tab', { name: 'Users & permissions', exact: true }).click();
		await page.getByRole('button', { name: 'Add user' }).click();

		const dialog = page.getByRole('dialog', { name: 'Add Team Member' });
		await expect(dialog).toBeVisible();
		await page.getByLabel('Username').fill('newagent');
		await page.keyboard.press('Escape');
		await expect(dialog).not.toBeVisible();
	});

	test('user dialogs complete add, credential, reset, and delete workflows', async ({ page }) => {
		const api = await mockWorkspaceApi(page);
		await page.goto('/inbox?tab=settings');
		await page.getByRole('tab', { name: 'Users & permissions', exact: true }).click();

		await page.getByRole('button', { name: 'Add user' }).click();
		const addDialog = page.getByRole('dialog', { name: 'Add Team Member' });
		await addDialog.getByLabel('Username').fill('newagent');
		await addDialog.getByRole('button', { name: 'Add user' }).click();

		const credentialsDialog = page.getByRole('dialog', { name: 'User credentials' });
		await expect(credentialsDialog).toContainText('test-slug-newagent');
		await credentialsDialog.getByRole('button', { name: 'Done' }).click();
		const userRow = page.getByText('newagent', { exact: true }).locator('..').locator('..').locator('..');
		await userRow.getByTitle('Reset Password').click();

		const resetDialog = page.getByRole('dialog', { name: 'Reset User Password' });
		await resetDialog.getByLabel('New Password').fill('Replacement99!');
		await resetDialog.getByRole('button', { name: 'Set Password' }).click();
		await expect(credentialsDialog).toContainText('Replacement99!');
		await credentialsDialog.getByRole('button', { name: 'Done' }).click();

		await userRow.getByTitle('Delete user').click();
		const deleteDialog = page.getByRole('dialog', { name: 'Delete User' });
		await deleteDialog.getByRole('button', { name: 'Delete User' }).click();
		await expect(deleteDialog).not.toBeVisible();
		await expect(page.locator('.font-medium.text-slate-800', { hasText: 'newagent' })).not.toBeVisible();

		expect(api.requests).toEqual(expect.arrayContaining([
			expect.objectContaining({ path: '/workspace/users', method: 'POST' }),
			expect.objectContaining({ path: '/workspace/users/user-2/password', method: 'PUT' }),
			expect.objectContaining({ path: '/workspace/users/user-2', method: 'DELETE' })
		]));
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
		await expect(page.getByRole('button', { name: 'Leads', exact: true })).not.toBeVisible();
		await expect(page.getByRole('button', { name: 'Contacts', exact: true })).not.toBeVisible();
		await expect(page.getByTestId('operator-identity')).not.toBeVisible();
	});

	test('pipeline editor generates unique keys for duplicate stage labels', async ({ page }) => {
		const api = await mockWorkspaceApi(page);
		await page.goto('/inbox?tab=settings');
		await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible();
		await page.getByRole('tab', { name: /Lead pipeline/ }).click();
		await page.getByRole('button', { name: 'Add another stage' }).click();
		await page.getByLabel('Stage label').nth(1).fill('New lead');
		await page.getByRole('button', { name: 'Save pipeline' }).click();
		await expect(page.getByText('Pipeline saved.', { exact: true })).toBeVisible();
		const save = api.requests.find((request) => request.path === '/workspace/pipelines/pipeline-1' && request.method === 'PUT');
		expect(save?.body?.states).toEqual([
			expect.objectContaining({ key: 'new_lead', label: 'New lead' }),
			expect.objectContaining({ key: 'new_lead_2', label: 'New lead' })
		]);
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
