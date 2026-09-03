<script lang="ts">
	type Concept = { id: string; title: string; type: string; tags: string[]; body_text: string; approved: boolean };
	type Pattern = { id: string; canonical_question: string; answer_text: string; trigger_phrases: string[]; approved: boolean };

	let { concepts = $bindable(), patterns = $bindable() }: { concepts: Concept[]; patterns: Pattern[] } = $props();

	function updateTriggers(pattern: Pattern, value: string) {
		pattern.trigger_phrases = value.split('\n').map((phrase) => phrase.trim()).filter(Boolean);
	}
</script>

<div class="mb-2.5 mt-5 flex items-center justify-between w-full">
	<div class="flex items-center gap-2">
		<h3 class="text-sm font-medium text-slate-800">Knowledge concepts</h3>
		<span class="rounded-md bg-slate-100 px-1.5 py-0.5 text-[10px] font-medium text-slate-500">{concepts.length}</span>
	</div>
	<div class="text-[11px] text-slate-400">Select concepts to include</div>
</div>
<div class="grid grid-cols-1 sm:grid-cols-2 gap-3 w-full">
	{#each concepts as concept (concept.id)}
		<div class="p-4 bg-white border rounded-xl space-y-2.5 transition-all {concept.approved ? 'border-slate-200 shadow-xs' : 'border-slate-200/60 bg-slate-50/50 opacity-60'}">
			<div class="flex items-center gap-2">
				<input type="checkbox" bind:checked={concept.approved} aria-label={`Include ${concept.title || 'knowledge concept'}`} class="rounded border-slate-300 text-blue-600 focus:ring-blue-500 cursor-pointer" />
				<input bind:value={concept.title} disabled={!concept.approved} aria-label="Concept title" placeholder="Concept title" class="min-w-0 flex-1 bg-white border border-slate-200 rounded-lg px-2.5 py-1 text-xs font-medium text-slate-900 outline-none focus:border-blue-500 disabled:bg-slate-100/70 disabled:text-slate-400 transition" />
				<input bind:value={concept.type} disabled={!concept.approved} aria-label="Concept type" placeholder="Type" class="w-24 bg-blue-50 border border-blue-100 rounded-lg px-2 py-1 text-[11px] font-medium text-blue-700 outline-none focus:border-blue-500 disabled:opacity-50 transition text-center capitalize truncate" />
			</div>
			<textarea bind:value={concept.body_text} disabled={!concept.approved} aria-label="Concept content" rows="4" class="w-full resize-y bg-white border border-slate-200 rounded-lg p-2.5 text-xs text-slate-600 font-normal outline-none focus:border-blue-500 disabled:bg-slate-100/70 disabled:text-slate-400 leading-relaxed transition"></textarea>
		</div>
	{/each}
</div>

<div class="mb-2.5 mt-6 flex items-center justify-between w-full">
	<div class="flex items-center gap-2">
		<h3 class="text-sm font-medium text-slate-800">Direct answer patterns</h3>
		<span class="rounded-md bg-blue-100 px-1.5 py-0.5 text-[10px] font-medium text-blue-700">{patterns.length}</span>
	</div>
	<div class="text-[11px] text-slate-400">Direct response triggers</div>
</div>
<div class="grid grid-cols-1 gap-3 w-full">
	{#each patterns as pattern (pattern.id)}
		<div class="p-4 bg-white border rounded-xl space-y-2.5 transition-all {pattern.approved ? 'border-blue-200 bg-blue-50/20 shadow-xs' : 'border-slate-200/60 bg-slate-50/50 opacity-60'}">
			<div class="flex items-center gap-2">
				<input type="checkbox" bind:checked={pattern.approved} aria-label={`Include pattern ${pattern.canonical_question || 'answer pattern'}`} class="rounded border-slate-300 text-blue-600 focus:ring-blue-500 cursor-pointer" />
				<input bind:value={pattern.canonical_question} disabled={!pattern.approved} aria-label="Canonical question" placeholder="Canonical question" class="min-w-0 flex-1 bg-white border border-slate-200 rounded-lg px-2.5 py-1 text-xs font-medium text-slate-900 outline-none focus:border-blue-500 disabled:bg-slate-100/70 disabled:text-slate-400 transition" />
			</div>
			<textarea bind:value={pattern.answer_text} disabled={!pattern.approved} aria-label="Pattern answer" rows="3" class="w-full resize-y bg-white border border-slate-200 rounded-lg p-2.5 text-xs text-slate-600 outline-none focus:border-blue-500 disabled:bg-slate-100/70 disabled:text-slate-400 leading-relaxed transition"></textarea>
			<div class="flex items-center justify-between pt-0.5">
				<label class="block text-[10px] font-medium uppercase tracking-wide text-slate-400" for={`triggers-${pattern.id}`}>
					Trigger phrases ({pattern.trigger_phrases.length}) <span class="font-normal text-slate-400">· one trigger per line</span>
				</label>
			</div>
			<textarea id={`triggers-${pattern.id}`} disabled={!pattern.approved} value={pattern.trigger_phrases.join('\n')} oninput={(event) => updateTriggers(pattern, event.currentTarget.value)} rows="3" class="w-full resize-y bg-white border border-slate-200 rounded-lg p-2.5 text-[11px] text-slate-600 outline-none focus:border-blue-500 disabled:bg-slate-100/70 disabled:text-slate-400 leading-relaxed font-mono transition"></textarea>
		</div>
	{/each}
</div>
