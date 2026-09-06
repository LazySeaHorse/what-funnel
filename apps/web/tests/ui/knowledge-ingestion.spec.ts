import { expect, test } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

test('Knowledge tab uses the same reviewed ingestion contract as onboarding', async ({ page }) => {
	const api = await mockWorkspaceApi(page, { role: 'manager', productMode: 'full_workspace' });
	await page.goto('/inbox?tab=knowledge');
	await expect(page.getByRole('heading', { name: 'Knowledge base', exact: true })).toBeVisible();

	await page.getByPlaceholder(/Paste business information/).fill('Consulting costs $100 per hour.');
	await page.getByRole('button', { name: 'Extract with AI', exact: true }).click();
	await expect(page.getByText('Review structured knowledge', { exact: true })).toBeVisible();
	await expect(page.getByLabel('Concept title')).toHaveValue('Pricing');
	await expect(page.getByLabel('Canonical question')).toHaveValue('What does it cost?');

	await page.getByLabel('Canonical question').fill('How much does consulting cost?');
	await page.getByRole('button', { name: 'Add selected to Knowledge Base', exact: true }).click();
	await expect(page.getByText(/1 concept and 1 pattern added/)).toBeVisible();

	const create = api.requests.find((request) => request.path === '/api/kb/ingestions' && request.method === 'POST');
	const publish = api.requests.find((request) => request.path.endsWith('/publish'));
	expect(create?.body).toEqual({ raw_text: 'Consulting costs $100 per hour.' });
	expect(publish?.body).toMatchObject({
		concepts: [expect.objectContaining({ title: 'Pricing', approved: true })],
		patterns: [expect.objectContaining({ canonical_question: 'How much does consulting cost?', approved: true })]
	});
	expect(api.requests.some((request) => request.path === '/api/kb/compile-paste')).toBe(false);
});

test('Knowledge tab can purge all concepts and patterns in one guarded request', async ({ page }) => {
	const api = await mockWorkspaceApi(page, {
		knowledge: {
			concepts: [{ id: 'concept-1', type: 'faq', title: 'Old FAQ', body_text: 'Old answer', created_at: '2026-01-01T00:00:00Z' }],
			patterns: [{ id: 'pattern-1', canonical_question: 'Old question?', answer_text: 'Old answer', trigger_phrases: ['old question'] }]
		}
	});
	page.on('dialog', (dialog) => dialog.accept());

	await page.goto('/inbox?tab=knowledge');
	await expect(page.getByRole('button', { name: 'Purge knowledge base' })).toBeEnabled();
	await page.getByRole('button', { name: 'Purge knowledge base' }).click();

	await expect(page.getByText('Knowledge base purged — 1 concept and 1 pattern removed.')).toBeVisible();
	await expect(page.getByText('No knowledge concepts found')).toBeVisible();
	expect(api.requests.filter((request) => request.path === '/api/kb/purge')).toEqual([
		expect.objectContaining({ method: 'DELETE' })
	]);
});

test('Knowledge tab filters concepts dynamically from global search input', async ({ page }) => {
	await mockWorkspaceApi(page, {
		role: 'manager',
		productMode: 'full_workspace',
		knowledge: {
			concepts: [
				{ id: 'c-1', type: 'pricing', title: 'Consulting Rates', body_text: 'Standard rate is $150/hr', tags: ['pricing', 'rates'] },
				{ id: 'c-2', type: 'hours', title: 'Office Hours', body_text: 'Open Monday to Friday', tags: ['hours'] }
			],
			patterns: []
		}
	});

	await page.goto('/inbox?tab=knowledge');
	await expect(page.getByText('Consulting Rates')).toBeVisible();
	await expect(page.getByText('Office Hours')).toBeVisible();

	await page.getByPlaceholder('Search knowledge...').fill('Consulting');
	await expect(page.getByText('Consulting Rates')).toBeVisible();
	await expect(page.getByText('Office Hours')).not.toBeVisible();

	await page.getByPlaceholder('Search knowledge...').fill('Nonexistent topic');
	await expect(page.getByText('No matching knowledge concepts')).toBeVisible();
	await expect(page.getByText('Try adjusting your search terms')).toBeVisible();
});

test('Knowledge tab can discard a reviewed ingestion to return to paste state', async ({ page }) => {
	await mockWorkspaceApi(page, { role: 'manager', productMode: 'full_workspace' });
	await page.goto('/inbox?tab=knowledge');

	await page.getByPlaceholder(/Paste business information/).fill('Return policy requires 14 days.');
	await page.getByRole('button', { name: 'Extract with AI', exact: true }).click();
	await expect(page.getByText('Review structured knowledge', { exact: true })).toBeVisible();

	await page.getByRole('button', { name: 'Discard', exact: true }).click();
	await expect(page.getByText('Review structured knowledge', { exact: true })).not.toBeVisible();
	await expect(page.getByRole('button', { name: 'Extract with AI', exact: true })).toBeVisible();
});

test('Knowledge tab resumes the latest active ingestion through the shared workflow', async ({ page }) => {
	await mockWorkspaceApi(page, {
		activeIngestion: {
			id: '81111111-1111-4111-8111-111111111111',
			status: 'queued',
			concepts: [],
			patterns: []
		}
	});

	await page.goto('/inbox?tab=knowledge');
	await expect(page.getByText('Review structured knowledge', { exact: true })).toBeVisible();
	await expect(page.getByLabel('Concept title')).toHaveValue('Pricing');
	await expect(page.getByLabel('Canonical question')).toHaveValue('What does it cost?');
});
