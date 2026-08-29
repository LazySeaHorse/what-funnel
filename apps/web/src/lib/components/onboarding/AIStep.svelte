<script lang="ts">
	import Icon from '$lib/Icon.svelte';

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
</script>

				<div class="text-center lg:text-left mb-6">
					<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {step} of {totalSteps}</div>
					<h2 class="text-2xl sm:text-3xl font-medium text-slate-900 tracking-tight mb-1">Meet your AI Assistant</h2>
					<p class="text-sm text-slate-500 font-normal">How should your assistant handle conversations?</p>
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
								<Icon name="bot" size={18} color="currentColor" />
							</div>
							<div>
								<div class="flex items-center gap-2">
									<span class="text-sm font-medium text-slate-900">Auto answer when confident</span>
									<span class="px-2 py-0.5 rounded-md bg-blue-100 text-blue-700 text-[10px] font-medium">Recommended</span>
								</div>
								<p class="text-xs text-slate-500 mt-1 leading-relaxed font-normal">AI will answer customer questions automatically when confidence is high.</p>
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
								<Icon name="sparkles" size={18} color="currentColor" />
							</div>
							<div>
								<span class="text-sm font-medium text-slate-900">Suggest replies only</span>
								<p class="text-xs text-slate-500 mt-1 leading-relaxed font-normal">AI will draft suggested responses for your team to review and dispatch.</p>
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
								<Icon name="edit" size={18} color="currentColor" />
							</div>
							<div>
								<span class="text-sm font-medium text-slate-900">Manual only</span>
								<p class="text-xs text-slate-500 mt-1 leading-relaxed font-normal">AI will not send messages automatically. All replies are composed manually.</p>
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
								<p class="mt-1 text-xs leading-relaxed text-slate-500">Credentials are encrypted before storage. What Funnel will not generate AI content until a provider is configured.</p>
							</div>
							<div class="space-y-1.5">
								<label for="ai-provider-key" class="block text-xs font-medium text-slate-700">API key {providerConfigured ? '(leave blank to keep current key)' : ''}</label>
								<input id="ai-provider-key" type="password" autocomplete="new-password" bind:value={providerApiKey} class="wf-input" placeholder={providerConfigured ? 'Configured' : 'Required'} />
								{#if !providerConfigured && !providerApiKey.trim()}
									<p class="text-[11px] text-amber-700">Add your AI provider API key, or choose Manual only.</p>
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
						</div>
					{/if}
				</div>
