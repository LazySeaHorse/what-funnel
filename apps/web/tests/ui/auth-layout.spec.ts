import { expect, test } from '@playwright/test';

test('serves a loading screen before the client application boots', async ({ request }) => {
	const response = await request.get('/login');
	const html = await response.text();

	expect(html).toContain('id="wf-boot"');
	expect(html).toContain('Starting What Funnel…');
});

for (const authPage of [
	{ path: '/login', formHeading: 'Sign in' },
	{ path: '/signup', formHeading: 'Create workspace' }
]) {
	test(`${authPage.path} renders inside the shared auth route layout`, async ({ page }) => {
		await page.goto(authPage.path);

		await expect(page).toHaveURL(new RegExp(`${authPage.path}$`));
		await expect(page.getByRole('heading', { name: /All conversations/ })).toBeVisible();
		await expect(page.getByRole('heading', { name: authPage.formHeading, exact: true })).toBeVisible();
		await expect(page.getByAltText('What Funnel dashboard illustration').first()).toBeVisible();
		await expect(page.getByRole('progressbar', { name: 'Loading page' })).toHaveCount(0);
	});
}

test('keeps sign-in and workspace loading feedback visible across a slow handoff', async ({ page }) => {
	let releaseLogin!: () => void;
	const loginGate = new Promise<void>((resolve) => (releaseLogin = resolve));
	let releaseSession!: () => void;
	const sessionGate = new Promise<void>((resolve) => (releaseSession = resolve));

	await page.route('**/api-gateway/auth/login', async (route) => {
		await loginGate;
		await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ status: 'ok' }) });
	});
	await page.route('**/api-gateway/auth/me', async (route) => {
		await sessionGate;
		await route.fulfill({
			status: 401,
			contentType: 'application/json',
			body: JSON.stringify({ error: 'Session unavailable in test' })
		});
	});

	await page.goto('/login');
	await page.getByLabel('Email or username').fill('manager@example.test');
	await page.getByLabel('Password').fill('password');
	await page.getByRole('button', { name: 'Sign in', exact: true }).click();

	await expect(page.getByRole('button', { name: 'Signing in...' })).toBeDisabled();
	releaseLogin();

	await expect(page).toHaveURL(/\/inbox$/);
	await expect(page.getByRole('status').filter({ hasText: 'Loading your workspace…' })).toBeVisible();
	releaseSession();
	await expect(page).toHaveURL(/\/login$/);
});
