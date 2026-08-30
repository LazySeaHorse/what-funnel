<script lang="ts">
	import Icon from '$lib/Icon.svelte';

	type Concept = { id: string; title: string; type: string; tags: string[]; body_markdown: string; approved: boolean };
	type Pattern = { id: string; canonical_question: string; answer_markdown: string; trigger_phrases: string[]; approved: boolean };

	let {
		step,
		totalSteps,
		rawText = $bindable(),
		status,
		concepts = $bindable(),
		patterns = $bindable(),
		compiling,
		errorMessage,
		onSkipWaiting,
		onEditNotes
	}: {
		step: number;
		totalSteps: number;
		rawText: string;
		status: 'input' | 'processing' | 'results' | 'publishing';
		concepts: Concept[];
		patterns: Pattern[];
		compiling: boolean;
		errorMessage: string;
		onSkipWaiting: () => void;
		onEditNotes: () => void;
	} = $props();

	function appendTemplateChunk(label: string, text: string) {
		if (rawText.includes(label)) return;
		rawText = rawText.trim() + `\n\n${label}:\n${text}`;
	}

	function updateTriggers(pattern: Pattern, value: string) {
		pattern.trigger_phrases = value.split('\n').map((phrase) => phrase.trim()).filter(Boolean);
	}
</script>

				{#if errorMessage}
					<div role="alert" class="mb-4 w-full rounded-xl border border-rose-200 bg-rose-50 p-3 text-xs font-medium text-rose-700">{errorMessage}</div>
				{/if}

				<div class="text-center lg:text-left mb-6">
					<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {step} of {totalSteps}</div>
					<h2 class="text-2xl font-medium text-slate-900 tracking-tight mb-1">Teach your AI assistant</h2>
					<p class="text-sm text-slate-500 font-normal max-w-lg lg:max-w-none mx-auto lg:mx-0">Add business notes, price lists, FAQs, hours, or policies. The AI compiler organizes it automatically.</p>
				</div>

				{#if status === 'input'}
					<div class="space-y-4 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
						<div class="flex flex-wrap items-center justify-center lg:justify-start gap-2">
							<span class="text-xs font-medium text-slate-500">Quick templates:</span>
							<button type="button" class="px-2.5 py-1 text-xs font-medium rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-700 transition" onclick={() => appendTemplateChunk('Services & Pricing', '- Standard service: $50\n- Premium package: $120')}>
								+ Pricing
							</button>
							<button type="button" class="px-2.5 py-1 text-xs font-medium rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-700 transition" onclick={() => appendTemplateChunk('Business Hours', '- Monday–Friday: 9:00 AM – 6:00 PM\n- Saturday: 10:00 AM – 4:00 PM')}>
								+ Hours
							</button>
							<button type="button" class="px-2.5 py-1 text-xs font-medium rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-700 transition" onclick={() => appendTemplateChunk('Cancellation Policy', '- 24-hour advance notice required')}>
								+ Policy
							</button>
							<button type="button" class="px-2.5 py-1 text-xs font-medium rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-700 transition" onclick={() => appendTemplateChunk('FAQs', '- Free customer parking on-site\n- Walk-ins accepted based on availability')}>
								+ FAQs
							</button>
						</div>

						<textarea
							class="w-full h-52 p-4 bg-white border border-slate-200 rounded-xl text-xs sm:text-sm text-slate-900 placeholder:text-slate-400 focus:border-blue-600 focus:ring-2 focus:ring-blue-100 outline-none leading-relaxed resize-none font-normal"
							placeholder="Paste raw business info, services, pricing, business hours, cancellation rules, FAQ answers, or message templates..."
							bind:value={rawText}
						></textarea>
					</div>

				{:else if status === 'processing' || status === 'publishing'}
					<div class="py-12 flex flex-col items-center justify-center text-center space-y-4 w-full">
						<div class="w-12 h-12 rounded-2xl bg-blue-50 border border-blue-100 flex items-center justify-center text-blue-600">
							<Icon name="sparkles" size={24} color="currentColor" />
						</div>
						<h2 class="text-xl font-medium text-slate-900">{status === 'publishing' ? 'Adding reviewed knowledge...' : 'Organizing your knowledge...'}</h2>
						<p class="text-xs sm:text-sm text-slate-500 max-w-sm lg:max-w-none font-normal">
							{status === 'publishing' ? 'Creating searchable concepts for your AI assistant.' : 'Structuring raw business notes into categorized knowledge concepts.'}
						</p>

						{#if status === 'processing'}<button
							type="button"
							class="mt-4 px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-700 text-xs font-medium rounded-xl transition"
							onclick={onSkipWaiting}
						>
							Skip waiting and go to next page →
						</button>{/if}
					</div>

				{:else if status === 'results'}
					<div class="flex items-center justify-between mb-4 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
						<div>
							<h2 class="text-2xl font-medium text-slate-900 tracking-tight">Structured Knowledge</h2>
							<p class="text-sm text-slate-500 font-normal">Review the knowledge concepts and deterministic answer patterns inferred from your notes.</p>
						</div>
						<button type="button" class="px-3 py-1.5 text-xs font-medium text-blue-600 hover:bg-blue-50 rounded-lg transition" onclick={onEditNotes}>
							Edit raw notes
						</button>
					</div>

					<div class="mb-2 mt-5 flex items-center gap-2 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
						<h3 class="text-sm font-medium text-slate-800">Knowledge concepts</h3>
						<span class="rounded-md bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-500">{concepts.length}</span>
					</div>
					<div class="grid grid-cols-1 sm:grid-cols-2 gap-3 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
						{#each concepts as concept (concept.id)}
							<div class="p-4 bg-slate-50/70 border rounded-xl space-y-2 {concept.approved ? 'border-slate-200' : 'border-slate-200 opacity-60'}">
								<div class="flex items-center gap-2">
									<input type="checkbox" bind:checked={concept.approved} aria-label={`Include ${concept.title || 'knowledge concept'}`} class="rounded border-slate-300 text-blue-600" />
									<input bind:value={concept.title} aria-label="Concept title" class="min-w-0 flex-1 bg-white border border-slate-200 rounded-lg px-2 py-1 text-xs font-medium text-slate-900 outline-none focus:border-blue-500" />
									<input bind:value={concept.type} aria-label="Concept type" class="w-20 bg-blue-50 border border-blue-100 rounded-lg px-2 py-1 text-[10px] font-medium text-blue-700 outline-none focus:border-blue-500" />
								</div>
								<textarea bind:value={concept.body_markdown} aria-label="Concept content" rows="4" class="w-full resize-y bg-white border border-slate-200 rounded-lg p-2 text-xs text-slate-600 font-normal outline-none focus:border-blue-500"></textarea>
							</div>
						{/each}
					</div>

					<div class="mb-2 mt-6 flex items-center gap-2 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
						<h3 class="text-sm font-medium text-slate-800">Deterministic answer patterns</h3>
						<span class="rounded-md bg-blue-100 px-1.5 py-0.5 text-[10px] text-blue-700">{patterns.length}</span>
					</div>
					<div class="grid grid-cols-1 gap-3 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
						{#each patterns as pattern (pattern.id)}
							<div class="p-4 bg-blue-50/40 border rounded-xl space-y-2 {pattern.approved ? 'border-blue-200' : 'border-slate-200 opacity-60'}">
								<div class="flex items-center gap-2">
									<input type="checkbox" bind:checked={pattern.approved} aria-label={`Include pattern ${pattern.canonical_question || 'answer pattern'}`} class="rounded border-slate-300 text-blue-600" />
									<input bind:value={pattern.canonical_question} aria-label="Canonical question" class="min-w-0 flex-1 bg-white border border-slate-200 rounded-lg px-2 py-1 text-xs font-medium text-slate-900 outline-none focus:border-blue-500" />
								</div>
								<textarea bind:value={pattern.answer_markdown} aria-label="Pattern answer" rows="3" class="w-full resize-y bg-white border border-slate-200 rounded-lg p-2 text-xs text-slate-600 outline-none focus:border-blue-500"></textarea>
								<label class="block text-[10px] font-medium uppercase tracking-wide text-slate-400" for={`triggers-${pattern.id}`}>Trigger phrases, one per line</label>
								<textarea id={`triggers-${pattern.id}`} value={pattern.trigger_phrases.join('\n')} oninput={(event) => updateTriggers(pattern, event.currentTarget.value)} rows="3" class="w-full resize-y bg-white border border-slate-200 rounded-lg p-2 text-[11px] text-slate-600 outline-none focus:border-blue-500"></textarea>
							</div>
						{/each}
					</div>
				{/if}
