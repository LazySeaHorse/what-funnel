<script lang="ts">
	import { PencilSquareIcon, TrashIcon } from '@fvilers/heroicons-svelte/24/outline';

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

<div class="p-4 rounded-2xl border border-slate-200/80 hover:border-slate-300 bg-white space-y-2.5 transition shadow-2xs">
	<div class="flex items-start justify-between gap-2">
		<h3 class="text-sm font-medium text-slate-900 leading-snug">{pattern.canonical_question}</h3>
		{#if showActions && (onEdit || onDelete)}
			<div class="flex items-center gap-0.5 shrink-0">
				{#if onEdit}
					<button
						type="button"
						onclick={onEdit}
						class="flex items-center gap-1 px-2 py-1 rounded-lg text-xs font-medium text-slate-500 hover:text-blue-600 hover:bg-blue-50/70 transition cursor-pointer"
						title="Edit pattern"
					>
						<PencilSquareIcon class="w-3.5 h-3.5" />
						<span>Edit</span>
					</button>
				{/if}
				{#if onDelete}
					<button
						type="button"
						onclick={onDelete}
						class="flex items-center gap-1 px-1.5 py-1 rounded-lg text-xs font-medium text-slate-400 hover:text-rose-600 hover:bg-rose-50/70 transition cursor-pointer"
						title="Delete pattern"
					>
						<TrashIcon class="w-3.5 h-3.5" />
						<span>Delete</span>
					</button>
				{/if}
			</div>
		{/if}
	</div>

	{#if pattern.trigger_phrases?.length}
		<div class="flex flex-wrap items-center gap-1 pt-0.5">
			<span class="text-[10px] font-medium uppercase tracking-wider text-slate-400 mr-0.5">Triggers:</span>
			{#each pattern.trigger_phrases as phrase}
				<span class="text-xs text-slate-700 bg-slate-100/90 border border-slate-200/50 px-2 py-0.5 rounded-lg">{phrase}</span>
			{/each}
		</div>
	{/if}

	<div class="text-xs text-slate-700 leading-relaxed whitespace-pre-wrap bg-slate-50/80 p-3 rounded-xl border border-slate-200/70">
		{pattern.answer_text}
	</div>
</div>
