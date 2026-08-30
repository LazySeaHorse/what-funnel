<script lang="ts">
	import { CpuChipIcon, SparklesIcon, PencilSquareIcon } from '@fvilers/heroicons-svelte/24/outline';
	import { apiRequest } from '$lib/api';

	let {
		step,
		totalSteps,
		aiMode = $bindable(),
		providerConfigured,
		providerApiKey = $bindable(),
		providerBaseURL = $bindable(),
		completionModel = $bindable(),
		embeddingModel = $bindable()
	}: {
		step: number;
		totalSteps: number;
		aiMode: 'auto_answer' | 'suggest_only' | 'manual';
		providerConfigured: boolean;
		providerApiKey: string;
		providerBaseURL: string;
		completionModel: string;
		embeddingModel: string;
	} = $props();

	let testing = $state(false);
	let testResult = $state<{ ok: boolean; message: string } | null>(null);

	async function testConnection() {
		if (!providerConfigured && !providerApiKey.trim()) {
			testResult = { ok: false, message: 'API key is required to test the connection.' };
			return;
		}
		if (!providerBaseURL.trim() || !completionModel.trim() || !embeddingModel.trim()) {
			testResult = { ok: false, message: 'Base URL, completion model, and embedding model are required.' };
			return;
		}

		testing = true;
		testResult = null;
		try {
			const res = await apiRequest('/workspace/account/ai-config/test', {
				method: 'POST',
				body: {
					config: JSON.stringify({
						api_key: providerApiKey.trim(),
						base_url: providerBaseURL.trim().replace(/\/$/, ''),
						completion_model: completionModel.trim(),
						embedding_model: embeddingModel.trim()
					})
				}
			});
			testResult = { ok: true, message: res?.message || 'Connection verified successfully.' };
		} catch (error: any) {
			testResult = { ok: false, message: error?.message || 'Connection test failed.' };
		} finally {
			testing = false;
		}
	}
