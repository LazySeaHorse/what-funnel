import { goto } from '$app/navigation';
import { apiRequest, type ApiRequestOptions } from '$lib/api';
import { decodeWorkspaceSettings } from '$lib/workspace-settings';
import { normalizeSavedTimeZone } from '$lib/timezones';
import { aiProviderConfigFingerprint, normalizeAIProviderConfig } from '$lib/ai-provider';
import { KnowledgeIngestionController } from '$lib/knowledge/ingestion-controller.svelte';
import type { IngestionResult } from '$lib/knowledge/ingestion';
import type {
	AIMode,
	KBStatus,
	OnboardingChannel,
	OnboardingUser,
	OnboardingWizardOptions,
	PipelineStage,
	ProductMode,
	StepItem
} from './types';

export const STEP_ITEMS: StepItem[] = [
	{ num: 1, label: 'Business info' },
	{ num: 2, label: 'Channels' },
	{ num: 3, label: 'Lead pipeline' },
	{ num: 4, label: 'Team members' },
	{ num: 5, label: 'AI assistant' },
	{ num: 6, label: 'Knowledge base' },
	{ num: 7, label: 'Review and finish' }
];

export const DEFAULT_CHANNELS: OnboardingChannel[] = [
	{ id: 'whatsapp', name: 'WhatsApp', type: 'whatsapp', icon: 'whatsapp', connected: false, color: '#25D366' },
	{ id: 'instagram', name: 'Instagram', type: 'instagram', icon: 'instagram', connected: false, color: '#E1306C' },
	{ id: 'messenger', name: 'Facebook Messenger', type: 'messenger', icon: 'messenger', connected: false, color: '#0084FF' },
	{ id: 'telegram', name: 'Telegram', type: 'telegram', icon: 'telegram', connected: false, color: '#229ED9' }
];

export const DEFAULT_PIPELINE_STAGES: PipelineStage[] = [
	{ key: 'new_lead', label: 'New Lead', color: '#64748B' },
	{ key: 'contacted', label: 'Contacted', color: '#0057D0' },
	{ key: 'follow_up', label: 'Follow-up', color: '#C27AFF' },
	{ key: 'converted', label: 'Converted', color: '#9AE600' },
	{ key: 'lost', label: 'Lost', color: '#EF4444' }
];

export function slugify(name: string): string {
	return name
		.toLowerCase()
		.trim()
		.replace(/[^a-z0-9]+/g, '-')
		.replace(/^-+|-+$/g, '');
}

export function sanitizePipelineStages(stages: PipelineStage[]): PipelineStage[] {
	const usedKeys = new Set<string>();
	return stages.map((s, idx) => {
		const slug = (s.label || '').trim().toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '');
		const baseKey = slug || s.key || `stage_${idx + 1}`;
		let key = baseKey;
		let counter = 1;
		while (usedKeys.has(key)) {
			counter++;
			key = `${baseKey}_${counter}`;
		}
		usedKeys.add(key);
		return {
			key,
			label: (s.label || '').trim() || 'Stage',
			color: s.color || '#3B82F6'
		};
	});
}

export class OnboardingWizardController {
	loading = $state(true);
	submitting = $state(false);
	error = $state('');
	productMode = $state<ProductMode>('full_workspace');
	pipelineID = $state('');

	// Step 1: Business info
	businessName = $state('');
	businessType = $state('');
	timezone = $state('UTC');

	// Step 2: Channels
	channels = $state<OnboardingChannel[]>(DEFAULT_CHANNELS.map((ch) => ({ ...ch })));

	// Step 3: Lead pipeline
	pipelineStages = $state<PipelineStage[]>(DEFAULT_PIPELINE_STAGES.map((st) => ({ ...st })));

	// Step 4: Team members & Workspace slug
	slug = $state('');
	users = $state<OnboardingUser[]>([]);

	// Step 5: AI Assistant
	aiMode = $state<AIMode>('auto_answer');
	aiProviderConfigured = $state(false);
	aiProviderApiKey = $state('');
	aiProviderBaseURL = $state('https://generativelanguage.googleapis.com/v1beta/openai/');
	aiAnalysisModel = $state('gemma-4-26b-a4b-it');
	aiReplyModel = $state('gemini-flash-lite-latest');
	aiEmbeddingModel = $state('gemini-embedding-001');
	aiVerifiedConfigFingerprint = $state('');

	// Step 6: Knowledge Base
	rawText = $state('');
	kbError = $state('');
	knowledgeIngestion: KnowledgeIngestionController;

	private internalStepNum = $state(1);
	private getStepNumFn?: () => number;
	private navigateFn: (path: string) => Promise<any> | any;
	private request: (path: string, options?: ApiRequestOptions) => Promise<any>;

