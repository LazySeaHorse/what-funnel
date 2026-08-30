<script lang="ts">
	import Icon from '$lib/Icon.svelte';
	import IngestionReview from '$lib/components/knowledge/IngestionReview.svelte';

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

					<div class="w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
						<IngestionReview bind:concepts bind:patterns />
					</div>
				{/if}
