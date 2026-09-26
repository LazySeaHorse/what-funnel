<script lang="ts">
	import { CheckIcon, SparklesIcon, XMarkIcon } from '@fvilers/heroicons-svelte/24/outline';
	import { Button } from '$lib/components/ui';

	export interface KnowledgeTemplate {
		label: string;
		buttonLabel: string;
		content: string;
	}

	const DEFAULT_TEMPLATES: KnowledgeTemplate[] = [
		{ label: 'Services & Pricing', buttonLabel: 'Pricing', content: '- Standard consultation: $150/hr\n- Monthly retainer: $2,500/mo includes 20 hours and priority SLA' },
		{ label: 'Business Hours', buttonLabel: 'Business Hours', content: '- Monday–Friday: 9:00 AM – 6:00 PM EST\n- Saturday–Sunday: Closed (urgent incident line available)' },
		{ label: 'Cancellation Policy', buttonLabel: 'Cancellation Policy', content: '- 24-hour advance notice required for full refund\n- Subscriptions can be cancelled anytime before renewal' },
		{ label: 'Frequently Asked Questions', buttonLabel: 'FAQs', content: '- How do I book an onboarding session? Book directly via calendar link after sign-up\n- Do you offer custom integrations? Yes, on enterprise plans' }
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
		showTemplates = true,
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

<div class="bg-gradient-to-b from-slate-50/80 to-white border border-slate-200/80 rounded-2xl p-4 sm:p-5 shadow-2xs transition-all w-full space-y-3">
	<!-- Header with Title, Badge, and Guidance -->
	<div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-1.5">
		<div>
			<div class="flex items-center gap-2">
				<h2 class="text-xs font-semibold text-slate-800 uppercase tracking-wider">{title}</h2>
				<div class="inline-flex items-center gap-1 text-[11px] font-medium text-blue-700 bg-blue-50 border border-blue-200/60 px-2 py-0.5 rounded-full">
					<SparklesIcon class="w-3 h-3 text-blue-600" />
					<span>{badge}</span>
				</div>
			</div>
			<p class="text-xs text-slate-500 mt-0.5">
				Paste raw website copy, FAQs, or service policies. AI structures it into searchable concepts and instant replies.
			</p>
		</div>
	</div>

	<!-- Starter Template Chips -->
	{#if showTemplates}
		<div class="flex flex-wrap items-center gap-1.5 pt-0.5">
			<span class="text-[11px] font-medium text-slate-400 mr-0.5">Quick starters:</span>
			{#each activeTemplates as t}
				{@const isAdded = value.includes(t.label)}
				<button
					type="button"
					disabled={isAdded || busy}
					onclick={() => appendTemplate(t.label, t.content)}
					class="inline-flex items-center gap-1 px-2.5 py-1 text-[11px] font-medium rounded-lg border transition-all cursor-pointer {isAdded ? 'bg-slate-100 text-slate-400 border-slate-200/60 cursor-not-allowed' : 'bg-white hover:bg-slate-50 border-slate-200 text-slate-700 hover:text-slate-900 shadow-2xs hover:border-slate-300 active:scale-[0.98]'}"
				>
					<span class="text-slate-400">{isAdded ? '✓' : '+'}</span>
					<span>{t.buttonLabel}</span>
				</button>
			{/each}
			{#if value.trim().length > 0}
				<button
					type="button"
					onclick={() => (value = '')}
					disabled={busy}
					class="text-[11px] text-slate-400 hover:text-slate-600 underline ml-auto cursor-pointer"
				>
					Clear text
				</button>
			{/if}
		</div>
	{/if}

	<!-- Textarea input -->
	<textarea
		bind:value
		disabled={busy}
		{placeholder}
		class="w-full h-28 sm:h-32 p-3.5 text-xs sm:text-sm text-slate-800 placeholder-slate-400 bg-white rounded-xl border border-slate-200 focus:outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 resize-none leading-relaxed disabled:opacity-60 transition shadow-2xs"
	></textarea>

	<!-- Footer with Action & Status -->
	<div class="flex flex-col sm:flex-row sm:items-center sm:justify-between pt-0.5 gap-2.5">
		<div class="flex items-center gap-2 min-w-0">
			{#if result?.added !== undefined}
				<div class="flex items-center gap-1.5 text-xs text-emerald-700 font-medium bg-emerald-50 border border-emerald-200/70 px-2.5 py-1 rounded-lg truncate">
					<CheckIcon class="w-4 h-4 shrink-0 text-emerald-600" />
					<span>{result.added} concept{result.added !== 1 ? 's' : ''} and {result.patternsAdded ?? 0} pattern{result.patternsAdded !== 1 ? 's' : ''} added</span>
				</div>
			{:else if result?.error}
				<div class="flex items-center gap-1.5 text-xs text-rose-700 font-medium bg-rose-50 border border-rose-200/70 px-2.5 py-1 rounded-lg truncate">
					<XMarkIcon class="w-4 h-4 shrink-0 text-rose-600" />
					<span>{result.error}</span>
				</div>
			{:else if phase === 'processing'}
				<div class="flex items-center gap-2 text-xs text-blue-700 font-medium bg-blue-50 border border-blue-200/70 px-2.5 py-1 rounded-lg">
					<span class="w-3.5 h-3.5 border-2 border-blue-600 border-t-transparent rounded-full animate-spin shrink-0"></span>
					<span>Extracting concepts and answer patterns…</span>
				</div>
			{:else if phase === 'publishing'}
				<div class="flex items-center gap-2 text-xs text-blue-700 font-medium bg-blue-50 border border-blue-200/70 px-2.5 py-1 rounded-lg">
					<span class="w-3.5 h-3.5 border-2 border-blue-600 border-t-transparent rounded-full animate-spin shrink-0"></span>
					<span>Publishing reviewed knowledge…</span>
				</div>
			{:else}
				<span class="text-[11px] text-slate-400">
					{value.trim() ? `${value.trim().split(/\s+/).length} words ready for extraction` : 'Tip: Include specific numbers, hours, and prices'}
				</span>
			{/if}
		</div>

		{#if showSubmitButton && onSubmit}
			<Button
				variant="primary"
				size="sm"
				onclick={onSubmit}
				disabled={busy || !value.trim()}
				busy={busy}
				class="self-end sm:self-auto shadow-xs"
			>
				<SparklesIcon class="w-3.5 h-3.5 text-white" />
				<span>{buttonText}</span>
			</Button>
		{/if}
	</div>
</div>
