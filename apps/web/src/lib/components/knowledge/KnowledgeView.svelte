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
		ChatBubbleLeftRightIcon,
		PencilSquareIcon,
		PlusIcon
	} from '@fvilers/heroicons-svelte/24/outline';
	import IngestionReview from '$lib/components/knowledge/IngestionReview.svelte';
	import { KnowledgeIngestionController } from '$lib/knowledge/ingestion-controller.svelte';

	let {
		reviewerID = '',
		searchQuery = '',
		autoReplyEnabled = false,
		providerConfigured = false,
		canManageAI = false,
		togglingAI = false,
		onToggleAI = () => {}
	}: {
		reviewerID?: string;
		searchQuery?: string;
		autoReplyEnabled?: boolean;
		providerConfigured?: boolean;
		canManageAI?: boolean;
		togglingAI?: boolean;
		onToggleAI?: () => void;
	} = $props();

	let concepts = $state<any[]>([]);
	let patterns = $state<any[]>([]);
	let suggestions = $state<any[]>([]);
	let lastRun = $state<any>(null);
	let loading = $state(true);
	let activeTab = $state<'concepts' | 'patterns' | 'suggestions'>('concepts');
	let pasteText = $state('');
	let pasteResult = $state<{ added?: number; patternsAdded?: number; error?: string } | null>(null);
	const ingestion = new KnowledgeIngestionController();
	let expandedConcepts = $state<Record<string, boolean>>({});
	let mining = $state(false);
	let miningResult = $state<{ messages_scanned?: number; clusters_found?: number; suggestions_created?: number } | null>(null);
	let purging = $state(false);
	let purgeResult = $state<{ concepts: number; patterns: number } | null>(null);
	let purgeError = $state('');

	// Inline editing state for concepts
	let editingConceptId = $state<string | null>(null);
	let editConceptDraft = $state<{ title: string; type: string; tags: string[]; body_text: string }>({
		title: '',
		type: 'faq',
		tags: [],
		body_text: ''
	});
	let editConceptTagInput = $state('');
	let savingConcept = $state(false);
	let saveConceptError = $state('');

	// Inline editing state for patterns
	let editingPatternId = $state<string | null>(null);
	let editPatternDraft = $state<{ canonical_question: string; answer_text: string; trigger_phrases: string[] }>({
		canonical_question: '',
		answer_text: '',
		trigger_phrases: []
	});
	let editPatternTriggerInput = $state('');
	let savingPattern = $state(false);
	let savePatternError = $state('');

	let filteredConcepts = $derived(
		!searchQuery.trim()
			? concepts
			: concepts.filter((c) =>
					(c.title || '').toLowerCase().includes(searchQuery.toLowerCase()) ||
					(c.body_text || '').toLowerCase().includes(searchQuery.toLowerCase()) ||
					(c.type || '').toLowerCase().includes(searchQuery.toLowerCase()) ||
					(c.tags || []).some((t: string) => t.toLowerCase().includes(searchQuery.toLowerCase()))
				)
	);

	let filteredPatterns = $derived(
		!searchQuery.trim()
			? patterns
			: patterns.filter((p) =>
					(p.canonical_question || '').toLowerCase().includes(searchQuery.toLowerCase()) ||
					(p.answer_text || '').toLowerCase().includes(searchQuery.toLowerCase()) ||
					(p.trigger_phrases || []).some((phrase: string) => phrase.toLowerCase().includes(searchQuery.toLowerCase()))
				)
	);

	let filteredSuggestions = $derived(
		!searchQuery.trim()
			? suggestions
			: suggestions.filter((s) => {
					const title = s._payload?.title || s._payload?.canonical_question || '';
					const body = s._payload?.body_text || s._payload?.answer_text || '';
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
	onDestroy(() => ingestion.dispose());

	// Concept Actions
	function startEditingConcept(concept: any) {
		editingConceptId = concept.id;
		editConceptDraft = {
			title: concept.title || '',
			type: concept.type || 'faq',
			tags: Array.isArray(concept.tags) ? [...concept.tags] : [],
			body_text: concept.body_text || ''
		};
		editConceptTagInput = '';
		saveConceptError = '';
	}

	function cancelEditingConcept() {
		editingConceptId = null;
		saveConceptError = '';
	}

	function addTagToConceptDraft() {
		const val = editConceptTagInput.trim().replace(/^#/, '');
		if (!val) return;
		if (!editConceptDraft.tags.includes(val)) {
			editConceptDraft.tags = [...editConceptDraft.tags, val];
		}
		editConceptTagInput = '';
	}

	function removeTagFromConceptDraft(tag: string) {
		editConceptDraft.tags = editConceptDraft.tags.filter((t) => t !== tag);
	}

	async function saveConcept(id: string) {
		if (!editConceptDraft.title.trim()) {
			saveConceptError = 'Title is required';
			return;
		}
		if (!editConceptDraft.body_text.trim()) {
			saveConceptError = 'Content is required';
			return;
		}
		savingConcept = true;
		saveConceptError = '';
		try {
			const res = await apiRequest(`/api/kb/concepts/${id}`, {
				method: 'PUT',
				body: editConceptDraft
			});
			const updated = res.concept || { ...editConceptDraft, id };
			concepts = concepts.map((c) => (c.id === id ? { ...c, ...updated } : c));
			editingConceptId = null;
		} catch (err: any) {
			saveConceptError = err.message || 'Failed to save concept changes';
		} finally {
			savingConcept = false;
		}
	}

	async function deleteConcept(id: string) {
		if (!confirm('Delete this knowledge concept?')) return;
		await apiRequest(`/api/kb/concepts/${id}`, { method: 'DELETE' });
		concepts = concepts.filter((concept) => concept.id !== id);
		if (editingConceptId === id) editingConceptId = null;
	}

	// Pattern Actions
	function startEditingPattern(pattern: any) {
		editingPatternId = pattern.id;
		editPatternDraft = {
			canonical_question: pattern.canonical_question || '',
			answer_text: pattern.answer_text || '',
			trigger_phrases: Array.isArray(pattern.trigger_phrases) ? [...pattern.trigger_phrases] : []
		};
		editPatternTriggerInput = '';
		savePatternError = '';
	}

	function cancelEditingPattern() {
		editingPatternId = null;
		savePatternError = '';
	}

	function addTriggerToPatternDraft() {
		const val = editPatternTriggerInput.trim();
		if (!val) return;
		if (!editPatternDraft.trigger_phrases.includes(val)) {
			editPatternDraft.trigger_phrases = [...editPatternDraft.trigger_phrases, val];
		}
		editPatternTriggerInput = '';
	}

	function removeTriggerFromPatternDraft(phrase: string) {
		editPatternDraft.trigger_phrases = editPatternDraft.trigger_phrases.filter((t) => t !== phrase);
	}

	async function savePattern(id: string) {
		if (!editPatternDraft.canonical_question.trim()) {
			savePatternError = 'Canonical question is required';
			return;
		}
		if (!editPatternDraft.answer_text.trim()) {
			savePatternError = 'Answer text is required';
			return;
		}
		savingPattern = true;
		savePatternError = '';
		try {
			const res = await apiRequest(`/api/kb/patterns/${id}`, {
				method: 'PUT',
				body: editPatternDraft
			});
			const updated = res.pattern || { ...editPatternDraft, id };
			patterns = patterns.map((p) => (p.id === id ? { ...p, ...updated } : p));
			editingPatternId = null;
		} catch (err: any) {
			savePatternError = err.message || 'Failed to save pattern changes';
		} finally {
			savingPattern = false;
		}
	}

	async function deletePattern(id: string) {
		if (!confirm('Delete this pattern?')) return;
		await apiRequest(`/api/kb/patterns/${id}`, { method: 'DELETE' });
		patterns = patterns.filter((pattern) => pattern.id !== id);
		if (editingPatternId === id) editingPatternId = null;
	}

	async function purgeKnowledgeBase() {
		if (ingestion.phase !== 'idle') return;
		if (!confirm('Permanently delete all concepts and deterministic patterns in this workspace? This cannot be undone.')) return;

		purging = true;
		purgeError = '';
		purgeResult = null;
		try {
			const result = await apiRequest('/api/kb/purge', { method: 'DELETE' });
			concepts = [];
			patterns = [];
			editingConceptId = null;
			editingPatternId = null;
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
		ingestion.discard();
		pasteResult = null;
	}

	async function handleIngestionResult(result: Awaited<ReturnType<KnowledgeIngestionController['start']>>) {
		if (result.status !== 'complete') return;
		pasteResult = { added: result.conceptsAdded, patternsAdded: result.patternsAdded };
		pasteText = '';
		await load(true);
	}

	async function resumeLatestIngestion() {
		try {
			const result = await ingestion.resumeLatest();
			if (result) await handleIngestionResult(result);
		} catch (error: any) {
			pasteResult = { error: error.message || 'Failed to resume knowledge ingestion' };
		}
	}

	async function compilePaste() {
		if (!pasteText.trim()) return;
		pasteResult = null;
		try {
			await handleIngestionResult(await ingestion.start(pasteText));
		} catch (error: any) {
			pasteResult = { error: error.message || 'Failed to compile' };
		}
	}

	async function publishIngestion() {
		pasteResult = null;
		try {
			await handleIngestionResult(await ingestion.publish());
		} catch (error: any) {
			pasteResult = { error: error.message || 'Failed to publish knowledge' };
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
			faq: 'bg-blue-50 text-blue-700 border-blue-200/70',
			pricing: 'bg-emerald-50 text-emerald-700 border-emerald-200/70',
			policy: 'bg-amber-50 text-amber-700 border-amber-200/70',
			hours: 'bg-purple-50 text-purple-700 border-purple-200/70',
			service: 'bg-rose-50 text-rose-700 border-rose-200/70'
		} as Record<string, string>)[(type || '').toLowerCase()] || 'bg-slate-50 text-slate-700 border-slate-200/70';
	}

	function toggleConceptExpansion(id: string) {
		expandedConcepts[id] = !expandedConcepts[id];
	}
</script>

<div class="flex-1 flex flex-col overflow-hidden bg-white">
	<!-- Top Level Header -->
	<header class="px-6 py-4 border-b border-slate-100 shrink-0 space-y-3.5">
		<!-- Row 1: Title & Primary Actions -->
		<div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
			<div>
				<h1 class="text-xl font-semibold text-slate-900 tracking-tight">Knowledge base</h1>
				<p class="text-xs text-slate-500 mt-0.5">Manage pricing, FAQs, services, and policies for AI answers.</p>
			</div>

			<div class="flex flex-wrap items-center gap-2.5">
				<!-- Audit Run Action -->
				<button
					onclick={triggerMining}
					disabled={mining}
					class="flex items-center gap-1.5 px-3 py-1.5 rounded-xl border border-slate-200 bg-white hover:bg-slate-50 text-xs font-medium text-slate-700 shadow-2xs transition active:scale-[0.98] disabled:opacity-50 cursor-pointer"
					title={lastRun ? `Last audit: ${formatDate(lastRun.run_at)} (${lastRun.messages_scanned} msgs)` : 'Analyze recent chats for missing knowledge'}
				>
					<SparklesIcon class="w-3.5 h-3.5 text-blue-600" />
					<span>{mining ? 'Scanning…' : 'Run audit now'}</span>
				</button>

				<!-- Purge Action (quiet danger button) -->
				<button
					onclick={purgeKnowledgeBase}
					disabled={purging || ingestion.phase !== 'idle'}
					class="flex items-center gap-1.5 px-3 py-1.5 rounded-xl border border-slate-200 hover:border-rose-200 bg-white hover:bg-rose-50/70 text-xs font-medium text-slate-500 hover:text-rose-600 shadow-2xs transition active:scale-[0.98] disabled:opacity-40 cursor-pointer"
					title="Permanently remove all concepts and deterministic patterns"
				>
					<TrashIcon class="w-3.5 h-3.5" />
					<span>{purging ? 'Purging…' : 'Purge knowledge base'}</span>
				</button>
			</div>
		</div>

		<!-- Row 2: Sub-navigation & Workspace Governance Controls -->
		<div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 pt-0.5">
			<!-- Segmented Sub-Tabs -->
			<nav class="inline-flex p-1 bg-slate-100/90 rounded-xl border border-slate-200/60 self-start" aria-label="Knowledge sections">
				{#each [{ key: 'concepts', label: 'KB Concepts', count: filteredConcepts.length }, { key: 'patterns', label: 'Patterns', count: filteredPatterns.length }, { key: 'suggestions', label: 'AI Suggestions', count: filteredSuggestions.length }] as tab}
					<button
						onclick={() => (activeTab = tab.key as typeof activeTab)}
						class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-all cursor-pointer {activeTab === tab.key ? 'bg-white text-slate-900 shadow-2xs font-semibold' : 'text-slate-500 hover:text-slate-800'}"
					>
						<span>{tab.label}</span>
						<span class="px-1.5 py-0.5 rounded-md text-[10px] font-medium {activeTab === tab.key ? 'bg-slate-100 text-slate-800' : 'bg-slate-200/60 text-slate-500'}">
							{tab.count}
						</span>
						{#if tab.key === 'suggestions' && filteredSuggestions.length > 0}
							<span class="w-1.5 h-1.5 rounded-full bg-amber-500 ml-0.5"></span>
						{/if}
					</button>
				{/each}
			</nav>

			<!-- Right: Global Auto-reply Switch & Audit Indicator -->
			<div class="flex items-center gap-2.5 self-start sm:self-auto">
				{#if lastRun}
					<div
						class="hidden md:flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-slate-50 border border-slate-200/70 text-[11px] text-slate-500"
						title={`${lastRun.messages_scanned} messages scanned · ${lastRun.clusters_found} clusters found · ${lastRun.suggestions_created} suggestions created`}
					>
						<span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
						<span>Audited {formatDate(lastRun.run_at)}</span>
					</div>
				{/if}

				<!-- Toggle Switch Component -->
				<button
					type="button"
					role="switch"
					aria-label="Global AI auto-reply default"
					aria-checked={autoReplyEnabled && providerConfigured}
					onclick={onToggleAI}
					disabled={!canManageAI || togglingAI || (!providerConfigured && !autoReplyEnabled)}
					class="h-8 flex items-center gap-2 px-2.5 rounded-xl border border-slate-200 bg-white hover:bg-slate-50 text-xs font-medium text-slate-700 transition shadow-2xs cursor-pointer disabled:cursor-default disabled:opacity-60 active:scale-[0.98]"
					title={!providerConfigured ? 'Configure an AI provider in Settings before enabling automatic replies' : 'New chats inherit this setting unless they have a chat override'}
				>
					<span
						class="relative inline-flex h-4 w-7 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out {autoReplyEnabled && providerConfigured ? 'bg-emerald-500' : 'bg-slate-300'}"
					>
						<span
							class="inline-block h-3 w-3 transform rounded-full bg-white shadow transition duration-200 ease-in-out {autoReplyEnabled && providerConfigured ? 'translate-x-3' : 'translate-x-0'}"
						></span>
					</span>
					<span>Global AI auto-reply</span>
					<span class="text-[10px] font-semibold {autoReplyEnabled && providerConfigured ? 'text-emerald-600' : 'text-slate-400'}">
						{autoReplyEnabled && providerConfigured ? 'ON' : 'OFF'}
					</span>
				</button>
			</div>
		</div>

		<!-- Feedback Notices -->
		{#if miningResult}
			<div class="px-3.5 py-2.5 bg-blue-50/80 border border-blue-200/80 rounded-xl text-xs text-blue-800 flex items-center justify-between gap-2 shadow-2xs">
				<div class="flex items-center gap-2">
					<SparklesIcon class="w-4 h-4 text-blue-600 shrink-0" />
					<span>Audit complete — {miningResult.messages_scanned} messages scanned, {miningResult.clusters_found} clusters found, {miningResult.suggestions_created} suggestions created.</span>
				</div>
				<button type="button" onclick={() => (miningResult = null)} class="text-blue-500 hover:text-blue-700 p-0.5 rounded cursor-pointer" aria-label="Dismiss banner">
					<XMarkIcon class="w-4 h-4" />
				</button>
			</div>
		{/if}
		{#if purgeResult}
			<div class="px-3.5 py-2.5 bg-emerald-50/80 border border-emerald-200/80 rounded-xl text-xs text-emerald-800 flex items-center justify-between gap-2 shadow-2xs">
				<div class="flex items-center gap-2">
					<CheckIcon class="w-4 h-4 text-emerald-600 shrink-0" />
					<span>Knowledge base purged — {purgeResult.concepts} concept{purgeResult.concepts !== 1 ? 's' : ''} and {purgeResult.patterns} pattern{purgeResult.patterns !== 1 ? 's' : ''} removed.</span>
				</div>
				<button type="button" onclick={() => (purgeResult = null)} class="text-emerald-500 hover:text-emerald-700 p-0.5 rounded cursor-pointer" aria-label="Dismiss banner">
					<XMarkIcon class="w-4 h-4" />
				</button>
			</div>
		{:else if purgeError}
			<div class="px-3.5 py-2.5 bg-rose-50/80 border border-rose-200/80 rounded-xl text-xs text-rose-800 flex items-center justify-between gap-2 shadow-2xs">
				<div class="flex items-center gap-2">
					<XMarkIcon class="w-4 h-4 text-rose-600 shrink-0" />
					<span>{purgeError}</span>
				</div>
				<button type="button" onclick={() => (purgeError = '')} class="text-rose-500 hover:text-rose-700 p-0.5 rounded cursor-pointer" aria-label="Dismiss banner">
					<XMarkIcon class="w-4 h-4" />
				</button>
			</div>
		{/if}
	</header>

	{#if loading}
		<div class="flex-1 flex items-center justify-center">
			<span class="w-5 h-5 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></span>
		</div>
	{:else if activeTab === 'concepts'}
		<div class="flex-1 overflow-y-auto min-h-0 flex flex-col">
			<!-- Ingestion Review / Paste Composer Area -->
			<div class="px-6 py-4 border-b border-slate-100 shrink-0">
				{#if ingestion.phase === 'review'}
					<div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-3">
						<div>
							<div class="text-sm font-semibold text-slate-900">Review structured knowledge</div>
							<div class="text-xs text-slate-500 mt-0.5">The same concept and deterministic-pattern review used during onboarding.</div>
						</div>
						<div class="flex items-center gap-2">
							<button onclick={discardIngestion} disabled={ingestion.busy} class="px-3 py-1.5 rounded-xl border border-slate-200 hover:bg-slate-100 text-slate-600 text-xs font-medium transition cursor-pointer disabled:opacity-50">
								Discard
							</button>
							<button onclick={publishIngestion} disabled={ingestion.busy} class="px-3.5 py-1.5 rounded-xl bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium transition disabled:opacity-50 cursor-pointer shadow-xs active:scale-[0.98]">
								{ingestion.busy ? 'Publishing…' : 'Add selected to Knowledge Base'}
							</button>
						</div>
					</div>
					{#if pasteResult?.error}
						<div class="mb-3 flex items-center gap-1.5 text-xs text-rose-600 font-medium">
							<XMarkIcon class="w-4 h-4" />
							<span>{pasteResult.error}</span>
						</div>
					{/if}
					<div class="pr-1"><IngestionReview bind:concepts={ingestion.concepts} bind:patterns={ingestion.patterns} /></div>
				{:else}
					<div class="bg-slate-50/70 border border-slate-200/80 rounded-2xl p-4 shadow-2xs">
						<div class="flex items-center justify-between mb-2">
							<h2 class="text-xs font-semibold text-slate-700 uppercase tracking-wider">Add business knowledge</h2>
							<span class="text-[11px] text-slate-400">AI-powered extraction</span>
						</div>
						<textarea
							bind:value={pasteText}
							disabled={ingestion.busy}
							placeholder="Paste business information, pricing, business hours, and policies. The system extracts concepts and answer patterns."
							class="w-full h-20 p-3 text-xs text-slate-700 placeholder-slate-400 bg-white rounded-xl border border-slate-200 focus:outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 resize-none leading-relaxed disabled:opacity-60 transition"
						></textarea>
						<div class="flex items-center justify-between mt-2.5">
							<div class="flex items-center gap-2">
								{#if pasteResult?.added !== undefined}
									<div class="flex items-center gap-1.5 text-xs text-emerald-600 font-medium">
										<CheckIcon class="w-4 h-4" />
										<span>{pasteResult.added} concept{pasteResult.added !== 1 ? 's' : ''} and {pasteResult.patternsAdded ?? 0} pattern{pasteResult.patternsAdded !== 1 ? 's' : ''} added</span>
									</div>
								{:else if pasteResult?.error}
									<div class="flex items-center gap-1.5 text-xs text-rose-600 font-medium">
										<XMarkIcon class="w-4 h-4" />
										<span>{pasteResult.error}</span>
									</div>
								{:else if ingestion.phase === 'processing'}
									<div class="flex items-center gap-1.5 text-xs text-blue-600 font-medium">
										<span class="w-3.5 h-3.5 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></span>
										<span>Extracting concepts and answer patterns…</span>
									</div>
								{:else if ingestion.phase === 'publishing'}
									<div class="flex items-center gap-1.5 text-xs text-blue-600 font-medium">
										<span class="w-3.5 h-3.5 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></span>
										<span>Publishing reviewed knowledge…</span>
									</div>
								{/if}
							</div>
							<button
								onclick={compilePaste}
								disabled={ingestion.busy || !pasteText.trim()}
								class="flex items-center gap-1.5 px-3.5 py-1.5 rounded-xl bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium transition disabled:opacity-50 cursor-pointer shadow-xs active:scale-[0.98]"
							>
								<SparklesIcon class="w-3.5 h-3.5 text-white" />
								<span>{ingestion.busy ? 'Processing…' : 'Extract with AI'}</span>
							</button>
						</div>
					</div>
				{/if}
			</div>

			<!-- Concepts List -->
			<div class="flex-1 overflow-y-auto px-6 py-4">
				{#if filteredConcepts.length === 0}
					<div class="flex flex-col items-center justify-center py-16 text-center">
						<div class="w-10 h-10 rounded-2xl bg-slate-100 flex items-center justify-center text-slate-400 mb-3">
							<BookOpenIcon class="w-5 h-5" />
						</div>
						<div class="text-sm font-medium text-slate-700">
							{searchQuery.trim() ? 'No matching knowledge concepts' : 'No knowledge concepts found'}
						</div>
						<div class="text-xs text-slate-400 mt-1 max-w-sm">
							{searchQuery.trim() ? 'Try adjusting your search terms' : 'Paste business information above and click "Extract with AI"'}
						</div>
					</div>
				{:else}
					<div class="grid grid-cols-1 md:grid-cols-2 gap-3.5 items-start">
						{#each filteredConcepts as concept (concept.id)}
							{#if editingConceptId === concept.id}
								<!-- Inline Concept Editor -->
								<div class="col-span-1 md:col-span-2 border-2 border-blue-500/60 rounded-2xl p-4 bg-white shadow-sm space-y-3 transition">
									<div class="flex items-center justify-between gap-2 border-b border-slate-100 pb-2.5">
										<span class="text-xs font-semibold text-slate-900 uppercase tracking-wider">Edit Concept</span>
										{#if saveConceptError}
											<span class="text-xs text-rose-600 font-medium">{saveConceptError}</span>
										{/if}
									</div>
									<div class="grid grid-cols-1 sm:grid-cols-4 gap-2.5">
										<div class="sm:col-span-3">
											<label for={`edit-concept-title-${concept.id}`} class="block text-[11px] font-medium text-slate-500 mb-1">Title</label>
											<input
												id={`edit-concept-title-${concept.id}`}
												bind:value={editConceptDraft.title}
												placeholder="Concept title"
												class="w-full bg-slate-50/50 focus:bg-white border border-slate-200 rounded-xl px-3 py-1.5 text-xs font-semibold text-slate-900 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 transition"
											/>
										</div>
										<div>
											<label for={`edit-concept-type-${concept.id}`} class="block text-[11px] font-medium text-slate-500 mb-1">Category</label>
											<select
												id={`edit-concept-type-${concept.id}`}
												bind:value={editConceptDraft.type}
												class="w-full bg-slate-50/50 focus:bg-white border border-slate-200 rounded-xl px-2.5 py-1.5 text-xs font-medium text-slate-800 outline-none focus:border-blue-500 capitalize transition"
											>
												<option value="faq">FAQ</option>
												<option value="pricing">Pricing</option>
												<option value="policy">Policy</option>
												<option value="hours">Hours</option>
												<option value="service">Service</option>
												<option value="general">General</option>
											</select>
										</div>
									</div>
									<div>
										<label for={`edit-concept-body-${concept.id}`} class="block text-[11px] font-medium text-slate-500 mb-1">Knowledge Content</label>
										<textarea
											id={`edit-concept-body-${concept.id}`}
											bind:value={editConceptDraft.body_text}
											rows="4"
											class="w-full bg-slate-50/50 focus:bg-white border border-slate-200 rounded-xl p-3 text-xs text-slate-700 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 leading-relaxed transition"
										></textarea>
									</div>
									<div>
										<label for={`edit-concept-tag-input-${concept.id}`} class="block text-[11px] font-medium text-slate-500 mb-1">Tags</label>
										<div class="flex flex-wrap items-center gap-1.5 mb-1.5">
											{#each editConceptDraft.tags as tag}
												<span class="inline-flex items-center gap-1 text-[11px] text-slate-700 bg-slate-100 border border-slate-200/80 px-2 py-0.5 rounded-md font-medium">
													<span>{tag}</span>
													<button type="button" onclick={() => removeTagFromConceptDraft(tag)} class="text-slate-400 hover:text-rose-600 cursor-pointer">×</button>
												</span>
											{/each}
										</div>
										<div class="flex items-center gap-2">
											<input
												id={`edit-concept-tag-input-${concept.id}`}
												bind:value={editConceptTagInput}
												onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), addTagToConceptDraft())}
												placeholder="Add tag and press Enter"
												class="flex-1 max-w-xs bg-slate-50/50 focus:bg-white border border-slate-200 rounded-xl px-2.5 py-1 text-xs text-slate-700 outline-none focus:border-blue-500 transition"
											/>
											<button type="button" onclick={addTagToConceptDraft} class="px-2.5 py-1 bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl text-xs font-medium cursor-pointer transition">Add</button>
										</div>
									</div>
									<div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
										<button type="button" onclick={cancelEditingConcept} class="px-3 py-1.5 text-xs font-medium text-slate-600 hover:text-slate-900 rounded-xl hover:bg-slate-100 transition cursor-pointer">
											Cancel
										</button>
										<button
											type="button"
											onclick={() => saveConcept(concept.id)}
											disabled={savingConcept}
											class="px-4 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 rounded-xl transition cursor-pointer shadow-xs disabled:opacity-50"
										>
											{savingConcept ? 'Saving…' : 'Save changes'}
										</button>
									</div>
								</div>
							{:else}
								<!-- Regular Concept Card -->
								<div class="border border-slate-200/80 hover:border-slate-300 rounded-2xl bg-white p-4 transition shadow-2xs space-y-2.5 flex flex-col justify-between">
									<div class="space-y-2.5">
										<div class="flex items-start justify-between gap-2">
											<div class="flex flex-wrap items-center gap-1.5 min-w-0">
												<span class="px-2 py-0.5 rounded-md text-[10px] font-semibold border capitalize {typeColor(concept.type)}">{typeLabel(concept.type)}</span>
												<h3 class="text-sm font-semibold text-slate-900 leading-snug">{concept.title}</h3>
												{#if concept.source === 'owner_pasted'}
													<span class="text-[10px] text-slate-400 bg-slate-100 px-1.5 py-0.5 rounded">pasted</span>
												{/if}
											</div>

											<!-- Action Controls -->
											<div class="flex items-center gap-0.5 shrink-0">
												<button
													type="button"
													onclick={() => startEditingConcept(concept)}
													class="flex items-center gap-1 px-2 py-1 rounded-lg text-xs font-medium text-slate-500 hover:text-blue-600 hover:bg-blue-50/70 transition cursor-pointer"
													title="Edit concept"
												>
													<PencilSquareIcon class="w-3.5 h-3.5" />
													<span>Edit</span>
												</button>
												<button
													type="button"
													onclick={() => deleteConcept(concept.id)}
													class="flex items-center gap-1 px-1.5 py-1 rounded-lg text-xs font-medium text-slate-400 hover:text-rose-600 hover:bg-rose-50/70 transition cursor-pointer"
													title="Delete concept"
												>
													<TrashIcon class="w-3.5 h-3.5" />
													<span>Delete</span>
												</button>
											</div>
										</div>

										{#if concept.tags?.length}
											<div class="flex flex-wrap items-center gap-1">
												{#each concept.tags as tag}
													<span class="text-[10px] text-slate-500 bg-slate-100/80 px-2 py-0.5 rounded-md font-medium">{tag}</span>
												{/each}
											</div>
										{/if}

										<!-- Direct Readable Body Text -->
										<div class="text-xs text-slate-600 leading-relaxed whitespace-pre-wrap {expandedConcepts[concept.id] ? '' : 'line-clamp-4'}">
											{concept.body_text}
										</div>

										{#if (concept.body_text || '').length > 180}
											<button
												type="button"
												onclick={() => toggleConceptExpansion(concept.id)}
												class="text-[11px] font-medium text-blue-600 hover:text-blue-700 cursor-pointer pt-0.5 inline-block"
											>
												{expandedConcepts[concept.id] ? 'Show less' : 'Show full content'}
											</button>
										{/if}
									</div>

									<div class="pt-2 mt-1 text-[10px] text-slate-400 border-t border-slate-100/80">
										<span>Added {formatDate(concept.created_at)}</span>
									</div>
								</div>
							{/if}
						{/each}
					</div>
				{/if}
			</div>
		</div>
	{:else if activeTab === 'patterns'}
		<div class="flex-1 overflow-y-auto px-6 py-4">
			{#if filteredPatterns.length === 0}
				<div class="flex flex-col items-center justify-center py-16 text-center">
					<div class="w-10 h-10 rounded-2xl bg-blue-50 flex items-center justify-center text-blue-500 mb-3">
						<ChatBubbleLeftRightIcon class="w-5 h-5" />
					</div>
					<div class="text-sm font-medium text-slate-700">
						{searchQuery.trim() ? 'No matching answer patterns' : 'No deterministic answer patterns yet'}
					</div>
					<div class="text-xs text-slate-400 mt-1 max-w-sm">
						{searchQuery.trim() ? 'Try adjusting your search terms' : 'Organize business knowledge or run an AI audit to create common question patterns'}
					</div>
				</div>
			{:else}
				<div class="grid grid-cols-1 md:grid-cols-2 gap-3.5 items-start">
					{#each filteredPatterns as pattern (pattern.id)}
						{#if editingPatternId === pattern.id}
							<!-- Inline Pattern Editor -->
							<div class="col-span-1 md:col-span-2 border-2 border-blue-500/60 rounded-2xl p-4 bg-white shadow-sm space-y-3 transition">
								<div class="flex items-center justify-between gap-2 border-b border-slate-100 pb-2.5">
									<span class="text-xs font-semibold text-slate-900 uppercase tracking-wider">Edit Answer Pattern</span>
									{#if savePatternError}
										<span class="text-xs text-rose-600 font-medium">{savePatternError}</span>
									{/if}
								</div>
								<div>
									<label for={`edit-pattern-question-${pattern.id}`} class="block text-[11px] font-medium text-slate-500 mb-1">Canonical Question</label>
									<input
										id={`edit-pattern-question-${pattern.id}`}
										bind:value={editPatternDraft.canonical_question}
										placeholder="Canonical question"
										class="w-full bg-slate-50/50 focus:bg-white border border-slate-200 rounded-xl px-3 py-1.5 text-xs font-semibold text-slate-900 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 transition"
									/>
								</div>
								<div>
									<label for={`edit-pattern-triggers-input-${pattern.id}`} class="block text-[11px] font-medium text-slate-500 mb-1">Trigger Phrases</label>
									<div class="flex flex-wrap items-center gap-1.5 mb-1.5">
										{#each editPatternDraft.trigger_phrases as phrase}
											<span class="inline-flex items-center gap-1 text-[11px] text-slate-700 bg-slate-100 border border-slate-200/80 px-2.5 py-0.5 rounded-lg">
												<span>{phrase}</span>
												<button type="button" onclick={() => removeTriggerFromPatternDraft(phrase)} class="text-slate-400 hover:text-rose-600 cursor-pointer">×</button>
											</span>
										{/each}
									</div>
									<div class="flex items-center gap-2">
										<input
											id={`edit-pattern-triggers-input-${pattern.id}`}
											bind:value={editPatternTriggerInput}
											onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), addTriggerToPatternDraft())}
											placeholder="Add trigger phrase and press Enter"
											class="flex-1 max-w-sm bg-slate-50/50 focus:bg-white border border-slate-200 rounded-xl px-2.5 py-1 text-xs text-slate-700 outline-none focus:border-blue-500 transition"
										/>
										<button type="button" onclick={addTriggerToPatternDraft} class="px-2.5 py-1 bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl text-xs font-medium cursor-pointer transition">Add</button>
									</div>
								</div>
								<div>
									<label for={`edit-pattern-answer-${pattern.id}`} class="block text-[11px] font-medium text-slate-500 mb-1">Deterministic Answer</label>
									<textarea
										id={`edit-pattern-answer-${pattern.id}`}
										bind:value={editPatternDraft.answer_text}
										rows="3"
										class="w-full bg-slate-50/50 focus:bg-white border border-slate-200 rounded-xl p-3 text-xs text-slate-700 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 leading-relaxed transition"
									></textarea>
								</div>
								<div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
									<button type="button" onclick={cancelEditingPattern} class="px-3 py-1.5 text-xs font-medium text-slate-600 hover:text-slate-900 rounded-xl hover:bg-slate-100 transition cursor-pointer">
										Cancel
									</button>
									<button
										type="button"
										onclick={() => savePattern(pattern.id)}
										disabled={savingPattern}
										class="px-4 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 rounded-xl transition cursor-pointer shadow-xs disabled:opacity-50"
									>
										{savingPattern ? 'Saving…' : 'Save changes'}
									</button>
								</div>
							</div>
						{:else}
							<!-- Regular Pattern Card (Conversational Q&A Flow) -->
							<div class="p-4 rounded-2xl border border-slate-200/80 hover:border-slate-300 bg-white space-y-2.5 transition shadow-2xs">
								<div class="flex items-start justify-between gap-2">
									<h3 class="text-sm font-semibold text-slate-900 leading-snug">{pattern.canonical_question}</h3>
									<div class="flex items-center gap-0.5 shrink-0">
										<button
											type="button"
											onclick={() => startEditingPattern(pattern)}
											class="flex items-center gap-1 px-2 py-1 rounded-lg text-xs font-medium text-slate-500 hover:text-blue-600 hover:bg-blue-50/70 transition cursor-pointer"
											title="Edit pattern"
										>
											<PencilSquareIcon class="w-3.5 h-3.5" />
											<span>Edit</span>
										</button>
										<button
											type="button"
											onclick={() => deletePattern(pattern.id)}
											class="flex items-center gap-1 px-1.5 py-1 rounded-lg text-xs font-medium text-slate-400 hover:text-rose-600 hover:bg-rose-50/70 transition cursor-pointer"
											title="Delete pattern"
										>
											<TrashIcon class="w-3.5 h-3.5" />
											<span>Delete</span>
										</button>
									</div>
								</div>

								{#if pattern.trigger_phrases?.length}
									<div class="flex flex-wrap items-center gap-1 pt-0.5">
										<span class="text-[10px] font-semibold uppercase tracking-wider text-slate-400 mr-0.5">Triggers:</span>
										{#each pattern.trigger_phrases as phrase}
											<span class="text-xs text-slate-700 bg-slate-100/90 border border-slate-200/50 px-2 py-0.5 rounded-lg">{phrase}</span>
										{/each}
									</div>
								{/if}

								<!-- Direct Answer Bubble -->
								<div class="text-xs text-slate-700 leading-relaxed whitespace-pre-wrap bg-slate-50/80 p-3 rounded-xl border border-slate-200/70">
									{pattern.answer_text}
								</div>
							</div>
						{/if}
					{/each}
				</div>
			{/if}
		</div>
	{:else}
		<!-- AI Suggestions Tab -->
		<div class="flex-1 overflow-y-auto px-6 py-4">
			{#if filteredSuggestions.length === 0}
				<div class="flex flex-col items-center justify-center py-16 text-center">
					<div class="w-10 h-10 rounded-2xl bg-amber-50 flex items-center justify-center text-amber-500 mb-3">
						<SparklesIcon class="w-5 h-5" />
					</div>
					<div class="text-sm font-medium text-slate-700">
						{searchQuery.trim() ? 'No matching suggestions' : 'No suggestions pending review'}
					</div>
					<div class="text-xs text-slate-400 mt-1 max-w-sm">
						{searchQuery.trim() ? 'Try adjusting your search terms' : 'When AI audits find knowledge gaps in conversations, recommendations will appear here'}
					</div>
				</div>
			{:else}
				<div class="grid grid-cols-1 md:grid-cols-2 gap-3.5 items-start">
					{#each filteredSuggestions as suggestion (suggestion.id)}
						<div class="p-4 rounded-2xl border border-slate-200/80 bg-white space-y-3 shadow-2xs">
							<div class="flex items-center justify-between gap-2">
								<div class="flex items-center gap-2 min-w-0">
									<span class="px-2 py-0.5 rounded text-[10px] font-semibold border capitalize {typeColor(suggestion._payload?.type ?? suggestion.type)}">
										{typeLabel(suggestion._payload?.type ?? suggestion.type)}
									</span>
									<h3 class="text-sm font-semibold text-slate-900 truncate">
										{suggestion._payload?.title ?? suggestion._payload?.canonical_question ?? 'Untitled suggestion'}
									</h3>
								</div>
								<span class="px-2 py-0.5 rounded-md text-[10px] font-semibold bg-blue-50 text-blue-700 border border-blue-100 shrink-0">
									{Math.round((suggestion.confidence ?? 0) * 100)}% match
								</span>
							</div>
							<div class="text-xs text-slate-700 bg-slate-50/80 p-3 rounded-xl leading-relaxed whitespace-pre-wrap border border-slate-200/70">
								{suggestion._payload?.body_text ?? suggestion._payload?.answer_text ?? ''}
							</div>
							<div class="flex items-center justify-end gap-2 pt-1 text-xs">
								<button onclick={() => reviewSuggestion(suggestion.id, 'reject')} class="px-3 py-1.5 text-slate-600 hover:text-slate-900 hover:bg-slate-100 rounded-xl transition cursor-pointer font-medium">
									Dismiss
								</button>
								<button onclick={() => reviewSuggestion(suggestion.id, 'approve')} class="px-3.5 py-1.5 bg-blue-600 hover:bg-blue-700 text-white font-medium rounded-xl transition cursor-pointer shadow-xs active:scale-[0.98]">
									Add to Knowledge Base
								</button>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>
