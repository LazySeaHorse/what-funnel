import { apiRequest, type ApiRequestOptions } from '$lib/api';
import {
	normalizeConcepts,
	normalizePatterns,
	type IngestionConcept,
	type IngestionPattern,
	type IngestionPhase,
	type IngestionResult
} from './ingestion';

type Request = (path: string, options?: ApiRequestOptions) => Promise<any>;

const ACTIVE_STATUSES = new Set(['queued', 'processing', 'review_required', 'publishing']);

export class KnowledgeIngestionController {
	id = $state('');
	phase = $state<IngestionPhase>('idle');
	concepts = $state<IngestionConcept[]>([]);
	patterns = $state<IngestionPattern[]>([]);
	busy = $state(false);

	private request: Request;
	private pollInterval: number;
	private abortController: AbortController | null = null;

	constructor(request: Request = apiRequest, pollInterval = 750) {
		this.request = request;
		this.pollInterval = pollInterval;
	}

	async start(rawText: string): Promise<IngestionResult> {
		this.begin('processing');
		try {
			const ingestion = await this.request('/api/kb/ingestions', {
				method: 'POST',
				body: { raw_text: rawText.trim() },
				signal: this.abortController?.signal
			});
			this.id = ingestion.id;
			return await this.poll(ingestion.id);
		} catch (error) {
			return this.handleFailure(error, 'idle');
		}
	}

	async resumeLatest(): Promise<IngestionResult | null> {
		this.cancelRequest();
		const controller = new AbortController();
		this.abortController = controller;
		try {
			const response = await this.request('/api/kb/ingestions/latest', { signal: controller.signal });
			const ingestion = response?.ingestion;
			if (!ingestion || !ACTIVE_STATUSES.has(ingestion.status)) {
				this.finishBusy();
				return null;
			}
			this.id = ingestion.id;
			this.busy = true;
			this.phase = ingestion.status === 'publishing' ? 'publishing' : 'processing';
			return await this.poll(ingestion.id);
		} catch (error) {
			return this.handleFailure(error, 'idle');
		}
	}

	async publish(): Promise<IngestionResult> {
		if (!this.id) throw new Error('The knowledge ingestion is unavailable. Organize your notes again.');
		if (!this.concepts.some((item) => item.approved) && !this.patterns.some((item) => item.approved)) {
			throw new Error('Select at least one concept or pattern to add.');
		}

		this.begin('publishing', false);
		try {
			await this.request(`/api/kb/ingestions/${this.id}/publish`, {
				method: 'POST',
				body: { concepts: this.concepts, patterns: this.patterns },
				signal: this.abortController?.signal
			});
			return await this.poll(this.id);
		} catch (error) {
			return this.handleFailure(error, 'review');
		}
	}

	discard() {
		this.cancelRequest();
		this.id = '';
		this.concepts = [];
		this.patterns = [];
		this.phase = 'idle';
		this.busy = false;
	}

	dispose() {
		this.discard();
	}

	private begin(phase: IngestionPhase, clearReview = true) {
		this.cancelRequest();
		this.abortController = new AbortController();
		this.busy = true;
		this.phase = phase;
		if (clearReview) {
			this.concepts = [];
			this.patterns = [];
		}
	}

	private async poll(id: string): Promise<IngestionResult> {
		const controller = this.abortController;
		if (!controller) return { status: 'cancelled' };

		while (!controller.signal.aborted) {
			const ingestion = await this.request(`/api/kb/ingestions/${id}`, { signal: controller.signal });
			switch (ingestion.status) {
				case 'review_required':
					this.concepts = normalizeConcepts(ingestion.concepts);
					this.patterns = normalizePatterns(ingestion.patterns);
					this.phase = 'review';
					this.finishBusy(controller);
					return { status: 'review_required' };
				case 'complete': {
					const result: IngestionResult = {
						status: 'complete',
						conceptsAdded: this.concepts.filter((item) => item.approved).length,
						patternsAdded: this.patterns.filter((item) => item.approved).length
					};
					this.id = '';
					this.concepts = [];
					this.patterns = [];
					this.phase = 'idle';
					this.finishBusy(controller);
					return result;
				}
				case 'failed':
					throw new Error(ingestion.error || 'Knowledge ingestion failed.');
				case 'publishing':
					this.phase = 'publishing';
					break;
				case 'queued':
				case 'processing':
					this.phase = 'processing';
					break;
				default:
					throw new Error(`Unknown knowledge ingestion status: ${ingestion.status}`);
			}
			await abortableDelay(this.pollInterval, controller.signal);
		}

		return { status: 'cancelled' };
	}

	private handleFailure(error: unknown, fallbackPhase: IngestionPhase): never | IngestionResult {
		if (isAbortError(error)) return { status: 'cancelled' };
		this.phase = fallbackPhase;
		this.finishBusy();
		throw error;
	}

	private finishBusy(controller?: AbortController) {
		if (!controller || this.abortController === controller) {
			this.busy = false;
			this.abortController = null;
		}
	}

	private cancelRequest() {
		this.abortController?.abort();
		this.abortController = null;
	}
}

function abortableDelay(milliseconds: number, signal: AbortSignal): Promise<void> {
	return new Promise((resolve, reject) => {
		if (signal.aborted) return reject(new DOMException('Aborted', 'AbortError'));
		const timeout = setTimeout(done, milliseconds);
		function done() {
			signal.removeEventListener('abort', aborted);
			resolve();
		}
		function aborted() {
			clearTimeout(timeout);
			reject(new DOMException('Aborted', 'AbortError'));
		}
		signal.addEventListener('abort', aborted, { once: true });
	});
}

function isAbortError(error: unknown): boolean {
	return error instanceof DOMException && error.name === 'AbortError';
}
