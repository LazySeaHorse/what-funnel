<script lang="ts">
	import { Button } from '$lib/components/ui';
	import { typeColor, typeLabel } from '$lib/knowledge/ingestion';

	export interface SuggestionItem {
		id: string;
		type?: string;
		confidence?: number;
		_payload?: {
			title?: string;
			canonical_question?: string;
			body_text?: string;
			answer_text?: string;
			type?: string;
		};
	}

	interface Props {
		suggestion: SuggestionItem;
		onApprove?: () => void;
		onReject?: () => void;
	}

	let { suggestion, onApprove, onReject }: Props = $props();
</script>

<div class="p-4 rounded-2xl border border-slate-200/80 bg-white space-y-3 shadow-2xs">
	<div class="flex items-center justify-between gap-2">
		<div class="flex items-center gap-2 min-w-0">
			<span class="px-2 py-0.5 rounded text-[10px] font-medium border capitalize {typeColor(suggestion._payload?.type ?? suggestion.type)}">
				{typeLabel(suggestion._payload?.type ?? suggestion.type)}
			</span>
			<h3 class="text-sm font-medium text-slate-900 truncate">
				{suggestion._payload?.title ?? suggestion._payload?.canonical_question ?? 'Untitled suggestion'}
			</h3>
		</div>
		<span class="px-2 py-0.5 rounded-md text-[10px] font-medium bg-blue-50 text-blue-700 border border-blue-100 shrink-0">
			{Math.round((suggestion.confidence ?? 0) * 100)}% match
		</span>
	</div>
	<div class="text-xs text-slate-700 bg-slate-50/80 p-3 rounded-xl leading-relaxed whitespace-pre-wrap border border-slate-200/70">
		{suggestion._payload?.body_text ?? suggestion._payload?.answer_text ?? ''}
	</div>
	<div class="flex items-center justify-end gap-2 pt-1 text-xs">
		{#if onReject}
			<Button variant="ghost" size="sm" onclick={onReject}>
				Dismiss
			</Button>
		{/if}
		{#if onApprove}
			<Button variant="primary" size="sm" onclick={onApprove}>
				Add to Knowledge Base
			</Button>
		{/if}
	</div>
</div>
