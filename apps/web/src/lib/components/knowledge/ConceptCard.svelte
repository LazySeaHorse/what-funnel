<script lang="ts">
	import { PencilSquareIcon, TrashIcon } from '@fvilers/heroicons-svelte/24/outline';
	import { Button } from '$lib/components/ui';
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

<div class="group relative border border-slate-200/80 hover:border-slate-300 rounded-2xl bg-white p-5 transition-all duration-150 shadow-2xs hover:shadow-xs flex flex-col justify-between space-y-3">
	<div class="space-y-3">
		<!-- Header: Type, Title, Source & Actions -->
		<div class="flex items-start justify-between gap-3">
			<div class="space-y-1.5 min-w-0 flex-1">
				<div class="flex flex-wrap items-center gap-1.5">
					<span class="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold tracking-wide border capitalize {typeColor(concept.type)}">
						{typeLabel(concept.type)}
					</span>
					{#if concept.source === 'owner_pasted'}
						<span class="inline-flex items-center gap-1 text-[10px] text-slate-500 bg-slate-100/90 border border-slate-200/60 px-1.5 py-0.5 rounded-md font-medium">
							<span>Pasted note</span>
						</span>
					{/if}
				</div>
				<h3 class="text-sm font-semibold text-slate-900 leading-snug tracking-tight">{concept.title}</h3>
			</div>

			{#if showActions && (onEdit || onDelete)}
				<div class="flex items-center gap-1 shrink-0 -mr-1 -mt-0.5">
					{#if onEdit}
						<Button
							variant="ghost"
							size="xs"
							onclick={onEdit}
							class="text-slate-500 hover:text-blue-600 hover:bg-blue-50/80 transition-colors"
							title="Edit concept"
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
							title="Delete concept"
						>
							<TrashIcon class="w-3.5 h-3.5" />
							<span>Delete</span>
						</Button>
					{/if}
				</div>
			{/if}
		</div>

		<!-- Tag Pills -->
		{#if concept.tags?.length}
			<div class="flex flex-wrap items-center gap-1.5 pt-0.5">
				{#each concept.tags as tag}
					<span class="inline-flex items-center text-[10px] text-slate-600 bg-slate-50 border border-slate-200/70 px-2 py-0.5 rounded-md font-medium">
						<span class="text-slate-400 mr-0.5 text-[9px]">#</span><span>{tag}</span>
					</span>
				{/each}
			</div>
		{/if}

		<!-- Body text with comfortable line-height -->
		<div class="text-xs text-slate-600 leading-relaxed whitespace-pre-wrap {expanded ? '' : 'line-clamp-4'}">
			{concept.body_text}
		</div>

		{#if (concept.body_text || '').length > 180 && onToggleExpand}
			<button
				type="button"
				onclick={onToggleExpand}
				class="text-[11px] font-medium text-blue-600 hover:text-blue-700 cursor-pointer pt-0.5 inline-flex items-center gap-1 hover:underline"
			>
				{expanded ? 'Show less' : 'Show full content'}
			</button>
		{/if}
	</div>

	<!-- Footer Metadata -->
	{#if concept.created_at}
		<div class="pt-2.5 mt-2 flex items-center justify-between text-[11px] text-slate-400 border-t border-slate-100">
			<span>Added {formatDate(concept.created_at)}</span>
		</div>
	{/if}
</div>
