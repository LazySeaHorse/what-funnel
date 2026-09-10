import { expect, test } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

test('Knowledge tab allows manual editing of concepts', async ({ page }) => {
	const api = await mockWorkspaceApi(page, {
		role: 'manager',
		productMode: 'full_workspace',
		knowledge: {
			concepts: [
				{
					id: 'c-1',
					type: 'pricing',
					title: 'Consulting Rates',
					body_text: 'Standard rate is $150/hr',
					tags: ['pricing', 'rates'],
					source: 'owner_pasted',
					created_at: '2026-08-20T14:32:00Z'
				}
			],
			patterns: []
		}
	});

	await page.goto('/inbox?tab=knowledge');
	await expect(page.getByRole('heading', { name: 'Knowledge base', exact: true })).toBeVisible();
	await expect(page.getByText('Consulting Rates')).toBeVisible();
	await expect(page.getByText('Standard rate is $150/hr')).toBeVisible();

	// Click Edit button on the concept
	await page.getByTitle('Edit concept').click();
	await expect(page.getByText('Edit Concept')).toBeVisible();

	// Update fields
	const titleInput = page.locator('#edit-concept-title-c-1');
	await titleInput.fill('Updated Enterprise Rates');

	const bodyInput = page.locator('#edit-concept-body-c-1');
	await bodyInput.fill('Updated standard rate is $250/hr with SLA guarantee');

	// Add a tag
	const tagInput = page.locator('#edit-concept-tag-input-c-1');
	await tagInput.fill('enterprise');
	await page.getByRole('button', { name: 'Add', exact: true }).click();

	// Save changes
	await page.getByRole('button', { name: 'Save changes', exact: true }).click();

	// Verify updated content in view
	await expect(page.getByText('Updated Enterprise Rates')).toBeVisible();
	await expect(page.getByText('Updated standard rate is $250/hr with SLA guarantee')).toBeVisible();
	await expect(page.getByText('enterprise', { exact: true })).toBeVisible();

	// Verify PUT API call was made
	const updateReq = api.requests.find(
		(r) => r.path === '/api/kb/concepts/c-1' && r.method === 'PUT'
	);
	expect(updateReq).toBeDefined();
	expect(updateReq?.body).toMatchObject({
		title: 'Updated Enterprise Rates',
		body_text: 'Updated standard rate is $250/hr with SLA guarantee',
		tags: ['pricing', 'rates', 'enterprise']
	});
});

test('Knowledge tab allows manual editing of answer patterns', async ({ page }) => {
	const api = await mockWorkspaceApi(page, {
		role: 'manager',
		productMode: 'full_workspace',
		knowledge: {
			concepts: [],
			patterns: [
				{
					id: 'p-1',
					canonical_question: 'What are your rates?',
					answer_text: 'Our standard rate is $150/hr.',
					trigger_phrases: ['rates', 'pricing'],
					created_at: '2026-08-20T14:32:00Z'
				}
			]
		}
	});

	await page.goto('/inbox?tab=knowledge');
	await expect(page.getByRole('heading', { name: 'Knowledge base', exact: true })).toBeVisible();

	// Switch to patterns tab
	await page.getByRole('button', { name: /Patterns/ }).click();
	await expect(page.getByText('What are your rates?')).toBeVisible();
	await expect(page.getByText('Our standard rate is $150/hr.')).toBeVisible();

	// Click Edit button on the pattern
	await page.getByTitle('Edit pattern').click();
	await expect(page.getByText('Edit Answer Pattern')).toBeVisible();

	// Update question and answer
	const questionInput = page.locator('#edit-pattern-question-p-1');
	await questionInput.fill('What are your consulting packages?');

	const answerInput = page.locator('#edit-pattern-answer-p-1');
	await answerInput.fill('Consulting packages start at $1,500/mo.');

	// Add trigger phrase
	const triggerInput = page.locator('#edit-pattern-triggers-input-p-1');
	await triggerInput.fill('consulting fees');
	await page.getByRole('button', { name: 'Add', exact: true }).click();

	// Save changes
	await page.getByRole('button', { name: 'Save changes', exact: true }).click();

	// Verify updated content in view
	await expect(page.getByText('What are your consulting packages?')).toBeVisible();
	await expect(page.getByText('Consulting packages start at $1,500/mo.')).toBeVisible();
	await expect(page.getByText('consulting fees')).toBeVisible();

	// Verify PUT API call was made
	const updateReq = api.requests.find(
		(r) => r.path === '/api/kb/patterns/p-1' && r.method === 'PUT'
	);
	expect(updateReq).toBeDefined();
	expect(updateReq?.body).toMatchObject({
		canonical_question: 'What are your consulting packages?',
		answer_text: 'Consulting packages start at $1,500/mo.',
		trigger_phrases: ['rates', 'pricing', 'consulting fees']
	});
});

