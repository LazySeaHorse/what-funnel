export interface IngestionConcept {
	id: string;
	title: string;
	type: string;
	tags: string[];
	body_text: string;
	approved: boolean;
}

export interface IngestionPattern {
	id: string;
	canonical_question: string;
	answer_text: string;
	trigger_phrases: string[];
	approved: boolean;
}

export type IngestionPhase = 'idle' | 'processing' | 'review' | 'publishing';

export type IngestionResult =
	| { status: 'review_required' }
	| { status: 'complete'; conceptsAdded: number; patternsAdded: number }
	| { status: 'cancelled' };

export function normalizeConcepts(items: unknown): IngestionConcept[] {
	return (Array.isArray(items) ? items : []).map((item: any) => ({
		id: item.id,
		title: item.title ?? '',
		type: item.type ?? 'faq',
		tags: Array.isArray(item.tags) ? item.tags : [],
		body_text: item.body_text ?? '',
		approved: item.status !== 'rejected'
	}));
}

export function normalizePatterns(items: unknown): IngestionPattern[] {
	return (Array.isArray(items) ? items : []).map((item: any) => ({
		id: item.id,
		canonical_question: item.canonical_question ?? '',
		answer_text: item.answer_text ?? '',
		trigger_phrases: Array.isArray(item.trigger_phrases) ? item.trigger_phrases : [],
		approved: item.status !== 'rejected'
	}));
}
