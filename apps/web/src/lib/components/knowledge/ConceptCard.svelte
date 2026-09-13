<script lang="ts">
	import { PencilSquareIcon, TrashIcon } from '@fvilers/heroicons-svelte/24/outline';
	import { typeColor, typeLabel } from '$lib/knowledge/ingestion';

	export interface ConceptItem {
		id: string;
		title: string;
		type: string;
		tags?: string[];
		body_text: string;
		source?: string;
		created_at?: string;
	}

	let {
		concept,
		expanded = false,
		showActions = true,
		onToggleExpand,
		onEdit,
		onDelete
	}: {
		concept: ConceptItem;
		expanded?: boolean;
		showActions?: boolean;
		onToggleExpand?: () => void;
		onEdit?: () => void;
		onDelete?: () => void;
	} = $props();

	function formatDate(iso?: string | null) {
		if (!iso) return 'Recently';
		return new Date(iso).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
	}
</script>

<div class="border border-slate-200/80 hover:border-slate-300 rounded-2xl bg-white p-4 transition shadow-2xs space-y-2.5 flex flex-col justify-between">
	<div class="space-y-2.5">
		<div class="flex items-start justify-between gap-2">
			<div class="flex flex-wrap items-center gap-1.5 min-w-0">
				<span class="px-2 py-0.5 rounded-md text-[10px] font-semibold border capitalize {typeColor(concept.type)}">
					{typeLabel(concept.type)}
				</span>
				<h3 class="text-sm font-semibold text-slate-900 leading-snug">{concept.title}</h3>
				{#if concept.source === 'owner_pasted'}
					<span class="text-[10px] text-slate-400 bg-slate-100 px-1.5 py-0.5 rounded">pasted</span>
				{/if}
			</div>

			{#if showActions && (onEdit || onDelete)}
				<div class="flex items-center gap-0.5 shrink-0">
					{#if onEdit}
						<button
							type="button"
							onclick={onEdit}
							class="flex items-center gap-1 px-2 py-1 rounded-lg text-xs font-medium text-slate-500 hover:text-blue-600 hover:bg-blue-50/70 transition cursor-pointer"
							title="Edit concept"
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
							title="Delete concept"
						>
							<TrashIcon class="w-3.5 h-3.5" />
							<span>Delete</span>
						</button>
					{/if}
				</div>
			{/if}
		</div>

		{#if concept.tags?.length}
			<div class="flex flex-wrap items-center gap-1">
				{#each concept.tags as tag}
					<span class="text-[10px] text-slate-500 bg-slate-100/80 px-2 py-0.5 rounded-md font-medium">{tag}</span>
				{/each}
			</div>
		{/if}

		<div class="text-xs text-slate-600 leading-relaxed whitespace-pre-wrap {expanded ? '' : 'line-clamp-4'}">
			{concept.body_text}
		</div>

		{#if (concept.body_text || '').length > 180 && onToggleExpand}
			<button
				type="button"
				onclick={onToggleExpand}
				class="text-[11px] font-medium text-blue-600 hover:text-blue-700 cursor-pointer pt-0.5 inline-block"
			>
				{expanded ? 'Show less' : 'Show full content'}
			</button>
		{/if}
	</div>

	{#if concept.created_at}
		<div class="pt-2 mt-1 text-[10px] text-slate-400 border-t border-slate-100/80">
			<span>Added {formatDate(concept.created_at)}</span>
		</div>
	{/if}
</div>
