<script lang="ts">
	import { PencilSquareIcon, TrashIcon } from '@fvilers/heroicons-svelte/24/outline';
	import { Button } from '$lib/components/ui';

	export interface PatternItem {
		id: string;
		canonical_question: string;
		answer_text: string;
		trigger_phrases?: string[];
	}

	let {
		pattern,
		showActions = true,
		onEdit,
		onDelete
	}: {
		pattern: PatternItem;
		showActions?: boolean;
		onEdit?: () => void;
		onDelete?: () => void;
	} = $props();
</script>

<div class="group relative p-5 rounded-2xl border border-slate-200/80 hover:border-slate-300 bg-white space-y-3.5 transition-all duration-150 shadow-2xs hover:shadow-xs flex flex-col justify-between">
	<div class="space-y-3">
		<!-- Header: Badge & Actions -->
		<div class="flex items-start justify-between gap-3">
			<div class="space-y-1 min-w-0 flex-1">
				<div class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md text-[10px] font-semibold bg-sky-50 text-sky-700 border border-sky-200/70 tracking-wide uppercase">
					<span>Deterministic Q&A</span>
				</div>
				<h3 class="text-sm font-semibold text-slate-900 leading-snug tracking-tight pt-0.5">{pattern.canonical_question}</h3>
			</div>

			{#if showActions && (onEdit || onDelete)}
				<div class="flex items-center gap-1 shrink-0 -mr-1 -mt-0.5">
					{#if onEdit}
						<Button
							variant="ghost"
							size="xs"
							onclick={onEdit}
							class="text-slate-500 hover:text-blue-600 hover:bg-blue-50/80 transition-colors"
							title="Edit pattern"
						>
							<PencilSquareIcon class="w-3.5 h-3.5" />
							<span>Edit</span>
						</Button>
					{/if}
					{#if onDelete}
						<Button
							variant="ghost"
							size="xs"
							onclick={onDelete}
							class="text-slate-400 hover:text-rose-600 hover:bg-rose-50/80 transition-colors"
							title="Delete pattern"
						>
							<TrashIcon class="w-3.5 h-3.5" />
							<span>Delete</span>
						</Button>
					{/if}
				</div>
			{/if}
		</div>

		<!-- Trigger Phrases -->
		{#if pattern.trigger_phrases?.length}
			<div class="flex flex-wrap items-center gap-1.5 pt-0.5">
				<span class="text-[10px] font-semibold uppercase tracking-wider text-slate-400 mr-0.5">Triggers:</span>
				{#each pattern.trigger_phrases as phrase}
					<span class="inline-flex items-center text-xs text-slate-700 bg-slate-50 border border-slate-200/70 px-2 py-0.5 rounded-lg font-medium">
						{phrase}
					</span>
				{/each}
			</div>
		{/if}

		<!-- Deterministic Answer Box -->
		<div class="rounded-xl border border-slate-200/70 bg-slate-50/60 p-3.5 space-y-1.5">
			<div class="text-[10px] font-semibold uppercase tracking-wider text-slate-400">
				Guaranteed response:
			</div>
			<div class="text-xs text-slate-700 leading-relaxed whitespace-pre-wrap">
				{pattern.answer_text}
			</div>
		</div>
	</div>
</div>
