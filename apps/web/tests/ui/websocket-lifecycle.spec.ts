import { expect, test } from '@playwright/test';
import { mockWorkspaceApi } from '../support/mock-api';

test('leaving the inbox closes its WebSocket without reconnecting', async ({ page }) => {
	await page.addInitScript(() => {
		const sockets: Array<{ closeCalls: number }> = [];

		class MockWebSocket {
			static readonly CONNECTING = 0;
			static readonly OPEN = 1;
			static readonly CLOSING = 2;
			static readonly CLOSED = 3;

			readonly url: string;
			readyState = MockWebSocket.CONNECTING;
			closeCalls = 0;
			onopen: ((event: Event) => void) | null = null;
			onmessage: ((event: MessageEvent) => void) | null = null;
			onerror: ((event: Event) => void) | null = null;
			onclose: ((event: CloseEvent) => void) | null = null;

			constructor(url: string | URL) {
				this.url = String(url);
				sockets.push(this);
				queueMicrotask(() => {
					if (this.readyState !== MockWebSocket.CONNECTING) return;
					this.readyState = MockWebSocket.OPEN;
					this.onopen?.(new Event('open'));
				});
			}

			close() {
				if (this.readyState === MockWebSocket.CLOSED) return;
				this.closeCalls++;
				this.readyState = MockWebSocket.CLOSED;
				const onclose = this.onclose;
				queueMicrotask(() => onclose?.(new CloseEvent('close')));
			}
		}

		Object.defineProperty(window, 'WebSocket', { configurable: true, value: MockWebSocket });
		Object.defineProperty(window, '__testSockets', { configurable: true, value: sockets });
	});

	await mockWorkspaceApi(page);
	await page.goto('/inbox');
	const appSockets = () => page.evaluate(() =>
		(window as any).__testSockets.filter((socket: { url: string }) => new URL(socket.url).pathname === '/ws')
	);
	await expect.poll(async () => (await appSockets()).length).toBe(1);

	await page.getByRole('button', { name: 'Toggle workspace menu' }).click();
	await page.getByRole('button', { name: 'Sign out' }).click();
	await expect(page).toHaveURL(/\/login$/);

	await expect.poll(async () => (await appSockets())[0].closeCalls).toBe(1);
	await page.waitForTimeout(2_200);
	await expect.poll(async () => (await appSockets()).length).toBe(1);
});