</script>

				<div class="text-center lg:text-left mb-6">
					<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {step} of {totalSteps}</div>
					<h2 class="text-2xl sm:text-3xl font-medium text-slate-900 tracking-tight mb-1">Configure AI assistant</h2>
					<p class="text-sm text-slate-500 font-normal">Select how the AI handles customer conversations.</p>
				</div>

				<div class="space-y-3 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
					<!-- Option 1: Auto answer -->
					<button
						type="button"
						class="w-full text-left p-4 rounded-xl border transition-all cursor-pointer flex items-start justify-between {aiMode === 'auto_answer' ? 'border-blue-600 bg-blue-50/40 ring-1 ring-blue-600' : 'border-slate-200 bg-white hover:border-slate-300'}"
						onclick={() => aiMode = 'auto_answer'}
					>
						<div class="flex items-start gap-3.5">
							<div class="w-8 h-8 rounded-lg {aiMode === 'auto_answer' ? 'bg-blue-600 text-white' : 'bg-slate-100 text-slate-500'} flex items-center justify-center shrink-0 mt-0.5">
								<CpuChipIcon class="w-5 h-5" />
							</div>
							<div>
								<div class="flex items-center gap-2">
									<span class="text-sm font-medium text-slate-900">Auto answer when confident</span>
									<span class="px-2 py-0.5 rounded-md bg-blue-100 text-blue-700 text-[10px] font-medium">Recommended</span>
								</div>
								<p class="text-xs text-slate-500 mt-1 leading-relaxed font-normal">The AI answers customer questions automatically when confidence is high.</p>
							</div>
						</div>
						<div class="w-4 h-4 rounded-full border flex items-center justify-center mt-1 shrink-0 {aiMode === 'auto_answer' ? 'border-blue-600' : 'border-slate-300'}">
							{#if aiMode === 'auto_answer'}
								<div class="w-2 h-2 rounded-full bg-blue-600"></div>
							{/if}
						</div>
					</button>

					<!-- Option 2: Suggest replies only -->
					<button
						type="button"
						class="w-full text-left p-4 rounded-xl border transition-all cursor-pointer flex items-start justify-between {aiMode === 'suggest_only' ? 'border-blue-600 bg-blue-50/40 ring-1 ring-blue-600' : 'border-slate-200 bg-white hover:border-slate-300'}"
						onclick={() => aiMode = 'suggest_only'}
					>
						<div class="flex items-start gap-3.5">
							<div class="w-8 h-8 rounded-lg {aiMode === 'suggest_only' ? 'bg-blue-600 text-white' : 'bg-slate-100 text-slate-500'} flex items-center justify-center shrink-0 mt-0.5">
								<SparklesIcon class="w-5 h-5" />
							</div>
							<div>
								<span class="text-sm font-medium text-slate-900">Suggest replies only</span>
								<p class="text-xs text-slate-500 mt-1 leading-relaxed font-normal">The AI creates draft responses. Team members review and send the responses.</p>
							</div>
						</div>
						<div class="w-4 h-4 rounded-full border flex items-center justify-center mt-1 shrink-0 {aiMode === 'suggest_only' ? 'border-blue-600' : 'border-slate-300'}">
							{#if aiMode === 'suggest_only'}
								<div class="w-2 h-2 rounded-full bg-blue-600"></div>
							{/if}
						</div>
					</button>

					<!-- Option 3: Manual only -->
					<button
						type="button"
						class="w-full text-left p-4 rounded-xl border transition-all cursor-pointer flex items-start justify-between {aiMode === 'manual' ? 'border-blue-600 bg-blue-50/40 ring-1 ring-blue-600' : 'border-slate-200 bg-white hover:border-slate-300'}"
						onclick={() => aiMode = 'manual'}
					>
						<div class="flex items-start gap-3.5">
							<div class="w-8 h-8 rounded-lg {aiMode === 'manual' ? 'bg-blue-600 text-white' : 'bg-slate-100 text-slate-500'} flex items-center justify-center shrink-0 mt-0.5">
								<PencilSquareIcon class="w-5 h-5" />
							</div>
							<div>
								<span class="text-sm font-medium text-slate-900">Manual only</span>
								<p class="text-xs text-slate-500 mt-1 leading-relaxed font-normal">The AI does not send messages. Team members write all replies manually.</p>
							</div>
						</div>
						<div class="w-4 h-4 rounded-full border flex items-center justify-center mt-1 shrink-0 {aiMode === 'manual' ? 'border-blue-600' : 'border-slate-300'}">
							{#if aiMode === 'manual'}
								<div class="w-2 h-2 rounded-full bg-blue-600"></div>
							{/if}
						</div>
					</button>

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
								<label for="ai-provider-key" class="block text-xs font-medium text-slate-700">API key {providerConfigured ? '(leave blank to keep current key)' : ''}</label>
								<input id="ai-provider-key" type="password" autocomplete="new-password" bind:value={providerApiKey} class="wf-input" placeholder={providerConfigured ? 'Configured' : 'Required'} />
								{#if !providerConfigured && !providerApiKey.trim()}
									<p class="text-[11px] text-amber-700">Enter your AI provider API key, or select Manual only.</p>
								{/if}
							</div>
							<div class="space-y-1.5">
								<label for="ai-provider-url" class="block text-xs font-medium text-slate-700">OpenAI-compatible base URL</label>
								<input id="ai-provider-url" type="url" bind:value={providerBaseURL} class="wf-input" required />
							</div>
							<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
								<label class="space-y-1.5 text-xs font-medium text-slate-700">Completion model<input aria-label="Completion model" bind:value={completionModel} class="wf-input" required /></label>
								<label class="space-y-1.5 text-xs font-medium text-slate-700">Embedding model<input aria-label="Embedding model" bind:value={embeddingModel} class="wf-input" required /></label>
							</div>
							<div class="flex items-center justify-between pt-1">
								<button
									type="button"
									onclick={testConnection}
									disabled={testing || (!providerConfigured && !providerApiKey.trim())}
									class="rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50"
								>
									{testing ? 'Testing...' : 'Test connection'}
								</button>
							</div>
							{#if testResult}
								<div role={testResult.ok ? 'status' : 'alert'} class="rounded-lg border p-2.5 text-xs font-medium {testResult.ok ? 'border-emerald-200 bg-emerald-50 text-emerald-700' : 'border-rose-200 bg-rose-50 text-rose-700'}">
									{testResult.message}
								</div>
							{/if}
						</div>
					{/if}
				</div>
