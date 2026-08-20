import { expect, test } from '@playwright/test';

test.describe('404 Not Found Page', () => {
	test('renders 404 page for unknown routes with mock styling and brand accent', async ({ page }) => {
		await page.setViewportSize({ width: 1440, height: 900 });
		await page.goto('/unknown-random-route-404');
		
		// Check headline elements
		await expect(page.getByRole('heading', { level: 1 })).toContainText('Oops! This page');
		await expect(page.getByRole('heading', { level: 1 })).toContainText('wandered off somewhere');

		// Check subtext
		await expect(page.getByText("We can't find the page you're looking for.")).toBeVisible();

		// Check action buttons
		const homeBtn = page.getByRole('link', { name: /Go back home/i });
		await expect(homeBtn).toBeVisible();
		await expect(homeBtn).toHaveAttribute('href', '/');

		const inboxBtn = page.getByRole('link', { name: /Go to inbox/i });
		await expect(inboxBtn).toBeVisible();
		await expect(inboxBtn).toHaveAttribute('href', '/inbox');

		// Check 404 hero image
		const heroImg = page.getByRole('img', { name: /404 Page Not Found/i });
		await expect(heroImg).toBeVisible();
	});

	test('renders directly at /404 route', async ({ page }) => {
		await page.goto('/404');
		await expect(page.getByRole('heading', { level: 1 })).toContainText('Oops! This page');
		await expect(page.getByRole('link', { name: /Go back home/i })).toBeVisible();
	});
});
