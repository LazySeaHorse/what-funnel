import { test, expect, type Page } from '@playwright/test';
import { mockIdleWebSocket, mockWorkspaceApi } from '../support/mock-api';
import { DeterministicMonkeyFuzzer, resolveFuzzSeed } from './monkey';
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

// Message shape matches the real API / app types (see monkey-mock.spec.ts):
// text lives in content.text and direction is inbound/outbound.
function sampleMessages(convo: { id: string; contact: { display_name: string } }) {
	return [
		{
			id: `msg-${convo.id}-1`,
			conversation_id: convo.id,
			content_type: 'text',
			content: { text: `Question from ${convo.contact.display_name} (${convo.id})` },
			direction: 'inbound',
			sender_type: 'contact',
			created_at: new Date(Date.now() - 120000).toISOString()
		},
		{
			id: `msg-${convo.id}-2`,
			conversation_id: convo.id,
			content_type: 'text',
			content: { text: `Answer to ${convo.contact.display_name} (${convo.id})` },
			direction: 'outbound',
			sender_type: 'agent',
			created_at: new Date(Date.now() - 60000).toISOString()
		}
	];
}

function messagesByConversation(convos: ReturnType<typeof createConvos>) {
	return Object.fromEntries(convos.map((c) => [c.id, sampleMessages(c)]));
}

/** Uncaught page exceptions fail the test (collected, then asserted at the end). */
function collectPageErrors(page: Page): string[] {
	const errors: string[] = [];
	page.on('pageerror', (err) => errors.push(err.message));
	return errors;
}

/**
 * Invariant: whichever conversation the chat header shows, the thread contains
 * exactly that conversation's messages, each rendered once and none from other
 * conversations (guards against out-of-order responses corrupting the thread).
 * Returns the id of the displayed conversation, or null when no chat is open.
 */
async function expectThreadMatchesHeader(
	page: Page,
	convos: ReturnType<typeof createConvos>,
	expectedId?: string
): Promise<string | null> {
	let shown: string | null = null;
	await expect
		.poll(
			async () => {
				const matches: string[] = [];
				for (const c of convos) {
					const n = await page.getByRole('heading', { name: c.contact.display_name, exact: true }).count();
					if (n > 0) matches.push(c.id);
				}
				if (matches.length > 1) return `multiple headers: ${matches.join(',')}`;
				shown = matches[0] ?? null;
				// The app keeps the previous conversation rendered until the newly selected one has loaded.
				if (expectedId && shown !== expectedId) return `waiting for ${expectedId}, showing ${shown}`;
				if (!shown) return 'ok';
				for (const c of convos) {
					for (const m of sampleMessages(c)) {
						const count = await page.getByText(m.content.text, { exact: true }).count();
						const expected = c.id === shown ? 1 : 0;
						if (count !== expected) {
							return `message "${m.content.text}" rendered ${count}x, expected ${expected}x while ${shown} is open`;
						}
					}
				}
				return 'ok';
			},
			{ timeout: 10000, message: 'thread must match the open conversation' }
		)
		.toBe('ok');
	return shown;
}

test.describe('Race Condition & Network Jitter UI Fuzzing', () => {
	test('handles rapid conversation switching with out-of-order response shuffling without UI corruption', async ({
		page
	}) => {
		test.setTimeout(60000);
		await page.setViewportSize({ width: 1440, height: 900 });

		const convos = createConvos();
		const pageErrors = collectPageErrors(page);
		await mockIdleWebSocket(page);
		await mockWorkspaceApi(page, {
			role: 'manager',
			productMode: 'full_workspace',
			conversations: convos,
			messagesByConversation: messagesByConversation(convos),
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

		let lastIndex = 0;
		for (let i = 0; i < 8; i++) {
			lastIndex = i % count;
			await convoItems.nth(lastIndex).click({ timeout: 2000 });
			// Deterministic micro-delay (0-50ms) between clicks to simulate frantic clicking
			await page.waitForTimeout((i * 17) % 50);
		}

		// After the shuffled responses settle, the open conversation must be the LAST one
		// clicked (not whichever response arrived last) and its thread must be intact.
		await expectThreadMatchesHeader(page, convos, convos[lastIndex].id);

		await expect(page.locator('main')).toBeVisible();
		expect(pageErrors, 'uncaught page errors').toEqual([]);
	});

	test('burst multi-clicks and rapid tab tearing do not cause uncaught exceptions or state corruption', async ({
		page
	}) => {
		test.setTimeout(90000);
		await page.setViewportSize({ width: 1440, height: 900 });

		const seed = resolveFuzzSeed(314159);
		const convos = createConvos();

		await mockIdleWebSocket(page);
		await mockWorkspaceApi(page, {
			role: 'manager',
			productMode: 'full_workspace',
			conversations: convos,
			messagesByConversation: messagesByConversation(convos),
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
			// Reordered/aborted in-flight requests legitimately log aborts.
			ignoredConsoleErrors: [/AbortError/i, /The user aborted a request/i]
		});

		await fuzzer.run();
		fuzzer.assertNoErrors();

		await expect(page.locator('main')).toBeVisible({ timeout: 5000 });
		await expectThreadMatchesHeader(page, convos);
	});

	test('concurrent background chatter alongside aggressive monkey fuzzing maintains stability', async ({
		page
	}) => {
		test.setTimeout(90000);
		await page.setViewportSize({ width: 1440, height: 900 });

		const seed = resolveFuzzSeed(271828);
		const convos = createConvos();

		await mockIdleWebSocket(page);
		await mockWorkspaceApi(page, {
			role: 'manager',
			productMode: 'full_workspace',
			conversations: convos,
			messagesByConversation: messagesByConversation(convos),
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
			ignoredConsoleErrors: [/AbortError/i]
		});

		try {
			await fuzzer.run();
			fuzzer.assertNoErrors();
		} finally {
			chatter.stop();
		}

		expect(chatter.getDispatchedCount()).toBeGreaterThan(0);
		await expect(page.locator('main')).toBeVisible({ timeout: 5000 });
		await expectThreadMatchesHeader(page, convos);
	});
});
