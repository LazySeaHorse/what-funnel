import type { Page } from '@playwright/test';
import { FUZZ_PAYLOADS } from './payloads';

export class Mulberry32 {
	private s: number;

	constructor(seed: number) {
		this.s = seed >>> 0;
	}

	next(): number {
		let t = (this.s += 0x6d2b79f5);
		t = Math.imul(t ^ (t >>> 15), t | 1);
		t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
		return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
	}

	int(min: number, max: number): number {
		return Math.floor(this.next() * (max - min + 1)) + min;
	}

	pick<T>(array: T[]): T {
		return array[this.int(0, array.length - 1)];
	}

	chance(probability: number): boolean {
		return this.next() < probability;
	}
}

export interface FuzzActionRecord {
	step: number;
	type: string;
	target?: string;
	value?: string;
	timestamp: number;
}

export interface MonkeyFuzzerOptions {
	seed?: number;
	maxActions?: number;
	actionDelayMs?: number;
	allowDestructive?: boolean;
	ignoredConsoleErrors?: Array<string | RegExp>;
	onAction?: (action: FuzzActionRecord) => void;
}

const DEFAULT_DESTRUCTIVE_PATTERNS = [
	/log\s*out/i,
	/sign\s*out/i,
	/delete\s*workspace/i,
	/delete\s*account/i,
	/permanently\s*delete/i,
	/reset\s*database/i
];

export class DeterministicMonkeyFuzzer {
	readonly seed: number;
	private rng: Mulberry32;
	private page: Page;
	private maxActions: number;
	private actionDelayMs: number;
	private allowDestructive: boolean;
	private ignoredConsoleErrors: Array<string | RegExp>;
	private onActionCallback?: (action: FuzzActionRecord) => void;

	private actionHistory: FuzzActionRecord[] = [];
	private caughtErrors: Array<{ type: 'pageerror' | 'console'; message: string; stack?: string }> = [];

	constructor(page: Page, options: MonkeyFuzzerOptions = {}) {
		this.page = page;
		this.seed = options.seed ?? (process.env.FUZZ_SEED ? parseInt(process.env.FUZZ_SEED, 10) : Math.floor(Math.random() * 1000000));
		this.rng = new Mulberry32(this.seed);
		this.maxActions = options.maxActions ?? 50;
		this.actionDelayMs = options.actionDelayMs ?? 40;
		this.allowDestructive = options.allowDestructive ?? false;
		this.ignoredConsoleErrors = options.ignoredConsoleErrors ?? [
			/favicon\.ico/i,
			/ws proxy error/i,
			/ws proxy socket error/i,
			/ECONNRESET/i,
			/WebSocket/i,
			/WS error/i,
			/Failed to load resource: the server responded with a status of 404/i
		];
		this.onActionCallback = options.onAction;

		this.setupErrorListeners();
	}

	private setupErrorListeners(): void {
		this.page.on('pageerror', (err) => {
			this.caughtErrors.push({
				type: 'pageerror',
				message: err.message,
				stack: err.stack
			});
		});

		this.page.on('console', (msg) => {
			if (msg.type() === 'error') {
				const text = msg.text();
				const isIgnored = this.ignoredConsoleErrors.some((pattern) =>
					typeof pattern === 'string' ? text.includes(pattern) : pattern.test(text)
				);
				if (!isIgnored) {
					this.caughtErrors.push({
						type: 'console',
						message: text
					});
				}
			}
		});
	}

	recordAction(type: string, target?: string, value?: string): void {
		const record: FuzzActionRecord = {
			step: this.actionHistory.length + 1,
			type,
			target,
			value,
			timestamp: Date.now()
		};
		this.actionHistory.push(record);
		if (this.onActionCallback) {
			this.onActionCallback(record);
		}
	}

	getHistory(): FuzzActionRecord[] {
		return [...this.actionHistory];
	}

	private isDestructive(text: string): boolean {
		if (this.allowDestructive) return false;
		return DEFAULT_DESTRUCTIVE_PATTERNS.some((p) => p.test(text));
	}

	/**
	 * Run the deterministic monkey loop
	 */
	async run(): Promise<void> {
		for (let step = 0; step < this.maxActions; step++) {
			this.assertNoErrors();

			// Pick action category with probabilistic weights
			const roll = this.rng.next();

			try {
				if (roll < 0.45) {
					await this.actionClickInteractive();
				} else if (roll < 0.70) {
					await this.actionFillInput();
				} else if (roll < 0.82) {
					await this.actionKeyPress();
				} else if (roll < 0.92) {
					await this.actionTabHop();
				} else {
					await this.actionScroll();
				}
			} catch (err: any) {
				// Benign action dispatch failure (element detached, modal closed mid-click)
				// is normal during rapid fuzzing; only real runtime page errors should fail the test.
			}

			if (this.actionDelayMs > 0) {
				await this.page.waitForTimeout(this.actionDelayMs);
			}

			this.assertNoErrors();
		}
	}

