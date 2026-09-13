<script lang="ts">
	import { typeColor } from '$lib/knowledge/ingestion';

	type Concept = { id: string; title: string; type: string; tags: string[]; body_text: string; approved: boolean };
	type Pattern = { id: string; canonical_question: string; answer_text: string; trigger_phrases: string[]; approved: boolean };

	let { concepts = $bindable(), patterns = $bindable() }: { concepts: Concept[]; patterns: Pattern[] } = $props();

	let activeTab = $state<'concepts' | 'patterns'>('concepts');

	function updateTriggers(pattern: Pattern, value: string) {
		pattern.trigger_phrases = value.split('\n').map((phrase) => phrase.trim()).filter(Boolean);
	}
</script>

<!-- Sub-Tabs Header matching KnowledgeView layout -->
<div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-4 mt-2">
	<nav class="inline-flex p-1 bg-slate-100/90 rounded-xl border border-slate-200/60 self-start" aria-label="Review sections">
		<button
			type="button"
			onclick={() => (activeTab = 'concepts')}
			class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs transition-all cursor-pointer {activeTab === 'concepts' ? 'bg-white text-slate-900 shadow-2xs font-medium' : 'text-slate-500 hover:text-slate-800 font-normal'}"
		>
			<span>KB Concepts</span>
			<span class="px-1.5 py-0.5 rounded-md text-[10px] font-medium {activeTab === 'concepts' ? 'bg-slate-100 text-slate-800' : 'bg-slate-200/60 text-slate-500'}">
				{concepts.length}
			</span>
		</button>
		<button
			type="button"
			onclick={() => (activeTab = 'patterns')}
			class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs transition-all cursor-pointer {activeTab === 'patterns' ? 'bg-white text-slate-900 shadow-2xs font-medium' : 'text-slate-500 hover:text-slate-800 font-normal'}"
		>
			<span>Patterns</span>
			<span class="px-1.5 py-0.5 rounded-md text-[10px] font-medium {activeTab === 'patterns' ? 'bg-slate-100 text-slate-800' : 'bg-slate-200/60 text-slate-500'}">
				{patterns.length}
			</span>
		</button>
	</nav>
	<div class="text-xs text-slate-500 font-normal">
		{activeTab === 'concepts' ? 'Review & select concepts to include' : 'Review & select deterministic Q&A triggers'}
	</div>
</div>

<!-- Concepts Tab: 2-column Grid -->
<div class={activeTab === 'concepts' ? 'block' : 'hidden'}>
	{#if concepts.length === 0}
		<div class="py-8 text-center text-xs text-slate-400 font-normal">
			No concepts extracted.
		</div>
	{:else}
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-3.5 w-full items-start">
			{#each concepts as concept (concept.id)}
				<div class="p-4 bg-white border rounded-2xl space-y-3 transition-all {concept.approved ? 'border-slate-200/80 shadow-2xs hover:border-slate-300' : 'border-slate-200/50 bg-slate-50/50 opacity-60'}">
					<div class="flex items-center gap-2">
						<input type="checkbox" bind:checked={concept.approved} aria-label={`Include ${concept.title || 'knowledge concept'}`} class="w-4 h-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500 cursor-pointer transition shrink-0" />
						<input bind:value={concept.title} disabled={!concept.approved} aria-label="Concept title" placeholder="Concept title" class="min-w-0 flex-1 bg-white border border-slate-200 rounded-xl px-3 py-1.5 text-xs font-medium text-slate-900 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 disabled:bg-slate-100/70 disabled:text-slate-400 transition" />
						<input bind:value={concept.type} disabled={!concept.approved} aria-label="Concept type" placeholder="Type" class="w-24 border rounded-xl px-2.5 py-1.5 text-[11px] font-medium outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 disabled:opacity-50 transition text-center capitalize truncate {concept.approved ? typeColor(concept.type) : 'bg-slate-100 text-slate-400 border-slate-200'}" />
					</div>
					<textarea bind:value={concept.body_text} disabled={!concept.approved} aria-label="Concept content" rows="4" class="w-full resize-y bg-slate-50/50 focus:bg-white border border-slate-200/80 rounded-xl p-3 text-xs text-slate-600 font-normal outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 disabled:bg-slate-100/70 disabled:text-slate-400 leading-relaxed transition"></textarea>
				</div>
			{/each}
		</div>
	{/if}
</div>

<!-- Patterns Tab: 2-column Grid -->
<div class={activeTab === 'patterns' ? 'block' : 'hidden'}>
	{#if patterns.length === 0}
		<div class="py-8 text-center text-xs text-slate-400 font-normal">
			No answer patterns extracted.
		</div>
	{:else}
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-3.5 w-full items-start">
			{#each patterns as pattern (pattern.id)}
				<div class="p-4 bg-white border rounded-2xl space-y-3 transition-all {pattern.approved ? 'border-blue-200/80 bg-blue-50/10 shadow-2xs hover:border-blue-300' : 'border-slate-200/50 bg-slate-50/50 opacity-60'}">
					<div class="flex items-center gap-2">
						<input type="checkbox" bind:checked={pattern.approved} aria-label={`Include pattern ${pattern.canonical_question || 'answer pattern'}`} class="w-4 h-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500 cursor-pointer transition shrink-0" />
						<input bind:value={pattern.canonical_question} disabled={!pattern.approved} aria-label="Canonical question" placeholder="Canonical question" class="min-w-0 flex-1 bg-white border border-slate-200 rounded-xl px-3 py-1.5 text-xs font-medium text-slate-900 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 disabled:bg-slate-100/70 disabled:text-slate-400 transition" />
					</div>
					<textarea bind:value={pattern.answer_text} disabled={!pattern.approved} aria-label="Pattern answer" rows="3" class="w-full resize-y bg-slate-50/50 focus:bg-white border border-slate-200/80 rounded-xl p-3 text-xs text-slate-600 font-normal outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 disabled:bg-slate-100/70 disabled:text-slate-400 leading-relaxed transition"></textarea>
					<div class="space-y-1.5 pt-0.5">
						<label class="block text-[10px] font-medium uppercase tracking-wider text-slate-400" for={`triggers-${pattern.id}`}>
							Trigger Phrases ({pattern.trigger_phrases.length}) <span class="font-normal lowercase text-slate-400">· one trigger per line</span>
						</label>
						<textarea id={`triggers-${pattern.id}`} disabled={!pattern.approved} value={pattern.trigger_phrases.join('\n')} oninput={(event) => updateTriggers(pattern, event.currentTarget.value)} rows="2" class="w-full resize-y bg-slate-50/50 focus:bg-white border border-slate-200/80 rounded-xl p-2.5 text-xs text-slate-700 font-normal outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 disabled:bg-slate-100/70 disabled:text-slate-400 leading-relaxed transition"></textarea>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
