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

<div class="group relative p-5 rounded-2xl border border-slate-200/80 hover:border-slate-300 bg-white space-y-3.5 shadow-2xs hover:shadow-xs transition-all duration-150 flex flex-col justify-between">
	<div class="space-y-3">
		<div class="flex items-start justify-between gap-3">
			<div class="space-y-1 min-w-0 flex-1">
				<div class="flex items-center gap-1.5">
					<span class="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold border capitalize {typeColor(suggestion._payload?.type ?? suggestion.type)}">
						{typeLabel(suggestion._payload?.type ?? suggestion.type)}
					</span>
					<span class="px-2 py-0.5 rounded-md text-[10px] font-semibold bg-blue-50 text-blue-700 border border-blue-200/70">
						{Math.round((suggestion.confidence ?? 0) * 100)}% match
					</span>
				</div>
				<h3 class="text-sm font-semibold text-slate-900 leading-snug tracking-tight truncate pt-0.5">
					{suggestion._payload?.title ?? suggestion._payload?.canonical_question ?? 'Untitled suggestion'}
				</h3>
			</div>
		</div>

		<div class="text-xs text-slate-700 bg-slate-50/70 p-3.5 rounded-xl leading-relaxed whitespace-pre-wrap border border-slate-200/70">
			{suggestion._payload?.body_text ?? suggestion._payload?.answer_text ?? ''}
		</div>
	</div>

	<div class="flex items-center justify-end gap-2 pt-1 text-xs">
		{#if onReject}
			<Button variant="ghost" size="sm" onclick={onReject} class="text-slate-500 hover:text-slate-800">
				Dismiss
			</Button>
		{/if}
		{#if onApprove}
			<Button variant="primary" size="sm" onclick={onApprove} class="shadow-2xs">
				Add to Knowledge Base
			</Button>
		{/if}
	</div>
</div>
