import { expect, test } from '@playwright/test';

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
	});
}
