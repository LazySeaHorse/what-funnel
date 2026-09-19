import type { Page } from '@playwright/test';

export interface BackgroundChatterOptions {
	intervalMs?: number;
	conversationIDs?: string[];
}

/**
 * Background Chatter Generator:
 * Dispatches simulated inbound provider events into the client window
 * concurrently while the monkey fuzzer or user is typing, clicking, and navigating.
 */
export class BackgroundChatter {
	private page: Page;
	private intervalMs: number;
	private running = false;
	private intervalTimer: NodeJS.Timeout | null = null;
	private eventCount = 0;

	constructor(page: Page, options: BackgroundChatterOptions = {}) {
		this.page = page;
		this.intervalMs = options.intervalMs ?? 200;
	}

	start(): void {
		if (this.running) return;
		this.running = true;

		const dispatch = async () => {
			if (!this.running) return;
			this.eventCount++;
			try {
				await this.page.evaluate((count) => {
					window.dispatchEvent(
						new CustomEvent('dev-message-sent', {
							detail: {
								id: `sim-inbound-${count}`,
								content: `Simulated inbound chatter #${count}`,
								sender: 'Alice Cooper',
								timestamp: Date.now()
							}
						})
					);
				}, this.eventCount);
			} catch {
				// Page may be navigating or closed
			}

			if (this.running) {
				this.intervalTimer = setTimeout(dispatch, this.intervalMs);
			}
		};

		this.intervalTimer = setTimeout(dispatch, this.intervalMs);
	}

	stop(): void {
		this.running = false;
		if (this.intervalTimer) {
			clearTimeout(this.intervalTimer);
			this.intervalTimer = null;
		}
	}

	getDispatchedCount(): number {
		return this.eventCount;
	}
}

/**
 * Executes two actions concurrently (without awaiting the first before launching the second)
 * to stress test mutual exclusion, optimistic state, and abort controllers.
 */
export async function runConcurrentActions(
	action1: () => Promise<void>,
	action2: () => Promise<void>
): Promise<void> {
	await Promise.allSettled([action1(), action2()]);
}
