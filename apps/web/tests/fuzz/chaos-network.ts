import type { Page } from '@playwright/test';
import { Mulberry32 } from './monkey';

export interface ChaosConfig {
	seed: number;
	errorRate?: number; // probability of HTTP 5xx / 4xx error (default: 0.12)
	delayRate?: number; // probability of high latency spike (default: 0.20)
	minDelayMs?: number;
	maxDelayMs?: number;
	abortRate?: number; // probability of connection abort (default: 0.04)
}

/**
 * Injects network faults into API requests:
 * - HTTP 500, 502, 503, 429 responses
 * - High latency spikes (jitter between minDelayMs and maxDelayMs)
 * - Aborted / failed TCP connections
 */
export async function injectNetworkChaos(page: Page, config: ChaosConfig): Promise<void> {
	const rng = new Mulberry32(config.seed);
	const errorRate = config.errorRate ?? 0.12;
	const delayRate = config.delayRate ?? 0.20;
	const abortRate = config.abortRate ?? 0.04;
	const minDelay = config.minDelayMs ?? 200;
	const maxDelay = config.maxDelayMs ?? 1000;

	await page.route('**/api-gateway/**', async (route) => {
		const roll = rng.next();
		const url = route.request().url();

		// Always allow initial auth & account bootstrap so the UI mounts cleanly
		if (url.includes('/auth/me') || url.includes('/workspace/account') || url.includes('/auth/csrf')) {
			return route.continue();
		}

		if (roll < abortRate) {
			// Abort connection to simulate network drop
			return route.abort('failed').catch(() => {});
		}

		if (roll < abortRate + errorRate) {
			// Return a server error
			const statuses = [500, 502, 503, 429];
			const status = rng.pick(statuses);
			return route.fulfill({
				status,
				contentType: 'application/json',
				body: JSON.stringify({
					error: `Chaos fault injected: status ${status}`,
					code: 'CHAOS_INJECTED'
				})
			});
		}

		if (roll < abortRate + errorRate + delayRate) {
			// Simulate high network latency
			const delay = rng.int(minDelay, maxDelay);
			await new Promise((resolve) => setTimeout(resolve, delay));
		}

		return route.continue();
	});
}
