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

export function typeLabel(type?: string) {
	return type ? type.charAt(0).toUpperCase() + type.slice(1).replace(/_/g, ' ') : 'General';
}

export function typeColor(type?: string) {
	return ({
		faq: 'bg-blue-50 text-blue-700 border-blue-200/70',
		pricing: 'bg-emerald-50 text-emerald-700 border-emerald-200/70',
		policy: 'bg-orange-50 text-orange-700 border-orange-200/70',
		hours: 'bg-purple-50 text-purple-700 border-purple-200/70',
		service: 'bg-rose-50 text-rose-700 border-rose-200/70'
	} as Record<string, string>)[(type || '').toLowerCase()] || 'bg-slate-50 text-slate-700 border-slate-200/70';
}

