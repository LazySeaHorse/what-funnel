<script lang="ts">
	type Concept = { id: string; title: string; type: string; tags: string[]; body_text: string; approved: boolean };
	type Pattern = { id: string; canonical_question: string; answer_text: string; trigger_phrases: string[]; approved: boolean };

	let { concepts = $bindable(), patterns = $bindable() }: { concepts: Concept[]; patterns: Pattern[] } = $props();

	function updateTriggers(pattern: Pattern, value: string) {
		pattern.trigger_phrases = value.split('\n').map((phrase) => phrase.trim()).filter(Boolean);
	}
</script>

<div class="mb-3 mt-4 flex items-center justify-between w-full">
	<div class="flex items-center gap-2">
		<h3 class="text-xs font-semibold uppercase tracking-wider text-slate-500">Extracted Concepts</h3>
		<span class="rounded-full bg-slate-100 px-2 py-0.5 text-[11px] font-medium text-slate-600">{concepts.length}</span>
	</div>
	<div class="text-[11px] text-slate-400">Review &amp; select concepts to include</div>
</div>
<div class="grid grid-cols-1 sm:grid-cols-2 gap-3 w-full">
	{#each concepts as concept (concept.id)}
		<div class="p-4 bg-white border rounded-2xl space-y-3 transition-all {concept.approved ? 'border-slate-200/90 shadow-2xs hover:border-slate-300' : 'border-slate-200/50 bg-slate-50/50 opacity-60'}">
			<div class="flex items-center gap-2">
				<input type="checkbox" bind:checked={concept.approved} aria-label={`Include ${concept.title || 'knowledge concept'}`} class="w-4 h-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500 cursor-pointer transition" />
				<input bind:value={concept.title} disabled={!concept.approved} aria-label="Concept title" placeholder="Concept title" class="min-w-0 flex-1 bg-white border border-slate-200 rounded-xl px-3 py-1.5 text-xs font-semibold text-slate-900 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 disabled:bg-slate-100/70 disabled:text-slate-400 transition" />
				<input bind:value={concept.type} disabled={!concept.approved} aria-label="Concept type" placeholder="Type" class="w-24 bg-blue-50/80 border border-blue-200/80 rounded-xl px-2.5 py-1.5 text-[11px] font-medium text-blue-700 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 disabled:opacity-50 transition text-center capitalize truncate" />
			</div>
			<textarea bind:value={concept.body_text} disabled={!concept.approved} aria-label="Concept content" rows="4" class="w-full resize-y bg-slate-50/50 focus:bg-white border border-slate-200/80 rounded-xl p-3 text-xs text-slate-600 font-normal outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 disabled:bg-slate-100/70 disabled:text-slate-400 leading-relaxed transition"></textarea>
		</div>
	{/each}
</div>

<div class="mb-3 mt-6 flex items-center justify-between w-full">
	<div class="flex items-center gap-2">
		<h3 class="text-xs font-semibold uppercase tracking-wider text-slate-500">Direct Answer Patterns</h3>
		<span class="rounded-full bg-blue-50 px-2 py-0.5 text-[11px] font-medium text-blue-700 border border-blue-100/60">{patterns.length}</span>
	</div>
	<div class="text-[11px] text-slate-400">Deterministic Q&amp;A triggers</div>
</div>
<div class="grid grid-cols-1 gap-3 w-full">
	{#each patterns as pattern (pattern.id)}
		<div class="p-4 bg-white border rounded-2xl space-y-3 transition-all {pattern.approved ? 'border-blue-200/80 bg-blue-50/10 shadow-2xs hover:border-blue-300' : 'border-slate-200/50 bg-slate-50/50 opacity-60'}">
			<div class="flex items-center gap-2">
				<input type="checkbox" bind:checked={pattern.approved} aria-label={`Include pattern ${pattern.canonical_question || 'answer pattern'}`} class="w-4 h-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500 cursor-pointer transition" />
				<input bind:value={pattern.canonical_question} disabled={!pattern.approved} aria-label="Canonical question" placeholder="Canonical question" class="min-w-0 flex-1 bg-white border border-slate-200 rounded-xl px-3 py-1.5 text-xs font-semibold text-slate-900 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 disabled:bg-slate-100/70 disabled:text-slate-400 transition" />
			</div>
			<textarea bind:value={pattern.answer_text} disabled={!pattern.approved} aria-label="Pattern answer" rows="3" class="w-full resize-y bg-slate-50/50 focus:bg-white border border-slate-200/80 rounded-xl p-3 text-xs text-slate-600 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 disabled:bg-slate-100/70 disabled:text-slate-400 leading-relaxed transition"></textarea>
			<div class="space-y-1.5 pt-0.5">
				<label class="block text-[10px] font-semibold uppercase tracking-wider text-slate-400" for={`triggers-${pattern.id}`}>
					Trigger Phrases ({pattern.trigger_phrases.length}) <span class="font-normal lowercase text-slate-400">· one trigger per line</span>
				</label>
				<textarea id={`triggers-${pattern.id}`} disabled={!pattern.approved} value={pattern.trigger_phrases.join('\n')} oninput={(event) => updateTriggers(pattern, event.currentTarget.value)} rows="2" class="w-full resize-y bg-slate-50/50 focus:bg-white border border-slate-200/80 rounded-xl p-2.5 text-xs text-slate-700 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 disabled:bg-slate-100/70 disabled:text-slate-400 leading-relaxed transition"></textarea>
			</div>
		</div>
	{/each}
</div>
