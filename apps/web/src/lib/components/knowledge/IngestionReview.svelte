<script lang="ts">
	type Concept = { id: string; title: string; type: string; tags: string[]; body_markdown: string; approved: boolean };
	type Pattern = { id: string; canonical_question: string; answer_markdown: string; trigger_phrases: string[]; approved: boolean };

	let { concepts = $bindable(), patterns = $bindable() }: { concepts: Concept[]; patterns: Pattern[] } = $props();

	function updateTriggers(pattern: Pattern, value: string) {
		pattern.trigger_phrases = value.split('\n').map((phrase) => phrase.trim()).filter(Boolean);
	}
</script>

<div class="mb-2 mt-5 flex items-center gap-2 w-full">
	<h3 class="text-sm font-medium text-slate-800">Knowledge concepts</h3>
	<span class="rounded-md bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-500">{concepts.length}</span>
</div>
<div class="grid grid-cols-1 sm:grid-cols-2 gap-3 w-full">
	{#each concepts as concept (concept.id)}
		<div class="p-4 bg-slate-50/70 border rounded-xl space-y-2 {concept.approved ? 'border-slate-200' : 'border-slate-200 opacity-60'}">
			<div class="flex items-center gap-2">
				<input type="checkbox" bind:checked={concept.approved} aria-label={`Include ${concept.title || 'knowledge concept'}`} class="rounded border-slate-300 text-blue-600" />
				<input bind:value={concept.title} aria-label="Concept title" class="min-w-0 flex-1 bg-white border border-slate-200 rounded-lg px-2 py-1 text-xs font-medium text-slate-900 outline-none focus:border-blue-500" />
				<input bind:value={concept.type} aria-label="Concept type" class="w-20 bg-blue-50 border border-blue-100 rounded-lg px-2 py-1 text-[10px] font-medium text-blue-700 outline-none focus:border-blue-500" />
			</div>
			<textarea bind:value={concept.body_markdown} aria-label="Concept content" rows="4" class="w-full resize-y bg-white border border-slate-200 rounded-lg p-2 text-xs text-slate-600 font-normal outline-none focus:border-blue-500"></textarea>
		</div>
	{/each}
</div>

<div class="mb-2 mt-6 flex items-center gap-2 w-full">
	<h3 class="text-sm font-medium text-slate-800">Deterministic answer patterns</h3>
	<span class="rounded-md bg-blue-100 px-1.5 py-0.5 text-[10px] text-blue-700">{patterns.length}</span>
</div>
<div class="grid grid-cols-1 gap-3 w-full">
	{#each patterns as pattern (pattern.id)}
		<div class="p-4 bg-blue-50/40 border rounded-xl space-y-2 {pattern.approved ? 'border-blue-200' : 'border-slate-200 opacity-60'}">
			<div class="flex items-center gap-2">
				<input type="checkbox" bind:checked={pattern.approved} aria-label={`Include pattern ${pattern.canonical_question || 'answer pattern'}`} class="rounded border-slate-300 text-blue-600" />
				<input bind:value={pattern.canonical_question} aria-label="Canonical question" class="min-w-0 flex-1 bg-white border border-slate-200 rounded-lg px-2 py-1 text-xs font-medium text-slate-900 outline-none focus:border-blue-500" />
			</div>
			<textarea bind:value={pattern.answer_markdown} aria-label="Pattern answer" rows="3" class="w-full resize-y bg-white border border-slate-200 rounded-lg p-2 text-xs text-slate-600 outline-none focus:border-blue-500"></textarea>
			<label class="block text-[10px] font-medium uppercase tracking-wide text-slate-400" for={`triggers-${pattern.id}`}>Trigger phrases, one per line</label>
			<textarea id={`triggers-${pattern.id}`} value={pattern.trigger_phrases.join('\n')} oninput={(event) => updateTriggers(pattern, event.currentTarget.value)} rows="3" class="w-full resize-y bg-white border border-slate-200 rounded-lg p-2 text-[11px] text-slate-600 outline-none focus:border-blue-500"></textarea>
		</div>
	{/each}
</div>
