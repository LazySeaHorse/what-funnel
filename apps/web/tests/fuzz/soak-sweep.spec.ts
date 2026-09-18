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
				tags: ['callback', 'demo'],
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
				notes: 'Closed deal successfully',
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
			content: { text: 'Hello! Can I get more details about your pricing?' },
			direction: 'inbound',
			sender_type: 'contact',
			created_at: new Date(Date.now() - 600000).toISOString()
		},
		{
			id: 'msg-2',
			conversation_id: 'convo-1',
			content_type: 'text',
			content: { text: 'Sure thing Alice! We have Starter and Enterprise plans.' },
			direction: 'outbound',
			sender_type: 'agent',
			created_at: new Date(Date.now() - 300000).toISOString()
		}
	];
}

function sampleKnowledge() {
	return {
		concepts: [
			{ id: 'concept-1', title: 'Business Hours', content: 'Monday to Friday, 9am to 6pm UTC.' },
			{ id: 'concept-2', title: 'Pricing Tiers', content: 'Starter: $49/mo, Pro: $199/mo, Enterprise: Custom quote.' },
			{ id: 'concept-3', title: 'Refund Policy', content: 'Full refund within 30 days of purchase.' }
		],
		patterns: [
			{ id: 'pattern-1', name: 'Greeting', match_phrases: ['hello', 'hi', 'hey'], response: 'Welcome to WhatFunnel!' },
			{ id: 'pattern-2', name: 'Pricing Request', match_phrases: ['how much', 'cost', 'pricing'], response: 'Our plans start at $49/mo.' }
		]
	};
}

const DEFAULT_SEEDS = [104729, 224737, 349281, 481920, 592810];
const SEEDS_TO_RUN: number[] = process.env.SOAK_SEEDS
	? process.env.SOAK_SEEDS.split(',').map((s) => parseInt(s.trim(), 10))
	: DEFAULT_SEEDS;

const ACTIONS_COUNT = process.env.SOAK_ACTIONS
	? parseInt(process.env.SOAK_ACTIONS, 10)
	: 100;

test.describe('Multi-Seed Soak Sweep Fuzzing', () => {
	for (const seed of SEEDS_TO_RUN) {
		test(`soak run with seed ${seed} (${ACTIONS_COUNT} actions deep)`, async ({ page }) => {
			test.setTimeout(90000);
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

			await page.goto('/inbox');
			await page.waitForLoadState('networkidle');
			await expect(page.locator('h1:has-text("Inbox")')).toBeVisible({ timeout: 15000 });

			const fuzzer = new DeterministicMonkeyFuzzer(page, {
				seed,
				maxActions: ACTIONS_COUNT,
				actionDelayMs: 25
			});

			await fuzzer.run();
			fuzzer.assertNoErrors();
		});
	}
});
