<script lang="ts">
	import { Button } from '$lib/components/ui';
	import { apiRequest } from '$lib/api';
	import type { PatternItem } from '$lib/components/knowledge/PatternCard.svelte';

	interface Props {
		pattern: PatternItem;
		onSave?: (updated: PatternItem) => void;
		onCancel?: () => void;
	}

	let { pattern, onSave, onCancel }: Props = $props();

	// svelte-ignore state_referenced_locally
	let draft = $state<{ canonical_question: string; answer_text: string; trigger_phrases: string[] }>({
		canonical_question: pattern.canonical_question || '',
		answer_text: pattern.answer_text || '',
		trigger_phrases: Array.isArray(pattern.trigger_phrases) ? [...pattern.trigger_phrases] : []
	});
	let triggerInput = $state('');
	let saving = $state(false);
	let saveError = $state('');

	function addTrigger() {
		const val = triggerInput.trim();
		if (!val) return;
		if (!draft.trigger_phrases.includes(val)) {
			draft.trigger_phrases = [...draft.trigger_phrases, val];
		}
		triggerInput = '';
	}

	function removeTrigger(phrase: string) {
		draft.trigger_phrases = draft.trigger_phrases.filter((t) => t !== phrase);
	}

	async function handleSave() {
		if (!draft.canonical_question.trim()) {
			saveError = 'Canonical question is required';
			return;
		}
		if (!draft.answer_text.trim()) {
			saveError = 'Answer text is required';
			return;
		}
		saving = true;
		saveError = '';
		try {
			const res = await apiRequest(`/api/kb/patterns/${pattern.id}`, {
				method: 'PUT',
				body: draft
			});
			const updated: PatternItem = res.pattern || { ...draft, id: pattern.id };
			onSave?.(updated);
		} catch (err: any) {
			saveError = err.message || 'Failed to save pattern changes';
		} finally {
			saving = false;
		}
	}
</script>

<div class="border border-blue-400/80 rounded-2xl p-5 bg-white shadow-md ring-2 ring-blue-500/10 space-y-4 transition-all">
	<div class="flex items-center justify-between gap-2 border-b border-slate-100 pb-3">
		<div class="flex items-center gap-2">
			<span class="text-xs font-semibold text-slate-900 uppercase tracking-wider">Edit Answer Pattern</span>
			<span class="text-[10px] text-sky-700 bg-sky-50 border border-sky-200/70 px-2 py-0.5 rounded-md font-medium">Deterministic Q&A</span>
		</div>
		{#if saveError}
			<span class="text-xs text-rose-600 font-medium bg-rose-50 px-2.5 py-0.5 rounded-md border border-rose-200">{saveError}</span>
		{/if}
	</div>
	<div>
		<label for={`edit-pattern-question-${pattern.id}`} class="block text-[11px] font-semibold text-slate-600 mb-1">Canonical Question</label>
		<input
			id={`edit-pattern-question-${pattern.id}`}
			bind:value={draft.canonical_question}
			placeholder="Canonical question"
			class="w-full bg-white border border-slate-200 rounded-xl px-3 py-2 text-xs font-medium text-slate-900 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 transition shadow-2xs"
		/>
	</div>
	<div>
		<label for={`edit-pattern-triggers-input-${pattern.id}`} class="block text-[11px] font-semibold text-slate-600 mb-1">Trigger Phrases</label>
		<div class="flex flex-wrap items-center gap-1.5 mb-2">
			{#each draft.trigger_phrases as phrase}
				<span class="inline-flex items-center gap-1.5 text-[11px] text-slate-700 bg-slate-50 border border-slate-200/80 px-2.5 py-1 rounded-lg font-medium shadow-2xs">
					<span>{phrase}</span>
					<button type="button" onclick={() => removeTrigger(phrase)} class="text-slate-400 hover:text-rose-600 cursor-pointer font-bold">×</button>
				</span>
			{/each}
		</div>
		<div class="flex items-center gap-2">
			<input
				id={`edit-pattern-triggers-input-${pattern.id}`}
				bind:value={triggerInput}
				onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), addTrigger())}
				placeholder="Add trigger phrase and press Enter"
				class="flex-1 min-w-0 bg-white border border-slate-200 rounded-xl px-3 py-1.5 text-xs text-slate-700 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 transition shadow-2xs"
			/>
			<Button variant="secondary" size="xs" onclick={addTrigger} class="shadow-2xs">Add</Button>
		</div>
	</div>
	<div>
		<label for={`edit-pattern-answer-${pattern.id}`} class="block text-[11px] font-semibold text-slate-600 mb-1">Deterministic Answer</label>
		<textarea
			id={`edit-pattern-answer-${pattern.id}`}
			bind:value={draft.answer_text}
			rows="3"
			class="w-full bg-white border border-slate-200 rounded-xl p-3 text-xs text-slate-700 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 leading-relaxed transition shadow-2xs"
		></textarea>
	</div>
	<div class="flex items-center justify-end gap-2 pt-3 border-t border-slate-100">
		<Button variant="ghost" size="sm" onclick={onCancel} class="text-slate-600 hover:text-slate-800">
			Cancel
		</Button>
		<Button
			variant="primary"
			size="sm"
			onclick={handleSave}
			disabled={saving}
			busy={saving}
			class="shadow-xs"
		>
			Save changes
		</Button>
	</div>
</div>