test('capture redesigned knowledge tab screenshots', async ({ page }) => {
	await mockWorkspaceApi(page, {
		role: 'manager',
		productMode: 'full_workspace',
		aiConfigured: true,
		autoReplyEnabled: true,
		knowledge: {
			concepts: [
				{
					id: 'c-1',
					type: 'pricing',
					title: 'Consulting & Implementation Rates',
					body_text: 'Standard rate is $150/hr for ad-hoc consultation. Enterprise onboarding packages start at $2,500 including 3 weeks of custom integration and priority SLA support.',
					tags: ['pricing', 'enterprise', 'onboarding'],
					source: 'owner_pasted',
					created_at: '2026-08-20T14:32:00Z'
				},
				{
					id: 'c-2',
					type: 'hours',
					title: 'Studio Operating Hours & Holiday Schedule',
					body_text: 'Monday through Friday, 9:00 AM to 6:00 PM EST. Urgent incident support is available 24/7 for Enterprise tier clients via dedicated PagerDuty bridge.',
					tags: ['hours', 'support', 'sla'],
					source: 'system',
					created_at: '2026-08-15T09:00:00Z'
				},
				{
					id: 'c-3',
					type: 'policy',
					title: 'Refund and Cancellation Policy',
					body_text: 'Subscriptions may be cancelled anytime before the next billing cycle. Prorated refunds are issued within 14 business days upon verified service disruption.',
					tags: ['policy', 'billing', 'refunds'],
					source: 'owner_pasted',
					created_at: '2026-08-10T11:20:00Z'
				}
			],
			patterns: [
				{
					id: 'p-1',
					canonical_question: 'What are your pricing packages?',
					answer_text: 'Our standard consulting rate is $150/hr. Enterprise onboarding starts at $2,500. Would you like a breakdown of our retainer tiers?',
					trigger_phrases: ['how much does it cost', 'pricing plans', 'rates', 'what do you charge'],
					created_at: '2026-08-20T14:35:00Z'
				},
				{
					id: 'p-2',
					canonical_question: 'What are your support hours?',
					answer_text: 'We are open Monday to Friday from 9:00 AM to 6:00 PM EST. Enterprise clients have 24/7 emergency coverage.',
					trigger_phrases: ['when are you open', 'business hours', 'operating hours', 'are you available'],
					created_at: '2026-08-15T09:15:00Z'
				}
			]
		}
	});

	await page.setViewportSize({ width: 1440, height: 900 });
	await page.goto('/inbox?tab=knowledge');
	await expect(page.getByRole('heading', { name: 'Knowledge base', exact: true })).toBeVisible();

	await page.screenshot({ path: 'test-results/redesigned-knowledge-concepts.png', fullPage: false });

	await page.getByRole('button', { name: /Patterns/ }).click();
	await page.screenshot({ path: 'test-results/redesigned-knowledge-patterns.png', fullPage: false });

	// Mobile view
	await page.setViewportSize({ width: 375, height: 812 });
	await page.getByRole('button', { name: /KB Concepts/ }).click();
	await page.screenshot({ path: 'test-results/redesigned-knowledge-mobile.png', fullPage: false });
});
