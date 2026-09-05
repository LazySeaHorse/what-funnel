import { expect, test } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

test.describe('offline handling and deduplicated indicator', () => {
	test('only top bar displays offline indicator when disconnected, inbox does not duplicate it', async ({ page }) => {
		// Intercept WebSocket creation to keep it in disconnected/closing state
		await page.addInitScript(() => {
			class DisconnectedWebSocket {
				static readonly CONNECTING = 0;
				static readonly OPEN = 1;
				static readonly CLOSING = 2;
				static readonly CLOSED = 3;

				readonly url: string;
				readyState = DisconnectedWebSocket.CLOSED;
				onopen: ((event: Event) => void) | null = null;
				onmessage: ((event: MessageEvent) => void) | null = null;
				onerror: ((event: Event) => void) | null = null;
				onclose: ((event: CloseEvent) => void) | null = null;

				constructor(url: string | URL) {
					this.url = String(url);
					queueMicrotask(() => {
						this.onerror?.(new Event('error'));
						this.onclose?.(new CloseEvent('close'));
					});
				}

				close() {}
			}

			Object.defineProperty(window, 'WebSocket', { configurable: true, value: DisconnectedWebSocket });
		});

		await mockWorkspaceApi(page);
		await page.goto('/inbox');

		// The top bar offline indicator should be visible
		const topbarIndicator = page.getByTestId('offline-indicator');
		await expect(topbarIndicator).toBeVisible();
		await expect(topbarIndicator).toHaveText(/Offline/);

		// Exactly one offline indicator component should exist on the page
		await expect(page.getByTestId('offline-indicator')).toHaveCount(1);

		// The conversation search bar in the inbox panel must NOT have any offline indicator
		const conversationSearchParent = page.getByPlaceholder('Search conversations').locator('..');
		await expect(conversationSearchParent.locator('text=Offline')).toHaveCount(0);
	});

	test('connected WebSocket hides offline indicator and does not rapidly poll conversations', async ({ page }) => {
		let conversationRequests = 0;
		page.on('request', (req) => {
			if (req.url().includes('/conversations?')) {
				conversationRequests++;
			}
		});

		await page.addInitScript(() => {
			class ConnectedWebSocket {
				static readonly CONNECTING = 0;
				static readonly OPEN = 1;
				static readonly CLOSING = 2;
				static readonly CLOSED = 3;

				readonly url: string;
				readyState = ConnectedWebSocket.CONNECTING;
				onopen: ((event: Event) => void) | null = null;
				onmessage: ((event: MessageEvent) => void) | null = null;
				onerror: ((event: Event) => void) | null = null;
				onclose: ((event: CloseEvent) => void) | null = null;

				constructor(url: string | URL) {
					this.url = String(url);
					queueMicrotask(() => {
						this.readyState = ConnectedWebSocket.OPEN;
						this.onopen?.(new Event('open'));
					});
				}

				close() {
					this.readyState = ConnectedWebSocket.CLOSED;
				}
			}

			Object.defineProperty(window, 'WebSocket', { configurable: true, value: ConnectedWebSocket });
		});

		await mockWorkspaceApi(page);
		await page.goto('/inbox');

		// Indicator should NOT be visible when connected
		const topbarIndicator = page.getByTestId('offline-indicator');
		await expect(topbarIndicator).not.toBeVisible();

		// Wait for initial load to settle
		await page.waitForTimeout(1000);
		await expect.poll(() => conversationRequests).toBeGreaterThan(0);
		const countAfterLoad = conversationRequests;

		// Wait 3.5s
		await page.waitForTimeout(3500);

		// With push-first real-time, no repeated polling occurs while connected.
		expect(conversationRequests).toBe(countAfterLoad);
	});

	test('disconnecting and reconnecting WebSocket updates indicator and triggers catch-up synchronization', async ({ page }) => {
		let conversationFetches = 0;
		page.on('request', (req) => {
			if (req.url().includes('/conversations?')) {
				conversationFetches++;
			}
		});

		await page.addInitScript(() => {
			const activeSockets: any[] = [];

			class ControllableWebSocket {
				static readonly CONNECTING = 0;
				static readonly OPEN = 1;
				static readonly CLOSING = 2;
				static readonly CLOSED = 3;

				readonly url: string;
				readyState = ControllableWebSocket.CONNECTING;
				onopen: ((event: Event) => void) | null = null;
				onmessage: ((event: MessageEvent) => void) | null = null;
				onerror: ((event: Event) => void) | null = null;
				onclose: ((event: CloseEvent) => void) | null = null;

				constructor(url: string | URL) {
					this.url = String(url);
					activeSockets.push(this);
					queueMicrotask(() => {
						if (this.readyState === ControllableWebSocket.CONNECTING) {
							this.readyState = ControllableWebSocket.OPEN;
							this.onopen?.(new Event('open'));
						}
					});
				}

				close() {
					if (this.readyState === ControllableWebSocket.CLOSED) return;
					this.readyState = ControllableWebSocket.CLOSED;
					const cb = this.onclose;
					queueMicrotask(() => cb?.(new CloseEvent('close')));
				}

				dropConnection() {
					this.readyState = ControllableWebSocket.CLOSED;
					const cb = this.onclose;
					queueMicrotask(() => cb?.(new CloseEvent('close')));
				}
			}

			Object.defineProperty(window, 'WebSocket', { configurable: true, value: ControllableWebSocket });
			(window as any).__activeSockets = activeSockets;
		});

		await mockWorkspaceApi(page);
		await page.goto('/inbox');

		// 1. Initial state: wait for WebSocket to be open
		const topbarIndicator = page.getByTestId('offline-indicator');
		await expect.poll(async () => page.evaluate(() => (window as any).__activeSockets?.[0]?.readyState === 1)).toBe(true);
		await expect(topbarIndicator).not.toBeVisible();
		const countBeforeDisconnect = conversationFetches;

		// 2. Simulate socket disconnect
		await page.evaluate(() => {
			const sockets = (window as any).__activeSockets;
			if (sockets && sockets.length > 0) {
				sockets[sockets.length - 1].dropConnection();
			}
		});

		// 3. Offline indicator becomes visible
		await expect(topbarIndicator).toBeVisible();

		// 4. Wait for store reconnect (attempt 1 uses delay = 2000ms)
		// When the new socket connects, it triggers catch-up sync and hides the indicator
		await expect(topbarIndicator).not.toBeVisible({ timeout: 10000 });
		await expect.poll(() => conversationFetches).toBeGreaterThan(countBeforeDisconnect);
	});

	test('browser online and offline events toggle offline indicator', async ({ page }) => {
		await page.addInitScript(() => {
			class ConnectedWebSocket {
				static readonly CONNECTING = 0;
				static readonly OPEN = 1;
				static readonly CLOSING = 2;
				static readonly CLOSED = 3;

				readonly url: string;
				readyState = ConnectedWebSocket.OPEN;
				onopen: ((event: Event) => void) | null = null;
				onmessage: ((event: MessageEvent) => void) | null = null;
				onerror: ((event: Event) => void) | null = null;
				onclose: ((event: CloseEvent) => void) | null = null;

				constructor(url: string | URL) {
					this.url = String(url);
					queueMicrotask(() => {
						this.readyState = ConnectedWebSocket.OPEN;
						this.onopen?.(new Event('open'));
					});
				}

				close() {
					this.readyState = ConnectedWebSocket.CLOSED;
				}
			}

			Object.defineProperty(window, 'WebSocket', { configurable: true, value: ConnectedWebSocket });
		});

		await mockWorkspaceApi(page);
		await page.goto('/inbox');
		await expect(page.getByRole('heading', { name: 'Inbox', level: 1 })).toBeVisible();

		const topbarIndicator = page.getByTestId('offline-indicator');
		await expect(topbarIndicator).not.toBeVisible();

		// Fire offline event
		await page.evaluate(() => {
			window.dispatchEvent(new Event('offline'));
		});

		// Offline indicator is now visible
		await expect(topbarIndicator).toBeVisible();

		// Fire online event
		await page.evaluate(() => {
			window.dispatchEvent(new Event('online'));
		});

		// Offline indicator is hidden
		await expect(topbarIndicator).not.toBeVisible();
	});
});
