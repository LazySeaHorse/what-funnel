<script lang="ts">
	import { onMount } from 'svelte';
	import { apiRequest } from '$lib/api';

	let { reviewerID = '' }: { reviewerID?: string } = $props();

	let concepts = $state<any[]>([]);
	let patterns = $state<any[]>([]);
	let suggestions = $state<any[]>([]);
	let lastRun = $state<any>(null);
	let loading = $state(true);
	let activeTab = $state<'concepts' | 'patterns' | 'suggestions'>('concepts');
	let pasteText = $state('');
	let pasting = $state(false);
	let pasteResult = $state<{ added?: number; queued?: number; error?: string } | null>(null);
	let expandedConcept = $state<string | null>(null);
	let mining = $state(false);
	let miningResult = $state<{ messages_scanned?: number; clusters_found?: number; suggestions_created?: number } | null>(null);

	async function load(refresh = false) {
		loading = !refresh;
		try {
			const [conceptsRes, patternsRes, suggestionsRes, miningRes] = await Promise.allSettled([
				apiRequest('/api/kb/concepts'),
				apiRequest('/api/kb/patterns'),
				apiRequest('/api/kb/suggestions?status_filter=pending'),
				apiRequest('/api/kb/mining-runs/latest')
			]);
			if (conceptsRes.status === 'fulfilled') concepts = conceptsRes.value?.concepts ?? [];
			if (patternsRes.status === 'fulfilled') patterns = patternsRes.value?.patterns ?? [];
			if (suggestionsRes.status === 'fulfilled') {
				suggestions = (suggestionsRes.value?.suggestions ?? []).map((suggestion: any) => {
					let payload = suggestion.proposed_payload ?? {};
					if (typeof payload === 'string') {
						try { payload = JSON.parse(payload); } catch { payload = {}; }
					}
					return { ...suggestion, _payload: payload };
				});
			}
			if (miningRes.status === 'fulfilled') lastRun = miningRes.value?.last_run ?? null;
		} finally {
			loading = false;
		}
	}

	onMount(() => { void load(); });

	async function deleteConcept(id: string) {
		if (!confirm('Delete this knowledge concept?')) return;
		await apiRequest(`/api/kb/concepts/${id}`, { method: 'DELETE' });
		concepts = concepts.filter((concept) => concept.id !== id);
		if (expandedConcept === id) expandedConcept = null;
	}

	async function deletePattern(id: string) {
		if (!confirm('Delete this pattern?')) return;
		await apiRequest(`/api/kb/patterns/${id}`, { method: 'DELETE' });
		patterns = patterns.filter((pattern) => pattern.id !== id);
	}

	async function compilePaste() {
		if (!pasteText.trim()) return;
		pasting = true;
		pasteResult = null;
		try {
			const result = await apiRequest('/api/kb/compile-paste', { method: 'POST', body: { raw_text: pasteText.trim() } });
			if (result.added_concepts) {
				pasteResult = { added: result.added_concepts.length };
				concepts = [...result.added_concepts, ...concepts];
			} else if (result.suggestion_ids) {
				pasteResult = { queued: result.suggestion_ids.length };
			}
			pasteText = '';
		} catch (error: any) {
			pasteResult = { error: error.message || 'Failed to compile' };
		} finally {
			pasting = false;
		}
	}

	async function reviewSuggestion(id: string, action: 'approve' | 'reject') {
		await apiRequest(`/api/kb/suggestions/${id}/${action}`, { method: 'POST', body: { reviewed_by: reviewerID } });
		suggestions = suggestions.filter((suggestion) => suggestion.id !== id);
		if (action === 'approve') await load(true);
	}

	async function triggerMining() {
		mining = true;
		miningResult = null;
		try {
			miningResult = await apiRequest('/api/kb/mine/trigger', { method: 'POST' });
			await load(true);
		} finally {
			mining = false;
		}
	}

	function formatDate(iso?: string | null) {
		if (!iso) return 'Never';
		return new Date(iso).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
	}

	function typeLabel(type?: string) {
		return type ? type.charAt(0).toUpperCase() + type.slice(1).replace(/_/g, ' ') : 'General';
	}

	function typeColor(type?: string) {
		return ({
			faq: 'bg-blue-50 text-blue-600 border-blue-200/80',
			pricing: 'bg-emerald-50 text-emerald-600 border-emerald-200/80',
			policy: 'bg-amber-50 text-amber-600 border-amber-200/80',
			hours: 'bg-purple-50 text-purple-600 border-purple-200/80',
			service: 'bg-rose-50 text-rose-600 border-rose-200/80'
		} as Record<string, string>)[(type || '').toLowerCase()] || 'bg-slate-50 text-slate-600 border-slate-200/80';
	}