	constructor(options?: OnboardingWizardOptions) {
		this.getStepNumFn = options?.getStepNum;
		this.navigateFn = options?.navigate ?? ((path: string) => goto(path));
		this.request = options?.request ?? apiRequest;
		this.knowledgeIngestion = options?.knowledgeIngestion ?? new KnowledgeIngestionController(this.request);
	}

	get stepNum(): number {
		return this.getStepNumFn ? this.getStepNumFn() : this.internalStepNum;
	}

	set stepNum(num: number) {
		this.internalStepNum = num;
	}

	get visibleStepItems(): StepItem[] {
		let items = STEP_ITEMS;
		if (this.productMode === 'chatbot_only') {
			items = items.filter((item) => item.num !== 3 && item.num !== 4);
		}
		if (this.aiMode === 'manual') {
			items = items.filter((item) => item.num !== 6);
		}
		return items;
	}

	get displayStepNum(): number {
		return Math.max(1, this.visibleStepItems.findIndex((item) => item.num === this.stepNum) + 1);
	}

	get kbStatus(): KBStatus {
		return this.knowledgeIngestion.phase === 'idle'
			? 'input'
			: this.knowledgeIngestion.phase === 'review'
				? 'results'
				: (this.knowledgeIngestion.phase as KBStatus);
	}

	get connectedChannelsText(): string {
		const conn = this.channels.filter((c) => c.connected).map((c) => c.name);
		return conn.length > 0 ? conn.join(', ') : 'WhatsApp, Instagram';
	}

	get aiModeLabel(): string {
		if (this.aiMode === 'auto_answer') return 'Auto answer when confident';
		if (this.aiMode === 'suggest_only') return 'Suggest replies only';
		return 'Manual only';
	}

	get kbTopicsSummary(): string {
		if (this.aiMode === 'manual') {
			return 'Skipped (Manual replies)';
		}
		if (this.knowledgeIngestion.concepts.length > 0) {
			return `${this.knowledgeIngestion.concepts.length} concepts in knowledge base`;
		}
		return 'Business information compiled';
	}

	get continueDisabled(): boolean {
		return (
			this.loading ||
			(this.stepNum === 5 && this.aiMode !== 'manual' && !this.aiProviderConfigured && !this.aiProviderApiKey.trim())
		);
	}

	async init(): Promise<void> {
		this.loading = true;
		this.error = '';

		try {
			await this.request('/auth/me');
		} catch {
			await this.navigateFn('/login');
			return;
		}

		try {
			const account = await this.request('/workspace/account');
			this.productMode = account?.product_mode === 'chatbot_only' ? 'chatbot_only' : 'full_workspace';
			const [pipelines, aiStatus] = await Promise.all([
				this.productMode === 'full_workspace' ? this.request('/workspace/pipelines') : Promise.resolve([]),
				this.request('/workspace/account/ai-config/status')
			]);
			this.aiProviderConfigured = aiStatus?.configured === true;
			if (aiStatus?.base_url) this.aiProviderBaseURL = aiStatus.base_url;
			if (aiStatus?.analysis_model) this.aiAnalysisModel = aiStatus.analysis_model;
			if (aiStatus?.reply_model) this.aiReplyModel = aiStatus.reply_model;
			if (aiStatus?.embedding_model) this.aiEmbeddingModel = aiStatus.embedding_model;
			if (account?.name) {
				this.businessName = account.name;
				if (!this.slug) this.slug = slugify(account.name);
			}
			const settings = decodeWorkspaceSettings(account?.settings);
			if (settings.business_type) this.businessType = settings.business_type;
			if (settings.timezone) this.timezone = normalizeSavedTimeZone(settings.timezone);
			if (settings.ai_enabled === false) this.aiMode = 'manual';
			else if (settings.ai_reply_mode_default === 'auto_send') this.aiMode = 'auto_answer';
			else if (settings.ai_reply_mode_default === 'draft_only') this.aiMode = 'suggest_only';

			if (this.productMode === 'full_workspace') {
				const pipeline = Array.isArray(pipelines) ? pipelines[0] : null;
				if (!pipeline?.id) throw new Error('Your default lead pipeline could not be loaded.');
				this.pipelineID = pipeline.id;
				if (Array.isArray(pipeline.states) && pipeline.states.length > 0) this.pipelineStages = pipeline.states;
			}

			try {
				const slugData = await this.request('/workspace/account/slug');
				if (slugData?.slug) this.slug = slugData.slug;
			} catch {}

			if (this.productMode === 'full_workspace') {
				try {
					const userList = await this.request('/workspace/users');
					if (Array.isArray(userList)) {
						this.users = userList
							.filter((u: any) => u.username)
							.map((u: any) => ({
								id: u.id,
								username: u.username,
								role: u.role
							}));
					}
				} catch {}
			}

			if (this.productMode === 'chatbot_only' && (this.stepNum === 3 || this.stepNum === 4)) {
				await this.skipWorkspaceOnlySteps();
				await this.goToStep(5);
				return;
			}

			const chList = await this.request('/channels');
			if (Array.isArray(chList) && chList.length > 0) {
				for (const c of chList) {
					const found = this.channels.find((item) => item.type === c.type);
					if (found) found.connected = c.status === 'connected';
				}
			}
			if (this.stepNum === 6 && settings.ai_enabled !== false) {
				void this.resumeLatestIngestion();
			}
		} catch (err: any) {
			this.error = err?.message || 'We could not load your saved setup. Refresh and try again.';
		} finally {
			this.loading = false;
		}
	}

