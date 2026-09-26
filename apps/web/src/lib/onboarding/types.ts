import type { ApiRequestOptions } from '$lib/api';
import type { KnowledgeIngestionController } from '$lib/knowledge/ingestion-controller.svelte';

export type ProductMode = 'full_workspace' | 'chatbot_only';
export type AIMode = 'auto_answer' | 'suggest_only' | 'manual';
export type KBStatus = 'input' | 'processing' | 'results' | 'publishing';

export interface OnboardingChannel {
	id: string;
	name: string;
	type: string;
	icon: string;
	connected: boolean;
	color: string;
}

export interface PipelineStage {
	key: string;
	label: string;
	color: string;
}

export interface OnboardingUser {
	id: string;
	username: string;
	role: string;
	plaintextPassword?: string;
}

export interface StepItem {
	num: number;
	label: string;
}

export interface OnboardingWizardOptions {
	getStepNum?: () => number;
	navigate?: (path: string) => Promise<any> | any;
	request?: (path: string, options?: ApiRequestOptions) => Promise<any>;
	knowledgeIngestion?: KnowledgeIngestionController;
}
