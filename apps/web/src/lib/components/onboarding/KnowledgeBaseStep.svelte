<script lang="ts">
	import { SparklesIcon, ArrowRightIcon } from '@fvilers/heroicons-svelte/24/outline';
	import KnowledgeComposer from '$lib/components/knowledge/KnowledgeComposer.svelte';
	import IngestionReview from '$lib/components/knowledge/IngestionReview.svelte';

	type Concept = { id: string; title: string; type: string; tags: string[]; body_text: string; approved: boolean };
	type Pattern = { id: string; canonical_question: string; answer_text: string; trigger_phrases: string[]; approved: boolean };

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
</script>

{#if errorMessage}
	<div role="alert" class="mb-4 w-full rounded-xl border border-rose-200 bg-rose-50 p-3 text-xs font-medium text-rose-700">{errorMessage}</div>
{/if}

{#if status === 'input'}
	<div class="text-center lg:text-left mb-6">
		<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {step} of {totalSteps}</div>
		<h2 class="text-2xl font-medium text-slate-900 tracking-tight mb-1">Add knowledge base sources</h2>
		<p class="text-sm text-slate-500 font-normal max-w-lg lg:max-w-none mx-auto lg:mx-0">Add notes, price lists, business hours, and policies. WhatFunnel extracts concepts and answer patterns automatically.</p>
	</div>

	<div class="w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
		<KnowledgeComposer
			bind:value={rawText}
			title="Add business knowledge"
			badge="AI-powered extraction"
			placeholder="Paste raw business info, services, pricing, business hours, cancellation rules, FAQ answers, or message templates..."
			showTemplates={true}
			showSubmitButton={false}
			busy={compiling}
			phase={status}
		/>
	</div>

{:else if status === 'processing' || status === 'publishing'}
	<div class="text-center lg:text-left mb-6">
		<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {step} of {totalSteps}</div>
		<h2 class="text-2xl font-medium text-slate-900 tracking-tight mb-1">Processing knowledge</h2>
		<p class="text-sm text-slate-500 font-normal max-w-lg lg:max-w-none mx-auto lg:mx-0">Analyzing and structuring notes into categorized knowledge concepts.</p>
	</div>

	<div class="w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
		<div class="bg-slate-50/70 border border-slate-200/80 rounded-2xl p-8 sm:p-12 shadow-2xs text-center flex flex-col items-center justify-center space-y-4">
			<div class="relative w-12 h-12 rounded-2xl bg-blue-50 border border-blue-100 flex items-center justify-center text-blue-600">
				<span class="absolute inset-0 rounded-2xl border-2 border-blue-400/30 animate-ping"></span>
				<SparklesIcon class="w-6 h-6" />
			</div>
			<div class="space-y-1">
				<h3 class="text-base font-semibold text-slate-900">{status === 'publishing' ? 'Publishing knowledge items…' : 'Compiling knowledge items…'}</h3>
				<p class="text-xs sm:text-sm text-slate-500 max-w-sm font-normal">
					{status === 'publishing' ? 'Creating searchable concepts for the knowledge base.' : 'Structuring notes into categorized knowledge concepts.'}
				</p>
			</div>

			{#if status === 'processing'}
				<button
					type="button"
					class="mt-2 inline-flex items-center gap-1.5 px-4 py-2 bg-white hover:bg-slate-50 text-slate-700 text-xs font-medium rounded-xl border border-slate-200 shadow-2xs transition cursor-pointer active:scale-[0.98]"
					onclick={onSkipWaiting}
				>
					<span>Skip waiting and go to next page</span>
					<ArrowRightIcon class="w-3.5 h-3.5" />
				</button>
			{/if}
		</div>
	</div>

{:else if status === 'results'}
	<div class="flex items-center justify-between mb-4 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
		<div>
			<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {step} of {totalSteps}</div>
			<h2 class="text-2xl font-medium text-slate-900 tracking-tight">Structured Knowledge</h2>
			<p class="text-sm text-slate-500 font-normal mt-0.5">Review the concepts and answer patterns extracted from your notes.</p>
		</div>
		<button type="button" class="px-3.5 py-1.5 text-xs font-medium text-blue-600 hover:bg-blue-50 rounded-xl transition cursor-pointer" onclick={onEditNotes}>
			Edit notes
		</button>
	</div>

	<div class="w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
		<IngestionReview bind:concepts bind:patterns />
	</div>
{/if}
