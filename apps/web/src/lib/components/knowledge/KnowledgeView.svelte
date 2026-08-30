<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { apiRequest } from '$lib/api';
	import {
		SparklesIcon,
		TrashIcon,
		CheckIcon,
		XMarkIcon,
		BookOpenIcon,
		ChevronDownIcon,
		ChatBubbleLeftRightIcon
	} from '@fvilers/heroicons-svelte/24/outline';
	import IngestionReview from '$lib/components/knowledge/IngestionReview.svelte';

	let { reviewerID = '', searchQuery = '' }: { reviewerID?: string; searchQuery?: string } = $props();

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

	let filteredConcepts = $derived(
		!searchQuery.trim()
			? concepts
			: concepts.filter((c) =>
					(c.title || '').toLowerCase().includes(searchQuery.toLowerCase()) ||
					(c.body_markdown || '').toLowerCase().includes(searchQuery.toLowerCase()) ||
					(c.type || '').toLowerCase().includes(searchQuery.toLowerCase()) ||
					(c.tags || []).some((t: string) => t.toLowerCase().includes(searchQuery.toLowerCase()))
				)
	);

	let filteredPatterns = $derived(
		!searchQuery.trim()
			? patterns
			: patterns.filter((p) =>
					(p.canonical_question || '').toLowerCase().includes(searchQuery.toLowerCase()) ||
					(p.answer_markdown || '').toLowerCase().includes(searchQuery.toLowerCase()) ||
					(p.trigger_phrases || []).some((phrase: string) => phrase.toLowerCase().includes(searchQuery.toLowerCase()))
				)
	);

	let filteredSuggestions = $derived(
		!searchQuery.trim()
			? suggestions
			: suggestions.filter((s) => {
					const title = s._payload?.title || s._payload?.canonical_question || '';
					const body = s._payload?.body_markdown || s._payload?.answer_markdown || '';
					return (
						title.toLowerCase().includes(searchQuery.toLowerCase()) ||
						body.toLowerCase().includes(searchQuery.toLowerCase()) ||
						(s.type || '').toLowerCase().includes(searchQuery.toLowerCase())
					);
				})
	);

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

	function discardIngestion() {
		ingestionPhase = 'idle';
		ingestionID = '';
		ingestionConcepts = [];
		ingestionPatterns = [];
		pasteResult = null;
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
		<div class="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-4">
			<div>
				<h1 class="text-xl font-medium text-slate-900 tracking-tight">Knowledge base</h1>
				<p class="text-xs text-slate-500 mt-0.5">Manage pricing, FAQs, services, and policies for AI answers.</p>
			</div>
			<div class="flex flex-wrap items-center gap-3 self-start sm:self-auto">
				<div class="text-left sm:text-right">
					<div class="text-[11px] font-medium text-slate-400 uppercase tracking-wide">Last AI audit</div>
					<div class="text-xs font-medium text-slate-700 mt-0.5">{formatDate(lastRun?.run_at)}</div>
					{#if lastRun}<div class="text-[11px] text-slate-400">{lastRun.messages_scanned} scanned · {lastRun.clusters_found} clusters · {lastRun.suggestions_created} suggestions</div>{/if}
				</div>
				<button onclick={triggerMining} disabled={mining} class="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-slate-50 border border-slate-200 text-xs font-medium text-slate-700 hover:bg-slate-100 transition disabled:opacity-50 cursor-pointer">
					<SparklesIcon class="w-3.5 h-3.5" />
					<span>{mining ? 'Scanning…' : 'Run audit now'}</span>
				</button>
				<button onclick={purgeKnowledgeBase} disabled={purging || ingestionPhase !== 'idle'} class="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-white border border-rose-200 text-xs font-medium text-rose-600 hover:bg-rose-50 transition disabled:opacity-40 cursor-pointer">
					<TrashIcon class="w-3.5 h-3.5" />
					<span>{purging ? 'Purging…' : 'Purge knowledge base'}</span>
				</button>
			</div>
		</div>

		{#if miningResult}
			<div class="mt-3 px-3.5 py-2.5 bg-blue-50 border border-blue-100 rounded-xl text-xs text-blue-700 flex items-center justify-between gap-2">
				<div class="flex items-center gap-2">
					<SparklesIcon class="w-4 h-4 text-blue-600" />
					<span>Audit complete — {miningResult.messages_scanned} messages scanned, {miningResult.clusters_found} clusters found, {miningResult.suggestions_created} suggestions created.</span>
				</div>
				<button type="button" onclick={() => miningResult = null} class="text-blue-500 hover:text-blue-700 p-0.5 rounded cursor-pointer" aria-label="Dismiss banner">
					<XMarkIcon class="w-3.5 h-3.5" />
				</button>
			</div>
		{/if}
		{#if purgeResult}
			<div class="mt-3 px-3.5 py-2.5 bg-emerald-50 border border-emerald-100 rounded-xl text-xs text-emerald-700 flex items-center justify-between gap-2">
				<div class="flex items-center gap-2">
					<CheckIcon class="w-4 h-4 text-emerald-600" />
					<span>Knowledge base purged — {purgeResult.concepts} concept{purgeResult.concepts !== 1 ? 's' : ''} and {purgeResult.patterns} pattern{purgeResult.patterns !== 1 ? 's' : ''} removed.</span>
				</div>
				<button type="button" onclick={() => purgeResult = null} class="text-emerald-500 hover:text-emerald-700 p-0.5 rounded cursor-pointer" aria-label="Dismiss banner">
					<XMarkIcon class="w-3.5 h-3.5" />
				</button>
			</div>
		{:else if purgeError}
			<div class="mt-3 px-3.5 py-2.5 bg-rose-50 border border-rose-100 rounded-xl text-xs text-rose-700 flex items-center justify-between gap-2">
				<div class="flex items-center gap-2">
					<XMarkIcon class="w-4 h-4 text-rose-600" />
					<span>{purgeError}</span>
				</div>
				<button type="button" onclick={() => purgeError = ''} class="text-rose-500 hover:text-rose-700 p-0.5 rounded cursor-pointer" aria-label="Dismiss banner">
					<XMarkIcon class="w-3.5 h-3.5" />
				</button>
			</div>
		{/if}

		<nav class="flex gap-1 mt-4" aria-label="Knowledge sections">
			{#each [{ key: 'concepts', label: 'KB Concepts', count: filteredConcepts.length }, { key: 'patterns', label: 'Patterns', count: filteredPatterns.length }, { key: 'suggestions', label: 'AI Suggestions', count: filteredSuggestions.length }] as tab}
				<button onclick={() => activeTab = tab.key as typeof activeTab} class="flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg text-xs font-medium transition-all cursor-pointer {activeTab === tab.key ? 'bg-blue-50 text-blue-600' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-50'}">
					<span>{tab.label}</span>
					<span class="px-1.5 py-0.5 rounded-md text-[10px] font-medium {activeTab === tab.key ? 'bg-blue-100 text-blue-600' : 'bg-slate-100 text-slate-500'}">{tab.count}</span>
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
						<div>
							<div class="text-sm font-medium text-slate-800">Review structured knowledge</div>
							<div class="text-[11px] text-slate-500">The same concept and deterministic-pattern review used during onboarding.</div>
						</div>
						<div class="flex items-center gap-2">
							<button onclick={discardIngestion} disabled={pasting} class="px-3 py-1.5 rounded-xl border border-slate-200 hover:bg-slate-100 text-slate-600 text-xs font-medium transition cursor-pointer disabled:opacity-50">
								Discard
							</button>
							<button onclick={publishIngestion} disabled={pasting} class="px-3.5 py-1.5 rounded-xl bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium transition disabled:opacity-50 cursor-pointer">
								{pasting ? 'Publishing…' : 'Add selected to Knowledge Base'}
							</button>
						</div>
					</div>
					{#if pasteResult?.error}
						<div class="mb-3 flex items-center gap-1.5 text-xs text-rose-600 font-medium">
							<XMarkIcon class="w-3.5 h-3.5" />
							<span>{pasteResult.error}</span>
						</div>
					{/if}
					<div class="max-h-[52vh] overflow-y-auto pr-1"><IngestionReview bind:concepts={ingestionConcepts} bind:patterns={ingestionPatterns} /></div>
				{:else}
					<div class="text-xs font-medium text-slate-700 mb-2">Add business knowledge</div>
					<textarea bind:value={pasteText} disabled={pasting} placeholder="Paste business information, pricing, business hours, and policies. The system extracts concepts and answer patterns." class="w-full h-20 p-3 text-xs text-slate-700 placeholder-slate-400 bg-slate-50 rounded-xl border border-slate-200 focus:outline-none focus:border-blue-400 resize-none leading-relaxed disabled:opacity-60"></textarea>
					<div class="flex items-center justify-between mt-2">
						<div class="flex items-center gap-2">
							{#if pasteResult?.added !== undefined}
								<div class="flex items-center gap-1.5 text-xs text-emerald-600 font-medium">
									<CheckIcon class="w-3.5 h-3.5" />
									<span>{pasteResult.added} concept{pasteResult.added !== 1 ? 's' : ''} and {pasteResult.patternsAdded ?? 0} pattern{pasteResult.patternsAdded !== 1 ? 's' : ''} added</span>
								</div>
							{:else if pasteResult?.error}
								<div class="flex items-center gap-1.5 text-xs text-rose-600 font-medium">
									<XMarkIcon class="w-3.5 h-3.5" />
									<span>{pasteResult.error}</span>
								</div>
							{:else if ingestionPhase === 'processing'}
								<div class="flex items-center gap-1.5 text-xs text-blue-600 font-medium">
									<span class="w-3.5 h-3.5 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></span>
									<span>Extracting concepts and answer patterns…</span>
								</div>
							{:else if ingestionPhase === 'publishing'}
								<div class="flex items-center gap-1.5 text-xs text-blue-600 font-medium">
									<span class="w-3.5 h-3.5 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></span>
									<span>Publishing reviewed knowledge…</span>
								</div>
							{/if}
						</div>
						<button onclick={compilePaste} disabled={pasting || !pasteText.trim()} class="flex items-center gap-1.5 px-3.5 py-1.5 rounded-xl bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium transition disabled:opacity-50 cursor-pointer">
							<SparklesIcon class="w-3.5 h-3.5 text-white" />
							<span>{pasting ? 'Processing…' : 'Extract with AI'}</span>
						</button>
					</div>
				{/if}
			</div>

			<div class="flex-1 overflow-y-auto px-6 py-4 space-y-2">
				{#if filteredConcepts.length === 0}
					<div class="flex flex-col items-center justify-center py-16 text-center">
						<div class="w-10 h-10 rounded-2xl bg-slate-100 flex items-center justify-center text-slate-400 mb-3">
							<BookOpenIcon class="w-5 h-5" />
						</div>
						<div class="text-sm font-medium text-slate-600">
							{searchQuery.trim() ? 'No matching knowledge concepts' : 'No knowledge concepts found'}
						</div>
						<div class="text-xs text-slate-400 mt-1 max-w-sm">
							{searchQuery.trim() ? 'Try adjusting your search terms' : 'Paste business information above and click "Extract with AI"'}
						</div>
					</div>
				{:else}
					{#each filteredConcepts as concept (concept.id)}
						<div class="border border-slate-200/80 rounded-xl overflow-hidden bg-white">
							<button onclick={() => expandedConcept = expandedConcept === concept.id ? null : concept.id} class="w-full px-4 py-3 flex items-center justify-between text-left hover:bg-slate-50/60 transition cursor-pointer">
								<div class="flex items-center gap-2.5 min-w-0">
									<span class="px-2 py-0.5 rounded text-[10px] font-medium border capitalize {typeColor(concept.type)}">{typeLabel(concept.type)}</span>
									<span class="text-xs font-medium text-slate-800 truncate">{concept.title}</span>
									{#if concept.source === 'owner_pasted'}
										<span class="text-[10px] text-slate-400 bg-slate-100 px-1.5 py-0.5 rounded">pasted</span>
									{/if}
									{#if concept.tags?.length}
										<div class="hidden md:flex items-center gap-1 ml-1">
											{#each concept.tags as tag}
												<span class="text-[10px] text-slate-400 bg-slate-50 border border-slate-100 px-1.5 py-0.2 rounded">#{tag}</span>
											{/each}
										</div>
									{/if}
								</div>
								<div class="text-slate-400 transition-transform duration-200 {expandedConcept === concept.id ? 'rotate-180' : ''}">
									<ChevronDownIcon class="w-4 h-4" />
								</div>
							</button>
							{#if expandedConcept === concept.id}
								<div class="px-4 pb-4 pt-1 border-t border-slate-100 bg-slate-50/40 text-xs space-y-2">
									<div class="text-slate-700 leading-relaxed whitespace-pre-wrap">{concept.body_markdown}</div>
									<div class="flex items-center justify-between pt-2 text-[11px] text-slate-400 border-t border-slate-100">
										<span>Added {formatDate(concept.created_at)}</span>
										<button onclick={() => deleteConcept(concept.id)} class="text-rose-500 hover:text-rose-700 transition cursor-pointer flex items-center gap-1">
											<TrashIcon class="w-3.5 h-3.5" />
											<span>Delete</span>
										</button>
									</div>
								</div>
							{/if}
						</div>
					{/each}
				{/if}
			</div>
		</div>
	{:else if activeTab === 'patterns'}
		<div class="flex-1 overflow-y-auto px-6 py-4 space-y-3">
			{#if filteredPatterns.length === 0}
				<div class="flex flex-col items-center justify-center py-16 text-center">
					<div class="w-10 h-10 rounded-2xl bg-blue-50 flex items-center justify-center text-blue-500 mb-3">
						<ChatBubbleLeftRightIcon class="w-5 h-5" />
					</div>
					<div class="text-sm font-medium text-slate-600">
						{searchQuery.trim() ? 'No matching answer patterns' : 'No deterministic answer patterns yet'}
					</div>
					<div class="text-xs text-slate-400 mt-1 max-w-sm">
						{searchQuery.trim() ? 'Try adjusting your search terms' : 'Organize business knowledge or run an AI audit to create common question patterns'}
					</div>
				</div>
			{:else}
				{#each filteredPatterns as pattern (pattern.id)}
					<div class="p-4 rounded-xl border border-slate-200/80 bg-white space-y-2.5">
						<div class="flex items-start justify-between gap-3">
							<div class="text-xs font-medium text-slate-900">{pattern.canonical_question}</div>
							<span class="px-2 py-0.5 rounded text-[10px] font-medium bg-blue-50 text-blue-700 border border-blue-100/80 shrink-0">Pattern</span>
						</div>
						{#if pattern.trigger_phrases?.length}
							<div class="flex flex-wrap items-center gap-1 pt-0.5">
								<span class="text-[10px] font-medium text-slate-400 uppercase tracking-wide mr-1">Triggers:</span>
								{#each pattern.trigger_phrases as phrase}
									<span class="text-[10px] text-slate-600 bg-slate-100 px-2 py-0.5 rounded-md font-mono">{phrase}</span>
								{/each}
							</div>
						{/if}
						<div class="text-xs text-slate-600 leading-relaxed whitespace-pre-wrap bg-slate-50/70 p-3 rounded-lg border border-slate-100">{pattern.answer_markdown}</div>
						<div class="flex justify-end pt-1 text-[11px]">
							<button onclick={() => deletePattern(pattern.id)} class="text-rose-500 hover:text-rose-700 transition cursor-pointer flex items-center gap-1">
								<TrashIcon class="w-3.5 h-3.5" />
								<span>Delete</span>
							</button>
						</div>
					</div>
				{/each}
			{/if}
		</div>
	{:else}
		<div class="flex-1 overflow-y-auto px-6 py-4 space-y-3">
			{#if filteredSuggestions.length === 0}
				<div class="flex flex-col items-center justify-center py-16 text-center">
					<div class="w-10 h-10 rounded-2xl bg-amber-50 flex items-center justify-center text-amber-500 mb-3">
						<SparklesIcon class="w-5 h-5" />
					</div>
					<div class="text-sm font-medium text-slate-600">
						{searchQuery.trim() ? 'No matching suggestions' : 'No suggestions pending review'}
					</div>
					<div class="text-xs text-slate-400 mt-1 max-w-sm">
						{searchQuery.trim() ? 'Try adjusting your search terms' : 'When AI audits find knowledge gaps in conversations, recommendations will appear here'}
					</div>
				</div>
			{:else}
				{#each filteredSuggestions as suggestion (suggestion.id)}
					<div class="p-4 rounded-xl border border-slate-200/80 bg-white space-y-3">
						<div class="flex items-center justify-between gap-2">
							<div class="flex items-center gap-2 min-w-0">
								<span class="px-2 py-0.5 rounded text-[10px] font-medium border capitalize {typeColor(suggestion._payload?.type ?? suggestion.type)}">{typeLabel(suggestion._payload?.type ?? suggestion.type)}</span>
								<span class="text-xs font-medium text-slate-800 truncate">{suggestion._payload?.title ?? suggestion._payload?.canonical_question ?? 'Untitled suggestion'}</span>
							</div>
							<span class="px-2 py-0.5 rounded-md text-[10px] font-medium bg-blue-50 text-blue-700 border border-blue-100 shrink-0">
								{Math.round((suggestion.confidence ?? 0) * 100)}% match
							</span>
						</div>
						<div class="text-xs text-slate-600 bg-slate-50 p-3 rounded-xl leading-relaxed whitespace-pre-wrap border border-slate-100">{suggestion._payload?.body_markdown ?? suggestion._payload?.answer_markdown ?? ''}</div>
						<div class="flex items-center justify-end gap-2 pt-1 text-xs">
							<button onclick={() => reviewSuggestion(suggestion.id, 'reject')} class="px-3 py-1.5 text-slate-600 hover:text-slate-900 hover:bg-slate-100 rounded-xl transition cursor-pointer font-medium">
								Dismiss
							</button>
							<button onclick={() => reviewSuggestion(suggestion.id, 'approve')} class="px-3.5 py-1.5 bg-blue-600 hover:bg-blue-700 text-white font-medium rounded-xl transition cursor-pointer shadow-xs">
								Add to Knowledge Base
							</button>
						</div>
					</div>
				{/each}
			{/if}
		</div>
	{/if}
</div>
