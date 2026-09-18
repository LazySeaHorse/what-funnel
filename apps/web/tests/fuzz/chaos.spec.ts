import { test, expect } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';
import { DeterministicMonkeyFuzzer } from './monkey';

function sampleConversations() {
	return [
		{
			id: 'convo-chaos-1',
			status: 'open',
			assigned_user_ids: ['user-1'],
			channel_type: 'whatsapp',
			last_message_at: new Date().toISOString(),
			contact: { display_name: 'Chaos Tester', external_identity: '+15550999' },
			lead: {
				id: 'lead-chaos-1',
				current_state_key: 'new',
				tags: ['chaos'],
				notes: 'Chaos test lead'
			},
			ai_control: { state: 'active', reply_override: 'inherit', run_state: 'idle' }
		}
	];
}

test.describe('Network Chaos UI Fuzzing', () => {
	test('UI remains resilient and un-crashed under intermittent 500s, latency spikes, and network drops', async ({
		page
	}) => {
		test.setTimeout(90000);
		await page.setViewportSize({ width: 1440, height: 900 });

		const seed = process.env.FUZZ_SEED ? parseInt(process.env.FUZZ_SEED, 10) : 777123;

		// 1. Mock API with built-in chaos fault injection
		await mockWorkspaceApi(page, {
			role: 'manager',
			productMode: 'full_workspace',
			conversations: sampleConversations(),
			aiConfigured: true,
			autoReplyEnabled: true,
			chaos: {
				seed,
				errorRate: 0.15,
				delayRate: 0.25,
				abortRate: 0.05
			}
		});

		await page.goto('/inbox');
		await page.waitForLoadState('networkidle');
		await expect(page.locator('h1:has-text("Inbox")')).toBeVisible({ timeout: 15000 });

		// 2. Run monkey fuzzer with expected network failure logs filtered out
		const fuzzer = new DeterministicMonkeyFuzzer(page, {
			seed,
			maxActions: 60,
			actionDelayMs: 25,
			ignoredConsoleErrors: [
				/favicon\.ico/i,
				/ws proxy/i,
				/WebSocket/i,
				/WS error/i,
				/ECONNRESET/i,
				/Failed to load resource/i,
				/net::ERR_FAILED/i,
				/Chaos fault injected/i,
				/Failed to fetch/i,
				/ApiError/i,
				/Load failed/i,
				/Service unavailable/i
			]
		});

		await fuzzer.run();
		fuzzer.assertNoErrors();

		// 3. Ensure UI root didn't crash into a white screen or root unhandled error page
		const mainRoot = page.locator('main');
		await expect(mainRoot).toBeVisible({ timeout: 5000 });
	});
});
