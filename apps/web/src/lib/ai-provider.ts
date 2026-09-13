import { ApiError } from '$lib/api';

export interface AIProviderConfigInput {
	api_key: string;
	base_url: string;
	analysis_model: string;
	reply_model: string;
	embedding_model: string;
}

export interface AIProviderTestCheck {
	role: 'analysis' | 'reply' | 'embedding';
	model: string;
	resolved_model?: string;
	ok: boolean;
	kind?: string;
	message: string;
}

export interface AIProviderTestResult {
	ok: boolean;
	message?: string;
	checks: AIProviderTestCheck[];
}

export function normalizeAIProviderConfig(config: AIProviderConfigInput): AIProviderConfigInput {
	return {
		api_key: config.api_key.trim(),
		base_url: config.base_url.trim().replace(/\/+$/, ''),
		analysis_model: config.analysis_model.trim(),
		reply_model: config.reply_model.trim(),
		embedding_model: config.embedding_model.trim()
	};
}

export function aiProviderConfigFingerprint(config: AIProviderConfigInput): string {
	return JSON.stringify(normalizeAIProviderConfig(config));
}

export function aiProviderTestResult(value: unknown): AIProviderTestResult | null {
	if (!value || typeof value !== 'object') return null;
	const candidate = value as Partial<AIProviderTestResult>;
	if (!Array.isArray(candidate.checks)) return null;
	return {
		ok: candidate.ok === true,
		message: typeof candidate.message === 'string' ? candidate.message : undefined,
		checks: candidate.checks
	};
}

export function aiProviderTestResultFromError(error: unknown): AIProviderTestResult | null {
	if (!(error instanceof ApiError)) return null;
	return aiProviderTestResult(error.data);
}
