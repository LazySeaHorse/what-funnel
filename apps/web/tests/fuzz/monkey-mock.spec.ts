import { test, expect } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';
import { DeterministicMonkeyFuzzer } from './monkey';

function sampleConversations() {
	return [
		{
			id: 'convo-1',
			status: 'open',
			assigned_user_ids: ['user-1'],
			channel_type: 'whatsapp',
			last_message_at: new Date().toISOString(),
			contact: { display_name: 'Alice Cooper', external_identity: '+15550101' },
			lead: {
				id: 'lead-1',
				current_state_key: 'new',
				tags: ['vip', 'urgent'],
				notes: 'Interested in enterprise package',
				estimated_value_cents: 50000
			},
			ai_control: { state: 'active', reply_override: 'inherit', run_state: 'idle' }
		},
		{
			id: 'convo-2',
			status: 'open',
			assigned_user_ids: [],
			channel_type: 'telegram',
			last_message_at: new Date(Date.now() - 3600000).toISOString(),
			contact: { display_name: 'Bob Marley', external_identity: '@bob_tele' },
			lead: {
				id: 'lead-2',
				current_state_key: 'qualified',
				tags: ['callback'],
				notes: 'Requested weekend demo',
				estimated_value_cents: 120000
			},
			ai_control: { state: 'paused', reply_override: 'human_only', run_state: 'idle' }
		},
		{
			id: 'convo-3',
			status: 'closed',
			assigned_user_ids: ['user-1'],
			channel_type: 'whatsapp',
			last_message_at: new Date(Date.now() - 86400000).toISOString(),
			contact: { display_name: 'Charlie Brown', external_identity: '+15550103' },
			lead: {
				id: 'lead-3',
				current_state_key: 'won',
				tags: ['onboarding-done'],
				notes: 'Closed deal',
				estimated_value_cents: 250000
			},
			ai_control: { state: 'active', reply_override: 'inherit', run_state: 'idle' }
		}
	];
}

function sampleMessages() {
	return [
		{
			id: 'msg-1',
			conversation_id: 'convo-1',
			content_type: 'text',
			content: { text: 'Hello! I need assistance with our onboarding.' },
			direction: 'inbound',
			sender_type: 'contact',
			created_at: new Date(Date.now() - 600000).toISOString()
		},
		{
			id: 'msg-2',
			conversation_id: 'convo-1',
			content_type: 'text',
			content: { text: 'Hi Alice, how can we help you today?' },
			direction: 'outbound',
			sender_type: 'agent',
			created_at: new Date(Date.now() - 300000).toISOString()
		}
	];
}

function sampleKnowledge() {
	return {
		concepts: [
			{ id: 'concept-1', title: 'Business Hours', content: 'We are open Monday to Friday, 9am to 6pm UTC.' },
			{ id: 'concept-2', title: 'Pricing Tiers', content: 'Starter: $49/mo, Pro: $199/mo, Enterprise: Custom quote.' }
		],
		patterns: [
			{ id: 'pattern-1', name: 'Greeting', match_phrases: ['hello', 'hi', 'hey'], response: 'Welcome to WhatFunnel!' }
		]
	};
}

test.describe('UI Monkey Fuzzing (Deterministic)', () => {
	test.beforeEach(async ({ page }) => {
		await page.setViewportSize({ width: 1440, height: 900 });
		await mockWorkspaceApi(page, {
			role: 'manager',
			productMode: 'full_workspace',
			conversations: sampleConversations(),
			messages: sampleMessages(),
			knowledge: sampleKnowledge(),
			aiConfigured: true,
			autoReplyEnabled: true
		});
	});

	test('fuzzing inbox & messaging workspace for unhandled crashes', async ({ page }) => {
		await page.goto('/inbox');
		await page.waitForLoadState('networkidle');

		// Verify inbox loaded
		await expect(page.locator('h1:has-text("Inbox")')).toBeVisible({ timeout: 10000 });

		const fuzzer = new DeterministicMonkeyFuzzer(page, {
			maxActions: 60,
			actionDelayMs: 30
		});

		await fuzzer.run();
		fuzzer.assertNoErrors();
	});

	test('fuzzing leads CRM dashboard & drawer for reactive state bugs', async ({ page }) => {
		await page.goto('/inbox?tab=leads');
		await page.waitForLoadState('networkidle');

		await expect(page.locator('h1:has-text("Leads")')).toBeVisible({ timeout: 10000 });

		const fuzzer = new DeterministicMonkeyFuzzer(page, {
			maxActions: 60,
			actionDelayMs: 30
		});

		await fuzzer.run();
		fuzzer.assertNoErrors();
	});

	test('fuzzing knowledge base studio with rapid inputs and cards', async ({ page }) => {
		await page.goto('/inbox?tab=knowledge');
		await page.waitForLoadState('networkidle');

		await expect(page.locator('h1:has-text("Knowledge")')).toBeVisible({ timeout: 10000 });

		const fuzzer = new DeterministicMonkeyFuzzer(page, {
			maxActions: 60,
			actionDelayMs: 30
		});

		await fuzzer.run();
		fuzzer.assertNoErrors();
	});

	test('global cross-section monkey chaos across full dashboard', async ({ page }) => {
		await page.goto('/inbox');
		await page.waitForLoadState('networkidle');

		const fuzzer = new DeterministicMonkeyFuzzer(page, {
			maxActions: 80,
			actionDelayMs: 25
		});

		await fuzzer.run();
		fuzzer.assertNoErrors();
	});
});
