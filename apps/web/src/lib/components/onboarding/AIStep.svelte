<script lang="ts">
	import { CpuChipIcon, SparklesIcon, PencilSquareIcon } from '@fvilers/heroicons-svelte/24/outline';
	import { Button, Input } from '$lib/components/ui';
	import {
		testAIProvider,
		validateAIProviderConfig,
		type AIProviderTestCheck
	} from '$lib/ai-provider';
	import AIProviderCheckList from '$lib/components/settings/AIProviderCheckList.svelte';

	let {
		step,
		totalSteps,
		aiMode = $bindable(),
		providerConfigured,
		providerApiKey = $bindable(),
		providerBaseURL = $bindable(),
		analysisModel = $bindable(),
		replyModel = $bindable(),
		embeddingModel = $bindable(),
		verifiedConfigFingerprint = $bindable('')
	}: {
		step: number;
		totalSteps: number;
		aiMode: 'auto_answer' | 'suggest_only' | 'manual';
		providerConfigured: boolean;
		providerApiKey: string;
		providerBaseURL: string;
		analysisModel: string;
		replyModel: string;
		embeddingModel: string;
		verifiedConfigFingerprint: string;
	} = $props();

	let testing = $state(false);
	let testResult = $state<{ ok: boolean; message: string } | null>(null);
	let testChecks = $state<AIProviderTestCheck[]>([]);

	async function testConnection() {
		const config = {
			api_key: providerApiKey,
			base_url: providerBaseURL,
			analysis_model: analysisModel,
			reply_model: replyModel,
			embedding_model: embeddingModel
		};
		const invalid = validateAIProviderConfig(config, providerConfigured, 'API key is required to test the connection.');
		if (invalid) {
			testResult = { ok: false, message: invalid };
			return;
		}

		testing = true;
		testResult = null;
		testChecks = [];
		const outcome = await testAIProvider(config);
		testChecks = outcome.checks;
		if (outcome.ok) {
			verifiedConfigFingerprint = outcome.fingerprint;
			testResult = { ok: true, message: outcome.message || 'Connection verified successfully.' };
		} else {
			verifiedConfigFingerprint = '';
			testResult = { ok: false, message: (outcome.error as Error)?.message || 'Connection test failed.' };
		}
		testing = false;
	}

	const modeOptions = [
		{
			mode: 'auto_answer',
			icon: CpuChipIcon,
			title: 'Auto answer when confident',
			description: 'The AI answers customer questions automatically when confidence is high.',
			recommended: true
		},
		{
			mode: 'suggest_only',
			icon: SparklesIcon,
			title: 'Suggest replies only',
			description: 'The AI creates draft responses. Team members review and send the responses.',
			recommended: false
		},
		{
			mode: 'manual',
			icon: PencilSquareIcon,
			title: 'Manual only',
			description: 'The AI does not send messages. Team members write all replies manually.',
			recommended: false
		}
	] as const;
</script>

				<div class="text-center lg:text-left mb-6">
					<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {step} of {totalSteps}</div>
					<h2 class="text-2xl sm:text-3xl font-medium text-slate-900 tracking-tight mb-1">Configure AI assistant</h2>
					<p class="text-sm text-slate-500 font-normal">Select how the AI handles customer conversations.</p>
				</div>

				<div class="space-y-3 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
					{#each modeOptions as option (option.mode)}
						<button
							type="button"
							class="w-full text-left p-4 rounded-xl border transition-all cursor-pointer flex items-start justify-between {aiMode === option.mode ? 'border-blue-600 bg-blue-50/40 ring-1 ring-blue-600' : 'border-slate-200 bg-white hover:border-slate-300'}"
							onclick={() => aiMode = option.mode}
						>
							<div class="flex items-start gap-3.5">
								<div class="w-8 h-8 rounded-lg {aiMode === option.mode ? 'bg-blue-600 text-white' : 'bg-slate-100 text-slate-500'} flex items-center justify-center shrink-0 mt-0.5">
									<option.icon class="w-5 h-5" />
								</div>
								<div>
									{#if option.recommended}
										<div class="flex items-center gap-2">
											<span class="text-sm font-medium text-slate-900">{option.title}</span>
											<span class="px-2 py-0.5 rounded-md bg-blue-100 text-blue-700 text-[10px] font-medium">Recommended</span>
										</div>
									{:else}
										<span class="text-sm font-medium text-slate-900">{option.title}</span>
									{/if}
									<p class="text-xs text-slate-500 mt-1 leading-relaxed font-normal">{option.description}</p>
								</div>
							</div>
							<div class="w-4 h-4 rounded-full border flex items-center justify-center mt-1 shrink-0 {aiMode === option.mode ? 'border-blue-600' : 'border-slate-300'}">
								{#if aiMode === option.mode}
									<div class="w-2 h-2 rounded-full bg-blue-600"></div>
								{/if}
							</div>
						</button>
					{/each}

					{#if aiMode !== 'manual'}
						<div class="mt-4 space-y-4 rounded-xl border border-slate-200 bg-slate-50/70 p-4">
							<div>
								<div class="flex items-center justify-between gap-3">
									<h3 class="text-sm font-medium text-slate-900">AI provider</h3>
									{#if providerConfigured}<span class="text-[11px] font-medium text-emerald-700">Configured</span>{/if}
								</div>
								<p class="mt-1 text-xs leading-relaxed text-slate-500">The system encrypts API credentials before storage. The AI does not generate responses until you configure a provider.</p>
							</div>
							<div class="space-y-1.5">
								<Input
									id="ai-provider-key"
									label={`API key ${providerConfigured ? '(leave blank to keep current key)' : ''}`}
									type="password"
									autocomplete="new-password"
									bind:value={providerApiKey}
									placeholder={providerConfigured ? 'Configured' : 'Required'}
								/>
								{#if !providerConfigured && !providerApiKey.trim()}
									<p class="text-[11px] text-orange-700">Enter your AI provider API key, or select Manual only.</p>
								{/if}
							</div>
							<Input
								id="ai-provider-url"
								label="OpenAI-compatible base URL"
								type="url"
								bind:value={providerBaseURL}
								required
							/>
							<div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
								<Input
									id="ai-provider-analysis-model"
									label="Analysis model"
									aria-label="Analysis model"
									bind:value={analysisModel}
									helper="Ingestion, spam, summaries"
									required
								/>
								<Input
									id="ai-provider-reply-model"
									label="Customer reply model"
									aria-label="Customer reply model"
									bind:value={replyModel}
									helper="Generated replies only"
									required
								/>
								<Input
									id="ai-provider-embedding-model"
									label="Embedding model"
									aria-label="Embedding model"
									bind:value={embeddingModel}
									required
								/>
							</div>
							<div class="flex items-center justify-between pt-1">
								<Button
									variant="secondary"
									size="xs"
									onclick={testConnection}
									disabled={testing || (!providerConfigured && !providerApiKey.trim())}
									busy={testing}
								>
									Test connection
								</Button>
							</div>
							{#if testResult}
								<div role={testResult.ok ? 'status' : 'alert'} class="rounded-lg border p-2.5 text-xs font-medium {testResult.ok ? 'border-emerald-200 bg-emerald-50 text-emerald-700' : 'border-rose-200 bg-rose-50 text-rose-700'}">
									{testResult.message}
								</div>
							{/if}
							<AIProviderCheckList checks={testChecks} class="space-y-1.5" />
						</div>
					{/if}
				</div>
