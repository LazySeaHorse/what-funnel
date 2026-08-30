<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { apiRequest } from '$lib/api';
	import IngestionReview from '$lib/components/knowledge/IngestionReview.svelte';

	let { reviewerID = '' }: { reviewerID?: string } = $props();

	let concepts = $state<any[]>([]);
	let patterns = $state<any[]>([]);
	let suggestions = $state<any[]>([]);
	let lastRun = $state<any>(null);
	let loading = $state(true);
	let activeTab = $state<'concepts' | 'patterns' | 'suggestions'>('concepts');
	let pasteText = $state('');
	let pasting = $state(false);
	let pasteResult = $state<{ added?: number; patternsAdded?: number; error?: string } | null>(null);
	let ingestionID = $state('');
	let ingestionPhase = $state<'idle' | 'processing' | 'review' | 'publishing'>('idle');
	let ingestionConcepts = $state<any[]>([]);
	let ingestionPatterns = $state<any[]>([]);
	let pollGeneration = 0;
	let expandedConcept = $state<string | null>(null);
	let mining = $state(false);
	let miningResult = $state<{ messages_scanned?: number; clusters_found?: number; suggestions_created?: number } | null>(null);
	let purging = $state(false);
	let purgeResult = $state<{ concepts: number; patterns: number } | null>(null);
	let purgeError = $state('');

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

	onMount(() => {
		void load();
		void resumeLatestIngestion();
	});
	onDestroy(() => { pollGeneration++; });

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

	async function purgeKnowledgeBase() {
		if (ingestionPhase !== 'idle') return;
		if (!confirm('Permanently delete all concepts and deterministic patterns in this workspace? This cannot be undone.')) return;

		purging = true;
		purgeError = '';
		purgeResult = null;
		try {
			const result = await apiRequest('/api/kb/purge', { method: 'DELETE' });
			concepts = [];
			patterns = [];
			expandedConcept = null;
			pasteResult = null;
			purgeResult = {
				concepts: result.cleared_concepts ?? 0,
				patterns: result.cleared_patterns ?? 0
			};
		} catch (error: any) {
			purgeError = error.message || 'Failed to purge the knowledge base.';
		} finally {
			purging = false;
		}
	}

	function mapConcepts(items: any[]) {
		return (items ?? []).map((item: any) => ({
			id: item.id,
			title: item.title ?? '',
			type: item.type ?? 'faq',
			tags: Array.isArray(item.tags) ? item.tags : [],
			body_markdown: item.body_markdown ?? '',
			approved: item.status !== 'rejected'
		}));
	}

	function mapPatterns(items: any[]) {
		return (items ?? []).map((item: any) => ({
			id: item.id,
			canonical_question: item.canonical_question ?? '',
			answer_markdown: item.answer_markdown ?? '',
			trigger_phrases: Array.isArray(item.trigger_phrases) ? item.trigger_phrases : [],
			approved: item.status !== 'rejected'
		}));
	}

	async function pollIngestion(id: string, generation: number) {
		while (generation === pollGeneration) {
			const ingestion = await apiRequest(`/api/kb/ingestions/${id}`);
			if (ingestion.status === 'review_required') {
				ingestionConcepts = mapConcepts(ingestion.concepts);
				ingestionPatterns = mapPatterns(ingestion.patterns);
				ingestionPhase = 'review';
				pasting = false;
				return;
			}
			if (ingestion.status === 'complete') {
				const added = ingestionConcepts.filter((item) => item.approved).length;
				const patternsAdded = ingestionPatterns.filter((item) => item.approved).length;
				pasteResult = { added, patternsAdded };
				pasteText = '';
				ingestionID = '';
				ingestionConcepts = [];
				ingestionPatterns = [];
				ingestionPhase = 'idle';
				pasting = false;
				await load(true);
				return;
			}
			if (ingestion.status === 'failed') {
				throw new Error(ingestion.error || 'Knowledge ingestion failed.');
			}
			ingestionPhase = ingestion.status === 'publishing' ? 'publishing' : 'processing';
			await new Promise((resolve) => setTimeout(resolve, 750));
		}
	}

	async function resumeLatestIngestion() {
		try {
			const response = await apiRequest('/api/kb/ingestions/latest');
			const ingestion = response?.ingestion;
			if (!ingestion || !['queued', 'processing', 'review_required', 'publishing'].includes(ingestion.status)) return;
			ingestionID = ingestion.id;
			pasting = true;
			const generation = ++pollGeneration;
			await pollIngestion(ingestion.id, generation);
		} catch (error: any) {
			pasteResult = { error: error.message || 'Failed to resume knowledge ingestion' };
			pasting = false;
			ingestionPhase = 'idle';
		}
	}

	async function compilePaste() {
		if (!pasteText.trim()) return;
		pasting = true;
		ingestionPhase = 'processing';
		pasteResult = null;
		try {
			const ingestion = await apiRequest('/api/kb/ingestions', { method: 'POST', body: { raw_text: pasteText.trim() } });
			ingestionID = ingestion.id;
			const generation = ++pollGeneration;
			await pollIngestion(ingestion.id, generation);
		} catch (error: any) {
			pasteResult = { error: error.message || 'Failed to compile' };
			pasting = false;
			ingestionPhase = 'idle';
		}
	}

	async function publishIngestion() {
		if (!ingestionID) return;
		if (!ingestionConcepts.some((item) => item.approved) && !ingestionPatterns.some((item) => item.approved)) {
			pasteResult = { error: 'Select at least one concept or pattern to add.' };
			return;
		}
		pasting = true;
		ingestionPhase = 'publishing';
		pasteResult = null;
		try {
			await apiRequest(`/api/kb/ingestions/${ingestionID}/publish`, {
				method: 'POST',
				body: { concepts: ingestionConcepts, patterns: ingestionPatterns }
			});
			const generation = ++pollGeneration;
			await pollIngestion(ingestionID, generation);
		} catch (error: any) {
			pasteResult = { error: error.message || 'Failed to publish knowledge' };
			pasting = false;
			ingestionPhase = 'review';
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
				<button onclick={purgeKnowledgeBase} disabled={purging || ingestionPhase !== 'idle'} class="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-white border border-rose-200 text-xs font-medium text-rose-600 hover:bg-rose-50 transition disabled:opacity-40">
					{purging ? 'Purging…' : 'Purge KB'}
				</button>
			</div>
		</div>
		{#if miningResult}
			<div class="mt-3 px-3 py-2 bg-blue-50 border border-blue-100 rounded-xl text-xs text-blue-700">Audit complete — {miningResult.messages_scanned} messages scanned, {miningResult.clusters_found} clusters found, {miningResult.suggestions_created} suggestions created.</div>
		{/if}
		{#if purgeResult}
			<div class="mt-3 px-3 py-2 bg-emerald-50 border border-emerald-100 rounded-xl text-xs text-emerald-700">Knowledge base purged — {purgeResult.concepts} concept{purgeResult.concepts !== 1 ? 's' : ''} and {purgeResult.patterns} pattern{purgeResult.patterns !== 1 ? 's' : ''} removed.</div>
		{:else if purgeError}
			<div class="mt-3 px-3 py-2 bg-rose-50 border border-rose-100 rounded-xl text-xs text-rose-700">{purgeError}</div>
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
				{#if ingestionPhase === 'review'}
					<div class="flex items-center justify-between mb-3">
						<div><div class="text-sm font-medium text-slate-800">Review structured knowledge</div><div class="text-[11px] text-slate-500">The same concept and deterministic-pattern review used during onboarding.</div></div>
						<button onclick={publishIngestion} disabled={pasting} class="px-3.5 py-1.5 rounded-xl bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium disabled:opacity-50">Add selected to Knowledge Base</button>
					</div>
					{#if pasteResult?.error}<div class="mb-3 text-xs text-rose-600 font-medium">✕ {pasteResult.error}</div>{/if}
					<div class="max-h-[52vh] overflow-y-auto pr-1"><IngestionReview bind:concepts={ingestionConcepts} bind:patterns={ingestionPatterns} /></div>
				{:else}
					<div class="text-xs font-medium text-slate-700 mb-2">Add business knowledge</div>
					<textarea bind:value={pasteText} disabled={pasting} placeholder="Paste anything — pricing, policies, FAQs, hours, services… The AI will extract concepts and deterministic answer patterns." class="w-full h-20 p-3 text-xs text-slate-700 placeholder-slate-400 bg-slate-50 rounded-xl border border-slate-200 focus:outline-none focus:border-blue-400 resize-none leading-relaxed disabled:opacity-60"></textarea>
					<div class="flex items-center justify-between mt-2">
						<div>
							{#if pasteResult?.added !== undefined}<span class="text-xs text-emerald-600 font-medium">✓ {pasteResult.added} concept{pasteResult.added !== 1 ? 's' : ''} and {pasteResult.patternsAdded ?? 0} pattern{pasteResult.patternsAdded !== 1 ? 's' : ''} added</span>
							{:else if pasteResult?.error}<span class="text-xs text-rose-600 font-medium">✕ {pasteResult.error}</span>
							{:else if ingestionPhase === 'processing'}<span class="text-xs text-blue-600 font-medium">Organizing concepts and deterministic patterns…</span>
							{:else if ingestionPhase === 'publishing'}<span class="text-xs text-blue-600 font-medium">Publishing reviewed knowledge…</span>{/if}
						</div>
						<button onclick={compilePaste} disabled={pasting || !pasteText.trim()} class="flex items-center gap-1.5 px-3.5 py-1.5 rounded-xl bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium transition disabled:opacity-50">{pasting ? 'Processing…' : 'Organize with AI'}</button>
					</div>
				{/if}
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
			{#if patterns.length === 0}<div class="flex flex-col items-center justify-center py-16 text-center"><div class="text-sm font-medium text-slate-600">No deterministic answer patterns yet</div><div class="text-xs text-slate-400 mt-1">Organize business knowledge or run an AI audit to create common question patterns</div></div>
			{:else}{#each patterns as pattern (pattern.id)}<div class="p-4 rounded-xl border border-slate-200/80 bg-white space-y-2"><div class="text-xs font-medium text-slate-800">{pattern.canonical_question}</div>{#if pattern.trigger_phrases?.length}<div class="text-[11px] text-slate-400">Triggers: {pattern.trigger_phrases.join(', ')}</div>{/if}<div class="text-xs text-slate-600 leading-relaxed whitespace-pre-wrap">{pattern.answer_markdown}</div><div class="flex justify-end pt-1 text-[11px]"><button onclick={() => deletePattern(pattern.id)} class="text-rose-500 hover:text-rose-700 transition">Delete</button></div></div>{/each}{/if}
		</div>
	{:else}
		<div class="flex-1 overflow-y-auto px-6 py-4 space-y-3">
			{#if suggestions.length === 0}<div class="flex flex-col items-center justify-center py-16 text-center"><div class="text-sm font-medium text-slate-600">No suggestions pending review</div><div class="text-xs text-slate-400 mt-1">When AI audits find knowledge gaps in conversations, recommendations will appear here</div></div>
			{:else}{#each suggestions as suggestion (suggestion.id)}<div class="p-4 rounded-xl border border-slate-200/80 bg-white space-y-2.5"><div class="flex items-center justify-between"><div class="flex items-center gap-2"><span class="px-2 py-0.5 rounded text-[10px] font-medium border capitalize {typeColor(suggestion._payload?.type ?? suggestion.type)}">{typeLabel(suggestion._payload?.type ?? suggestion.type)}</span><span class="text-xs font-medium text-slate-800">{suggestion._payload?.title ?? suggestion._payload?.canonical_question ?? '—'}</span></div><span class="text-[10px] text-slate-400">conf {((suggestion.confidence ?? 0) * 100).toFixed(0)}%</span></div><div class="text-xs text-slate-600 bg-slate-50 p-2.5 rounded-lg leading-relaxed whitespace-pre-wrap">{suggestion._payload?.body_markdown ?? suggestion._payload?.answer_markdown ?? ''}</div><div class="flex items-center justify-between pt-1 text-xs"><span class="text-[11px] text-slate-400">{suggestion.type?.replace(/_/g, ' ')}</span><div class="flex items-center gap-2"><button onclick={() => reviewSuggestion(suggestion.id, 'reject')} class="px-2.5 py-1 text-slate-500 hover:text-slate-700 hover:bg-slate-100 rounded-lg transition">Dismiss</button><button onclick={() => reviewSuggestion(suggestion.id, 'approve')} class="px-3 py-1 bg-blue-600 hover:bg-blue-700 text-white font-medium rounded-lg transition">Add to Knowledge Base</button></div></div></div>{/each}{/if}
		</div>
	{/if}
</div>
