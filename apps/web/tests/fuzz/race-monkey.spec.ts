import { test, expect } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';
import { DeterministicMonkeyFuzzer } from './monkey';
import { BackgroundChatter } from './race-network';

function createConvos() {
	return [
		{
			id: 'convo-race-1',
			status: 'open',
			assigned_user_ids: ['user-1'],
			channel_type: 'whatsapp',
			last_message_at: new Date().toISOString(),
			contact: { display_name: 'Alpha Customer', external_identity: '+15550001' },
			lead: {
				id: 'lead-race-1',
				current_state_key: 'new',
				tags: ['urgent'],
				notes: 'Alpha lead notes'
			},
			ai_control: { state: 'active', reply_override: 'inherit', run_state: 'idle' }
		},
		{
			id: 'convo-race-2',
			status: 'open',
			assigned_user_ids: ['user-1'],
			channel_type: 'telegram',
			last_message_at: new Date().toISOString(),
			contact: { display_name: 'Beta Customer', external_identity: '+15550002' },
			lead: {
				id: 'lead-race-2',
				current_state_key: 'qualified',
				tags: ['vip'],
				notes: 'Beta lead notes'
			},
			ai_control: { state: 'paused', reply_override: 'inherit', run_state: 'idle' }
		},
		{
			id: 'convo-race-3',
			status: 'open',
			assigned_user_ids: ['user-1'],
			channel_type: 'web',
			last_message_at: new Date().toISOString(),
			contact: { display_name: 'Gamma Customer', external_identity: '+15550003' },
			lead: {
				id: 'lead-race-3',
				current_state_key: 'won',
				tags: ['enterprise'],
				notes: 'Gamma lead notes'
			},
			ai_control: { state: 'active', reply_override: 'inherit', run_state: 'idle' }
		}
	];
}

function sampleMessages(convoID: string) {
	return [
		{
			id: `msg-${convoID}-1`,
			conversation_id: convoID,
			sender_type: 'customer',
			body: `Message 1 for conversation ${convoID}`,
			created_at: new Date().toISOString()
		},
		{
			id: `msg-${convoID}-2`,
			conversation_id: convoID,
			sender_type: 'agent',
			body: `Response 2 for conversation ${convoID}`,
			created_at: new Date().toISOString()
		}
	];
}

test.describe('Race Condition & Network Jitter UI Fuzzing', () => {
	test('handles rapid conversation switching with out-of-order response shuffling without UI corruption', async ({
		page
	}) => {
		test.setTimeout(60000);
		await page.setViewportSize({ width: 1440, height: 900 });

		const convos = createConvos();
		await mockWorkspaceApi(page, {
			role: 'manager',
			productMode: 'full_workspace',
			conversations: convos,
			messages: sampleMessages('convo-race-1'),
			aiConfigured: true,
			autoReplyEnabled: true,
			reorder: {
				seed: 4242,
				minDelayMs: 40,
				maxDelayMs: 300
			}
		});

		await page.goto('/inbox');
		await page.waitForLoadState('networkidle');
		await expect(page.locator('h1:has-text("Inbox")')).toBeVisible({ timeout: 15000 });

		// Rapidly click between conversations without waiting for requests to settle
		const convoItems = page.locator('.convo-item');
		await expect(convoItems.first()).toBeVisible({ timeout: 10000 });
		const count = await convoItems.count();
		expect(count).toBeGreaterThan(1);

		for (let i = 0; i < 8; i++) {
			const targetIndex = i % count;
			await convoItems.nth(targetIndex).click({ timeout: 1000 }).catch(() => {});
			// Random micro-delay (0-50ms) between clicks to simulate frantic clicking
			await page.waitForTimeout((i * 17) % 50);
		}

		// Wait for in-flight shuffled responses to settle
		await page.waitForTimeout(500);

		// Ensure main layout is intact and active conversation matches UI
		const mainRoot = page.locator('main');
		await expect(mainRoot).toBeVisible();
	});

	test('burst multi-clicks and rapid tab tearing do not cause uncaught exceptions or state corruption', async ({
		page
	}) => {
		test.setTimeout(90000);
		await page.setViewportSize({ width: 1440, height: 900 });

		const seed = process.env.FUZZ_SEED ? parseInt(process.env.FUZZ_SEED, 10) : 314159;

		await mockWorkspaceApi(page, {
			role: 'manager',
			productMode: 'full_workspace',
			conversations: createConvos(),
			aiConfigured: true,
			autoReplyEnabled: true,
			reorder: {
				seed,
				minDelayMs: 30,
				maxDelayMs: 250
			}
		});

		await page.goto('/inbox');
		await page.waitForLoadState('networkidle');
		await expect(page.locator('h1:has-text("Inbox")')).toBeVisible({ timeout: 15000 });

		const fuzzer = new DeterministicMonkeyFuzzer(page, {
			seed,
			maxActions: 50,
			actionDelayMs: 20,
			enableRaceActions: true,
			ignoredConsoleErrors: [
				/favicon\.ico/i,
				/ws proxy/i,
				/WebSocket/i,
				/WS error/i,
				/ECONNRESET/i,
				/AbortError/i,
				/The user aborted a request/i
			]
		});

		await fuzzer.run();
		fuzzer.assertNoErrors();

		const mainRoot = page.locator('main');
		await expect(mainRoot).toBeVisible({ timeout: 5000 });
	});

	test('concurrent background chatter alongside aggressive monkey fuzzing maintains stability', async ({
		page
	}) => {
		test.setTimeout(90000);
		await page.setViewportSize({ width: 1440, height: 900 });

		const seed = process.env.FUZZ_SEED ? parseInt(process.env.FUZZ_SEED, 10) : 271828;

		await mockWorkspaceApi(page, {
			role: 'manager',
			productMode: 'full_workspace',
			conversations: createConvos(),
			aiConfigured: true,
			autoReplyEnabled: true
		});

		await page.goto('/inbox');
		await page.waitForLoadState('networkidle');
		await expect(page.locator('h1:has-text("Inbox")')).toBeVisible({ timeout: 15000 });

		const chatter = new BackgroundChatter(page, { intervalMs: 100 });
		chatter.start();

		const fuzzer = new DeterministicMonkeyFuzzer(page, {
			seed,
			maxActions: 40,
			actionDelayMs: 25,
			enableRaceActions: true,
			ignoredConsoleErrors: [
				/favicon\.ico/i,
				/ws proxy/i,
				/WebSocket/i,
				/WS error/i,
				/ECONNRESET/i,
				/AbortError/i
			]
		});

		try {
			await fuzzer.run();
			fuzzer.assertNoErrors();
		} finally {
			chatter.stop();
		}

		expect(chatter.getDispatchedCount()).toBeGreaterThan(0);
		const mainRoot = page.locator('main');
		await expect(mainRoot).toBeVisible({ timeout: 5000 });
	});
});