	async goToStep(num: number): Promise<void> {
		if (this.productMode === 'chatbot_only' && (num === 3 || num === 4)) {
			num = 5;
		}
		this.internalStepNum = num;
		await this.navigateFn(`/onboarding/${num}`);
	}

	async skipWorkspaceOnlySteps(): Promise<void> {
		await Promise.all([
			this.request('/onboarding/status', { method: 'PATCH', body: { step: 'pipeline_setup', action: 'skip' } }),
			this.request('/onboarding/status', { method: 'PATCH', body: { step: 'team_setup', action: 'skip' } })
		]);
	}

	async handleBack(): Promise<void> {
		if (this.stepNum === 6 && this.knowledgeIngestion.phase === 'review') {
			this.editKnowledgeNotes();
			return;
		}
		if (this.stepNum === 7 && this.aiMode === 'manual') {
			await this.goToStep(5);
			return;
		}
		if (this.productMode === 'chatbot_only' && this.stepNum === 5) {
			await this.goToStep(2);
		} else if (this.stepNum > 1) {
			await this.goToStep(this.stepNum - 1);
		} else {
			await this.navigateFn('/login');
		}
	}

	async goToTour(): Promise<void> {
		await this.navigateFn('/inbox?tour=true');
	}

	async goToInbox(): Promise<void> {
		await this.navigateFn('/inbox');
	}

	async toggleChannel(_channel?: any): Promise<void> {
		await this.navigateFn('/inbox?tab=settings');
	}

	async addTeamMember(username: string, password: string, role: 'agent' | 'manager'): Promise<void> {
		const res = await this.request('/workspace/users', {
			method: 'POST',
			body: { username, password, role }
		});
		this.users = [
			...this.users,
			{
				id: res.id,
				username: res.username || username,
				role: res.role || role,
				plaintextPassword: res.password || password
			}
		];
	}

	async removeTeamMember(id: string): Promise<void> {
		await this.request(`/workspace/users/${id}`, { method: 'DELETE' });
		this.users = this.users.filter((u) => u.id !== id);
	}

	async handleKnowledgeIngestionResult(result: IngestionResult): Promise<void> {
		if (result.status !== 'complete') return;
		await this.request('/onboarding/status', {
			method: 'PATCH',
			body: { step: 'kb_setup', action: 'complete' }
		});
		await this.goToStep(7);
	}

	async resumeLatestIngestion(): Promise<void> {
		try {
			const result = await this.knowledgeIngestion.resumeLatest();
			if (result) await this.handleKnowledgeIngestionResult(result);
		} catch (err: any) {
			this.kbError = err?.message || 'Could not resume knowledge ingestion.';
		}
	}

	async startCompilingKB(): Promise<void> {
		if (!this.rawText.trim()) {
			await this.request('/onboarding/status', {
				method: 'PATCH',
				body: { step: 'kb_setup', action: 'skip' }
			});
			await this.goToStep(7);
			return;
		}

		this.kbError = '';

		try {
			const result = await this.knowledgeIngestion.start(this.rawText);
			await this.handleKnowledgeIngestionResult(result);
		} catch (err: any) {
			this.kbError = err?.message || 'Failed to process knowledge text. Check your AI provider settings and try again.';
		}
	}

	async publishCompiledKB(): Promise<void> {
		this.kbError = '';
		try {
			const result = await this.knowledgeIngestion.publish();
			await this.handleKnowledgeIngestionResult(result);
		} catch (err: any) {
			this.kbError = err?.message || 'Failed to add the reviewed concepts to your knowledge base.';
		}
	}

	editKnowledgeNotes(): void {
		this.knowledgeIngestion.discard();
		this.kbError = '';
	}

	async skipWaitingToNextStep(): Promise<void> {
		this.knowledgeIngestion.discard();
		await this.goToStep(7);
	}

	async saveBusinessInfo(): Promise<void> {
		if (!this.businessName.trim()) throw new Error('Business name is required.');
		await this.request('/workspace/account', {
			method: 'PATCH',
			body: { name: this.businessName.trim() }
		});
		await this.request('/workspace/account/settings', {
			method: 'PATCH',
			body: { business_type: this.businessType, timezone: this.timezone }
		});

		if (!this.slug) {
			this.slug = slugify(this.businessName);
		}

		await this.request('/onboarding/status', {
			method: 'PATCH',
			body: { step: 'business_basics', action: 'complete' }
		});

		await this.goToStep(2);
	}

