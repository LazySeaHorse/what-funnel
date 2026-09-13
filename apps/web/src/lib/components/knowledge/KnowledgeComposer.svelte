<script lang="ts">
	import { CheckIcon, SparklesIcon, XMarkIcon } from '@fvilers/heroicons-svelte/24/outline';

	export interface KnowledgeTemplate {
		label: string;
		buttonLabel: string;
		content: string;
	}

	const DEFAULT_TEMPLATES: KnowledgeTemplate[] = [
		{ label: 'Services & Pricing', buttonLabel: 'Pricing', content: '- Standard service: $50\n- Premium package: $120' },
		{ label: 'Business Hours', buttonLabel: 'Hours', content: '- Monday–Friday: 9:00 AM – 6:00 PM\n- Saturday: 10:00 AM – 4:00 PM' },
		{ label: 'Cancellation Policy', buttonLabel: 'Policy', content: '- 24-hour advance notice required' },
		{ label: 'FAQs', buttonLabel: 'FAQs', content: '- Free customer parking on-site\n- Walk-ins accepted based on availability' }
	];

	let {
		value = $bindable(''),
		title = 'Add business knowledge',
		badge = 'AI-powered extraction',
		placeholder = 'Paste business information, pricing, business hours, and policies. The system extracts concepts and answer patterns.',
		busy = false,
		phase = 'idle',
		result = null,
		templates = [],
		showTemplates = false,
		buttonText = 'Extract with AI',
		showSubmitButton = true,
		onSubmit
	}: {
		value: string;
		title?: string;
		badge?: string;
		placeholder?: string;
		busy?: boolean;
		phase?: 'idle' | 'processing' | 'review' | 'publishing';
		result?: { added?: number; patternsAdded?: number; error?: string } | null;
		templates?: KnowledgeTemplate[];
		showTemplates?: boolean;
		buttonText?: string;
		showSubmitButton?: boolean;
		onSubmit?: () => void;
	} = $props();

	const activeTemplates = $derived(templates.length > 0 ? templates : DEFAULT_TEMPLATES);

	function appendTemplate(label: string, text: string) {
		if (value.includes(label)) return;
		const trimmed = value.trim();
		value = trimmed ? `${trimmed}\n\n${label}:\n${text}` : `${label}:\n${text}`;
	}
</script>

<div class="bg-slate-50/70 border border-slate-200/80 rounded-2xl p-4 shadow-2xs transition-all w-full">
	<div class="flex items-center justify-between mb-2">
		<h2 class="text-xs font-medium text-slate-700 uppercase tracking-wider">{title}</h2>
		<div class="flex items-center gap-1.5 text-[11px] font-medium text-slate-500 bg-white border border-slate-200/70 px-2 py-0.5 rounded-lg shadow-2xs">
			<SparklesIcon class="w-3 h-3 text-blue-600" />
			<span>{badge}</span>
		</div>
	</div>

	{#if showTemplates}
		<div class="flex flex-wrap items-center gap-1.5 mb-2.5">
			<span class="text-[11px] font-medium text-slate-500">Templates:</span>
			{#each activeTemplates as t}
				{@const isAdded = value.includes(t.label)}
				<button
					type="button"
					disabled={isAdded || busy}
					onclick={() => appendTemplate(t.label, t.content)}
					class="inline-flex items-center gap-1 px-2 py-0.5 text-[11px] font-medium rounded-lg border transition cursor-pointer {isAdded ? 'bg-slate-100/60 text-slate-400 border-slate-200/50 cursor-not-allowed' : 'bg-white hover:bg-slate-100 border-slate-200 text-slate-700 hover:text-slate-900 shadow-2xs active:scale-[0.98]'}"
				>
					<span class="text-slate-400">{isAdded ? '✓' : '+'}</span>
					<span>{t.buttonLabel}</span>
				</button>
			{/each}
		</div>
	{/if}

	<textarea
		bind:value
		disabled={busy}
		{placeholder}
		class="w-full h-28 sm:h-32 p-3.5 text-xs sm:text-sm text-slate-700 placeholder-slate-400 bg-white rounded-xl border border-slate-200 focus:outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 resize-none leading-relaxed disabled:opacity-60 transition"
	></textarea>

	<div class="flex items-center justify-between mt-2.5 gap-2">
		<div class="flex items-center gap-2 min-w-0">
			{#if result?.added !== undefined}
				<div class="flex items-center gap-1.5 text-xs text-emerald-600 font-medium truncate">
					<CheckIcon class="w-4 h-4 shrink-0" />
					<span>{result.added} concept{result.added !== 1 ? 's' : ''} and {result.patternsAdded ?? 0} pattern{result.patternsAdded !== 1 ? 's' : ''} added</span>
				</div>
			{:else if result?.error}
				<div class="flex items-center gap-1.5 text-xs text-rose-600 font-medium truncate">
					<XMarkIcon class="w-4 h-4 shrink-0" />
					<span>{result.error}</span>
				</div>
			{:else if phase === 'processing'}
				<div class="flex items-center gap-1.5 text-xs text-blue-600 font-medium">
					<span class="w-3.5 h-3.5 border-2 border-blue-600 border-t-transparent rounded-full animate-spin shrink-0"></span>
					<span>Extracting concepts and answer patterns…</span>
				</div>
			{:else if phase === 'publishing'}
				<div class="flex items-center gap-1.5 text-xs text-blue-600 font-medium">
					<span class="w-3.5 h-3.5 border-2 border-blue-600 border-t-transparent rounded-full animate-spin shrink-0"></span>
					<span>Publishing reviewed knowledge…</span>
				</div>
			{/if}
		</div>

		{#if showSubmitButton && onSubmit}
			<button
				type="button"
				onclick={onSubmit}
				disabled={busy || !value.trim()}
				class="flex items-center gap-1.5 px-3.5 py-1.5 rounded-xl bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium transition disabled:opacity-50 cursor-pointer shadow-xs active:scale-[0.98] shrink-0"
			>
				<SparklesIcon class="w-3.5 h-3.5 text-white" />
				<span>{busy ? 'Processing…' : buttonText}</span>
			</button>
		{/if}
	</div>
</div>