</script>

<div class="flex-1 flex flex-col overflow-hidden">
	<header class="px-6 pt-6 pb-4 border-b border-slate-100 shrink-0">
		<div class="flex items-start justify-between">
			<div>
				<h1 class="text-xl font-medium text-slate-900 tracking-tight">Knowledge Base</h1>
				<p class="text-xs text-slate-500 mt-0.5">Train What Funnel AI on your pricing, FAQs, services, and policies</p>
			</div>
			<div class="flex items-center gap-3">
				<div class="text-right">
					<div class="text-[11px] font-medium text-slate-400 uppercase tracking-wide">Last AI Audit</div>
					<div class="text-xs font-medium text-slate-700 mt-0.5">{formatDate(lastRun?.run_at)}</div>
					{#if lastRun}<div class="text-[11px] text-slate-400">{lastRun.messages_scanned} scanned · {lastRun.clusters_found} clusters · {lastRun.suggestions_created} suggestions</div>{/if}
				</div>
				<button onclick={triggerMining} disabled={mining} class="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-slate-50 border border-slate-200 text-xs font-medium text-slate-700 hover:bg-slate-100 transition disabled:opacity-50">
					{mining ? 'Scanning…' : 'Run Audit Now'}
				</button>
			</div>
		</div>
		{#if miningResult}
			<div class="mt-3 px-3 py-2 bg-blue-50 border border-blue-100 rounded-xl text-xs text-blue-700">Audit complete — {miningResult.messages_scanned} messages scanned, {miningResult.clusters_found} clusters found, {miningResult.suggestions_created} suggestions created.</div>
		{/if}
		<nav class="flex gap-1 mt-4" aria-label="Knowledge sections">
			{#each [{ key: 'concepts', label: 'KB Concepts', count: concepts.length }, { key: 'patterns', label: 'Patterns', count: patterns.length }, { key: 'suggestions', label: 'AI Suggestions', count: suggestions.length }] as tab}
				<button onclick={() => activeTab = tab.key as typeof activeTab} class="flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg text-xs font-medium transition-all {activeTab === tab.key ? 'bg-blue-50 text-blue-600' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-50'}">
					{tab.label}<span class="px-1.5 py-0.5 rounded-md text-[10px] font-medium {activeTab === tab.key ? 'bg-blue-100 text-blue-600' : 'bg-slate-100 text-slate-500'}">{tab.count}</span>
				</button>
			{/each}
		</nav>
	</header>

	{#if loading}
		<div class="flex-1 flex items-center justify-center"><span class="w-5 h-5 border-2 border-blue-400 border-t-transparent rounded-full animate-spin"></span></div>
	{:else if activeTab === 'concepts'}
		<div class="flex-1 overflow-y-auto min-h-0 flex flex-col">
			<div class="px-6 py-4 border-b border-slate-100 shrink-0">
				<div class="text-xs font-medium text-slate-700 mb-2">Add business knowledge</div>
				<textarea bind:value={pasteText} placeholder="Paste anything — pricing, policies, FAQs, hours, services… The AI will extract and structure it automatically." class="w-full h-20 p-3 text-xs text-slate-700 placeholder-slate-400 bg-slate-50 rounded-xl border border-slate-200 focus:outline-none focus:border-blue-400 resize-none leading-relaxed"></textarea>
				<div class="flex items-center justify-between mt-2">
					<div>
						{#if pasteResult?.added !== undefined}<span class="text-xs text-emerald-600 font-medium">✓ {pasteResult.added} concept{pasteResult.added !== 1 ? 's' : ''} added to KB</span>
						{:else if pasteResult?.queued !== undefined}<span class="text-xs text-amber-600 font-medium">⏳ {pasteResult.queued} concepts queued for review (AI Suggestions tab)</span>
						{:else if pasteResult?.error}<span class="text-xs text-rose-600 font-medium">✕ {pasteResult.error}</span>{/if}
					</div>
					<button onclick={compilePaste} disabled={pasting || !pasteText.trim()} class="flex items-center gap-1.5 px-3.5 py-1.5 rounded-xl bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium transition disabled:opacity-50">{pasting ? 'Compiling…' : 'Compile with AI'}</button>
				</div>
			</div>
			<div class="flex-1 overflow-y-auto px-6 py-4 space-y-2">
				{#if concepts.length === 0}
					<div class="flex flex-col items-center justify-center py-16 text-center"><div class="text-sm font-medium text-slate-600">No knowledge concepts yet</div><div class="text-xs text-slate-400 mt-1">Paste your business info above and click "Compile with AI"</div></div>
				{:else}
					{#each concepts as concept (concept.id)}
						<div class="border border-slate-200/80 rounded-xl overflow-hidden">
							<button onclick={() => expandedConcept = expandedConcept === concept.id ? null : concept.id} class="w-full px-4 py-3 flex items-center justify-between text-left hover:bg-slate-50/60 transition">
								<div class="flex items-center gap-2.5 min-w-0"><span class="px-2 py-0.5 rounded text-[10px] font-medium border capitalize {typeColor(concept.type)}">{typeLabel(concept.type)}</span><span class="text-xs font-medium text-slate-800 truncate">{concept.title}</span>{#if concept.source === 'owner_pasted'}<span class="text-[10px] text-slate-400 bg-slate-100 px-1.5 py-0.5 rounded">pasted</span>{/if}</div>
								<span class="text-slate-400">⌄</span>
							</button>
							{#if expandedConcept === concept.id}<div class="px-4 pb-4 pt-1 border-t border-slate-100 bg-slate-50/40 text-xs space-y-2"><div class="text-slate-700 leading-relaxed whitespace-pre-wrap">{concept.body_markdown}</div><div class="flex items-center justify-between pt-2 text-[11px] text-slate-400 border-t border-slate-100"><span>Added {formatDate(concept.created_at)}</span><button onclick={() => deleteConcept(concept.id)} class="text-rose-500 hover:text-rose-700 transition">Delete</button></div></div>{/if}
						</div>
					{/each}
				{/if}
			</div>
		</div>
	{:else if activeTab === 'patterns'}
		<div class="flex-1 overflow-y-auto px-6 py-4 space-y-3">
			{#if patterns.length === 0}<div class="flex flex-col items-center justify-center py-16 text-center"><div class="text-sm font-medium text-slate-600">No conversation patterns mined yet</div><div class="text-xs text-slate-400 mt-1">Run an AI audit above to scan customer messages for common question patterns</div></div>
			{:else}{#each patterns as pattern (pattern.id)}<div class="p-4 rounded-xl border border-slate-200/80 bg-white space-y-2"><div class="text-xs font-medium text-slate-800">{pattern.canonical_question}</div>{#if pattern.trigger_phrases?.length}<div class="text-[11px] text-slate-400">Triggers: {pattern.trigger_phrases.join(', ')}</div>{/if}<div class="text-xs text-slate-600 leading-relaxed whitespace-pre-wrap">{pattern.answer_markdown}</div><div class="flex justify-end pt-1 text-[11px]"><button onclick={() => deletePattern(pattern.id)} class="text-rose-500 hover:text-rose-700 transition">Delete</button></div></div>{/each}{/if}
		</div>
	{:else}
		<div class="flex-1 overflow-y-auto px-6 py-4 space-y-3">
			{#if suggestions.length === 0}<div class="flex flex-col items-center justify-center py-16 text-center"><div class="text-sm font-medium text-slate-600">No suggestions pending review</div><div class="text-xs text-slate-400 mt-1">When AI audits find knowledge gaps in conversations, recommendations will appear here</div></div>
			{:else}{#each suggestions as suggestion (suggestion.id)}<div class="p-4 rounded-xl border border-slate-200/80 bg-white space-y-2.5"><div class="flex items-center justify-between"><div class="flex items-center gap-2"><span class="px-2 py-0.5 rounded text-[10px] font-medium border capitalize {typeColor(suggestion._payload?.type ?? suggestion.type)}">{typeLabel(suggestion._payload?.type ?? suggestion.type)}</span><span class="text-xs font-medium text-slate-800">{suggestion._payload?.title ?? suggestion._payload?.canonical_question ?? '—'}</span></div><span class="text-[10px] text-slate-400">conf {((suggestion.confidence ?? 0) * 100).toFixed(0)}%</span></div><div class="text-xs text-slate-600 bg-slate-50 p-2.5 rounded-lg leading-relaxed whitespace-pre-wrap">{suggestion._payload?.body_markdown ?? suggestion._payload?.answer_markdown ?? ''}</div><div class="flex items-center justify-between pt-1 text-xs"><span class="text-[11px] text-slate-400">{suggestion.type?.replace(/_/g, ' ')}</span><div class="flex items-center gap-2"><button onclick={() => reviewSuggestion(suggestion.id, 'reject')} class="px-2.5 py-1 text-slate-500 hover:text-slate-700 hover:bg-slate-100 rounded-lg transition">Dismiss</button><button onclick={() => reviewSuggestion(suggestion.id, 'approve')} class="px-3 py-1 bg-blue-600 hover:bg-blue-700 text-white font-medium rounded-lg transition">Add to Knowledge Base</button></div></div></div>{/each}{/if}
		</div>
	{/if}
</div>