	async saveChannels(): Promise<void> {
		await this.request('/onboarding/status', {
			method: 'PATCH',
			body: { step: 'channel_connect', action: this.channels.some((channel) => channel.connected) ? 'complete' : 'skip' }
		});

		if (this.productMode === 'chatbot_only') {
			await this.skipWorkspaceOnlySteps();
			await this.goToStep(5);
		} else {
			await this.goToStep(3);
		}
	}

	async savePipeline(): Promise<void> {
		if (!this.pipelineID) throw new Error('Your default lead pipeline is unavailable. Refresh and try again.');
		const sanitizedStages = sanitizePipelineStages(this.pipelineStages);

		await this.request(`/workspace/pipelines/${this.pipelineID}`, {
			method: 'PUT',
			body: {
				name: 'Default Pipeline',
				states: sanitizedStages
			}
		});

		await this.request('/onboarding/status', {
			method: 'PATCH',
			body: { step: 'pipeline_setup', action: 'complete' }
		});

		await this.goToStep(4);
	}

	async saveTeam(): Promise<void> {
		const effectiveSlug = this.slug.trim() || slugify(this.businessName) || 'workspace';
		await this.request('/workspace/account/slug', {
			method: 'PUT',
			body: { slug: effectiveSlug }
		});
		this.slug = effectiveSlug;

		await this.request('/onboarding/status', {
			method: 'PATCH',
			body: { step: 'team_setup', action: this.users.length > 0 ? 'complete' : 'skip' }
		});

		await this.goToStep(5);
	}

	async saveAI(): Promise<void> {
		const replyMode = this.aiMode === 'auto_answer' ? 'auto_send' : 'draft_only';
		if (this.aiMode !== 'manual' && !this.aiProviderConfigured && !this.aiProviderApiKey.trim()) {
			throw new Error('Add your AI provider API key, or choose Manual only.');
		}
		if (this.aiMode !== 'manual') {
			const providerConfig = normalizeAIProviderConfig({
				api_key: this.aiProviderApiKey,
				base_url: this.aiProviderBaseURL,
				analysis_model: this.aiAnalysisModel,
				reply_model: this.aiReplyModel,
				embedding_model: this.aiEmbeddingModel
			});
			if (this.aiVerifiedConfigFingerprint !== aiProviderConfigFingerprint(providerConfig)) {
				await this.request('/workspace/account/ai-config/test', {
					method: 'POST',
					body: providerConfig
				});
			}

			await this.request('/workspace/account/ai-config', {
				method: 'PUT',
				body: providerConfig
			});
			this.aiProviderConfigured = true;
			this.aiProviderApiKey = '';
		}
		await this.request('/workspace/account/settings', {
			method: 'PATCH',
			body: { ai_enabled: this.aiMode !== 'manual', ai_reply_mode_default: replyMode }
		});

		await this.request('/onboarding/status', {
			method: 'PATCH',
			body: { step: 'reply_mode', action: 'complete' }
		});

		if (this.aiMode === 'manual') {
			await this.request('/onboarding/status', {
				method: 'PATCH',
				body: { step: 'kb_setup', action: 'skip' }
			});
			await this.goToStep(7);
		} else {
			await this.goToStep(6);
		}
	}

	async saveKnowledgeBase(): Promise<void> {
		if (this.kbStatus === 'input') {
			await this.startCompilingKB();
		} else if (this.kbStatus === 'results') {
			await this.publishCompiledKB();
		}
	}

	async finishReview(): Promise<void> {
		await this.request('/onboarding/status', {
			method: 'PATCH',
			body: { step: 'review_finish', action: 'complete' }
		});

		await this.request('/onboarding/status', {
			method: 'PATCH',
			body: { step: 'done', action: 'complete' }
		});

		await this.goToStep(8);
	}

	async handleContinue(): Promise<void> {
		this.error = '';
		this.submitting = true;

		try {
			switch (this.stepNum) {
				case 1:
					await this.saveBusinessInfo();
					break;
				case 2:
					await this.saveChannels();
					break;
				case 3:
					await this.savePipeline();
					break;
				case 4:
					await this.saveTeam();
					break;
				case 5:
					await this.saveAI();
					break;
				case 6:
					await this.saveKnowledgeBase();
					break;
				case 7:
					await this.finishReview();
					break;
			}
		} catch (err: any) {
			this.error = err?.message || 'Failed to save step settings. Please try again.';
		} finally {
			this.submitting = false;
		}
	}

	dispose(): void {
		this.knowledgeIngestion.dispose();
	}
}