	private async actionClickInteractive(): Promise<void> {
		const selector = 'button:visible:not([disabled]), a[href]:visible, [role="button"]:visible:not([disabled]), input[type="checkbox"]:visible:not([disabled])';
		const elements = await this.page.locator(selector).all();

		if (elements.length === 0) return;

		// Select a candidate
		const targetIndex = this.rng.int(0, elements.length - 1);
		const element = elements[targetIndex];

		const text = (await element.innerText().catch(() => '')) || (await element.getAttribute('aria-label').catch(() => '')) || '';
		if (this.isDestructive(text)) {
			return; // Skip destructive buttons
		}

		const tagName = await element.evaluate((el) => el.tagName.toLowerCase()).catch(() => 'element');
		const label = text.slice(0, 40).replace(/\s+/g, ' ').trim() || tagName;

		this.recordAction('CLICK', `${tagName}[${label}]`);
		await element.click({ timeout: 500, force: true }).catch(() => {});
	}

	private async actionFillInput(): Promise<void> {
		const selector = 'input:visible:not([type="hidden"]):not([type="checkbox"]):not([type="radio"]):not([disabled]):not([readonly]), textarea:visible:not([disabled]):not([readonly])';
		const inputs = await this.page.locator(selector).all();

		if (inputs.length === 0) return;

		const targetIndex = this.rng.int(0, inputs.length - 1);
		const input = inputs[targetIndex];

		const name = (await input.getAttribute('name').catch(() => '')) || (await input.getAttribute('id').catch(() => '')) || (await input.getAttribute('placeholder').catch(() => '')) || 'input';
		const payload = this.rng.pick(FUZZ_PAYLOADS);

		this.recordAction('FILL', `${name}`, payload.length > 50 ? `${payload.slice(0, 47)}...` : payload);

		// Either fill directly or type with random dispatch
		await input.fill(payload, { timeout: 500 }).catch(() => {});
	}

	private async actionKeyPress(): Promise<void> {
		const keys = ['Escape', 'Enter', 'Tab', 'ArrowDown', 'ArrowUp', 'Backspace', ' '];
		const key = this.rng.pick(keys);

		this.recordAction('KEY', key);
		await this.page.keyboard.press(key).catch(() => {});
	}

	private async actionTabHop(): Promise<void> {
		const sections = ['Inbox', 'Leads', 'Knowledge', 'Simulate', 'Settings'];
		const section = this.rng.pick(sections);

		const navButton = this.page.locator(`button:has-text("${section}")`).first();
		if (await navButton.isVisible().catch(() => false)) {
			this.recordAction('NAV', section);
			await navButton.click({ timeout: 1000 }).catch(() => {});
		}
	}

	private async actionScroll(): Promise<void> {
		const deltaY = this.rng.pick([-500, -200, 200, 500, 1000]);
		this.recordAction('SCROLL', `deltaY=${deltaY}`);
		await this.page.mouse.wheel(0, deltaY).catch(() => {});
	}

	assertNoErrors(): void {
		if (this.caughtErrors.length === 0) return;

		const firstError = this.caughtErrors[0];
		const recentHistory = this.actionHistory.slice(-15);
		const historyReport = recentHistory
			.map((h) => `  ${h.step}. [${h.type}] ${h.target || ''} ${h.value ? JSON.stringify(h.value) : ''}`)
			.join('\n');

		const failureMessage = [
			'\n💥 UI MONKEY FUZZER FOUND A FAILURE!',
			`----------------------------------------------------------------------`,
			`Random Seed: ${this.seed}`,
			`Error Type:  ${firstError.type}`,
			`Message:     ${firstError.message}`,
			firstError.stack ? `Stack Trace:\n${firstError.stack}` : '',
			`----------------------------------------------------------------------`,
			`To reproduce this exact run:`,
			`  FUZZ_SEED=${this.seed} npx playwright test tests/fuzz/monkey-mock.spec.ts`,
			`----------------------------------------------------------------------`,
			`Last Actions Prior to Failure:`,
			historyReport,
			`----------------------------------------------------------------------\n`
		]
			.filter(Boolean)
			.join('\n');

		throw new Error(failureMessage);
	}
}
