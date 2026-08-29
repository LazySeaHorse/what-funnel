<script lang="ts">
	import Icon from '$lib/Icon.svelte';

	type Concept = { id?: string; title: string; type?: string; category?: string; tags?: string[]; body_markdown?: string; content?: string };

	let {
		step,
		totalSteps,
		rawText = $bindable(),
		status,
		concepts,
		compiling,
		onSkipToDashboard
	}: {
		step: number;
		totalSteps: number;
		rawText: string;
		status: 'input' | 'processing' | 'results';
		concepts: Concept[];
		compiling: boolean;
		onSkipToDashboard: () => void;
	} = $props();

	function appendTemplateChunk(label: string, text: string) {
		if (rawText.includes(label)) return;
		rawText = rawText.trim() + `\n\n${label}:\n${text}`;
	}
</script>

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

				{:else if status === 'processing'}
					<div class="py-12 flex flex-col items-center justify-center text-center space-y-4 w-full">
						<div class="w-12 h-12 rounded-2xl bg-blue-50 border border-blue-100 flex items-center justify-center text-blue-600">
							<Icon name="sparkles" size={24} color="currentColor" />
						</div>
						<h2 class="text-xl font-medium text-slate-900">Organizing your knowledge...</h2>
						<p class="text-xs sm:text-sm text-slate-500 max-w-sm lg:max-w-none font-normal">
							Structuring raw business notes into categorized concepts and FAQ patterns.
						</p>

						<button
							type="button"
							class="mt-4 px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-700 text-xs font-medium rounded-xl transition"
							onclick={onSkipToDashboard}
						>
							Skip waiting & go to Dashboard →
						</button>
					</div>

				{:else if status === 'results'}
					<div class="flex items-center justify-between mb-4 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
						<div>
							<h2 class="text-2xl font-medium text-slate-900 tracking-tight">Structured Knowledge</h2>
							<p class="text-sm text-slate-500 font-normal">Concepts inferred from your business notes:</p>
						</div>
						<button type="button" class="px-3 py-1.5 text-xs font-medium text-blue-600 hover:bg-blue-50 rounded-lg transition" onclick={() => { /* handled by parent via status binding */ }}>
							Edit raw notes
						</button>
					</div>

					<div class="grid grid-cols-1 sm:grid-cols-2 gap-3 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
						{#each concepts as concept}
							<div class="p-4 bg-slate-50/70 border border-slate-200 rounded-xl space-y-2">
								<div class="flex items-center justify-between">
									<span class="text-xs font-medium text-slate-900">{concept.title || 'Knowledge Concept'}</span>
									<span class="px-2 py-0.5 rounded-md bg-blue-100 text-blue-700 text-[10px] font-medium">{concept.category || concept.type || 'Rule'}</span>
								</div>
								<p class="text-xs text-slate-600 line-clamp-3 font-normal">{concept.body_markdown || concept.content || ''}</p>
							</div>
						{/each}
					</div>
				{/if}
