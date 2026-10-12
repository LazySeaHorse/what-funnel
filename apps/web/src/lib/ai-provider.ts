import { ApiError, apiRequest, type ApiRequestOptions } from '$lib/api';

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

type AIProviderRequest = (path: string, options?: ApiRequestOptions) => Promise<any>;

/** Provider settings shown before an account has saved its own. */
export const DEFAULT_AI_PROVIDER_CONFIG = {
	base_url: 'https://generativelanguage.googleapis.com/v1beta/openai/',
	analysis_model: 'gemma-4-26b-a4b-it',
	reply_model: 'gemini-flash-lite-latest',
	embedding_model: 'gemini-embedding-001'
};

export type AIProviderSettingsValues = typeof DEFAULT_AI_PROVIDER_CONFIG;

/** Overlays the non-empty fields of an ai-config status response on the current values. */
export function applyAIProviderStatus(
	status: any,
	current: AIProviderSettingsValues
): { configured: boolean; values: AIProviderSettingsValues } {
	return {
		configured: status?.configured === true,
		values: {
			base_url: status?.base_url || current.base_url,
			analysis_model: status?.analysis_model || current.analysis_model,
			reply_model: status?.reply_model || current.reply_model,
			embedding_model: status?.embedding_model || current.embedding_model
		}
	};
}

/** Returns the message to show when the form cannot be submitted, or null when it can. */
export function validateAIProviderConfig(
	config: AIProviderConfigInput,
	configured: boolean,
	missingKeyMessage: string
): string | null {
	if (!configured && !config.api_key.trim()) return missingKeyMessage;
	if (!config.base_url.trim() || !config.analysis_model.trim() || !config.reply_model.trim() || !config.embedding_model.trim()) {
		return 'Base URL and all three model names are required.';
	}
	return null;
}

export type AIProviderTestOutcome =
	| { ok: true; checks: AIProviderTestCheck[]; message?: string; fingerprint: string }
	| { ok: false; checks: AIProviderTestCheck[]; error: unknown };

/** Tests the provider connection; the fingerprint of a passing config lets a later save skip re-testing. */
export async function testAIProvider(
	input: AIProviderConfigInput,
	request: AIProviderRequest = apiRequest
): Promise<AIProviderTestOutcome> {
	try {
		const config = normalizeAIProviderConfig(input);
		const res = await request('/workspace/account/ai-config/test', { method: 'POST', body: config });
		const result = aiProviderTestResult(res);
		if (!result?.ok) throw new Error('Provider returned an invalid connection-test result.');
		return { ok: true, checks: result.checks, message: result.message, fingerprint: aiProviderConfigFingerprint(config) };
	} catch (error) {
		return { ok: false, checks: aiProviderTestResultFromError(error)?.checks ?? [], error };
	}
}

/** Saves the provider config, first testing it unless it matches the last verified fingerprint. */
export async function saveAIProviderConfig(
	input: AIProviderConfigInput,
	verifiedFingerprint: string,
	request: AIProviderRequest = apiRequest
): Promise<void> {
	const config = normalizeAIProviderConfig(input);
	if (verifiedFingerprint !== aiProviderConfigFingerprint(config)) {
		await request('/workspace/account/ai-config/test', { method: 'POST', body: config });
	}
	await request('/workspace/account/ai-config', { method: 'PUT', body: config });
}
