<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { apiRequest } from '$lib/api';
	import {
		SparklesIcon,
		TrashIcon,
		CheckIcon,
		XMarkIcon,
		BookOpenIcon,
		ChatBubbleLeftRightIcon,
		PlusIcon
	} from '@fvilers/heroicons-svelte/24/outline';
	import IngestionReview from '$lib/components/knowledge/IngestionReview.svelte';
	import KnowledgeComposer from '$lib/components/knowledge/KnowledgeComposer.svelte';
	import ConceptCard from '$lib/components/knowledge/ConceptCard.svelte';
	import ConceptEditModal from '$lib/components/knowledge/ConceptEditModal.svelte';
	import PatternCard from '$lib/components/knowledge/PatternCard.svelte';
	import PatternEditModal from '$lib/components/knowledge/PatternEditModal.svelte';
	import SuggestionCard from '$lib/components/knowledge/SuggestionCard.svelte';
	import { Button } from '$lib/components/ui';
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
	let faqLimit = $state<{ max_active: number; active: number; excluded: number } | null>(null);
	let suggestions = $state<any[]>([]);
	let lastRun = $state<any>(null);
	let loading = $state(true);
	let activeTab = $state<'concepts' | 'patterns' | 'suggestions'>('concepts');
	let pasteText = $state('');
	let pasteResult = $state<{ added?: number; patternsAdded?: number; error?: string } | null>(null);
	const ingestion = new KnowledgeIngestionController();
	let showAddKnowledgeModal = $state(false);
	let expandedConcepts = $state<Record<string, boolean>>({});
	let mining = $state(false);
	let miningResult = $state<{ messages_scanned?: number; clusters_found?: number; suggestions_created?: number } | null>(null);
	let purging = $state(false);
	let purgeResult = $state<{ concepts: number; patterns: number } | null>(null);
	let purgeError = $state('');
	let actionError = $state('');
	let loadError = $state('');

	// Active editing targets
	let editingConceptId = $state<string | null>(null);
	let editingPatternId = $state<string | null>(null);

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
		loadError = '';
		try {
			const [conceptsRes, patternsRes, suggestionsRes, miningRes] = await Promise.allSettled([
				apiRequest('/api/kb/concepts'),
				apiRequest('/api/kb/patterns'),
				apiRequest('/api/kb/suggestions?status_filter=pending'),
				apiRequest('/api/kb/mining-runs/latest')
			]);
			if (conceptsRes.status === 'fulfilled') concepts = conceptsRes.value?.concepts ?? [];
			if (patternsRes.status === 'fulfilled') {
				patterns = patternsRes.value?.patterns ?? [];
				faqLimit = patternsRes.value?.limit ?? null;
			}
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
			const failed = [
				[conceptsRes, 'concepts'],
				[patternsRes, 'patterns'],
				[suggestionsRes, 'AI suggestions'],
				[miningRes, 'the latest audit']
			].filter(([res]) => (res as PromiseSettledResult<unknown>).status === 'rejected');
			for (const [res, name] of failed) {
				console.error(`Failed to load ${name}`, (res as PromiseRejectedResult).reason);
			}
			if (failed.length) loadError = `Failed to load ${failed.map(([, name]) => name).join(', ')}. Showing what could be loaded.`;
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		void load();
		void resumeLatestIngestion();
	});
	onDestroy(() => ingestion.dispose());

	function errorMessage(err: unknown, fallback: string) {
		return err instanceof Error && err.message ? err.message : fallback;
	}

	async function deleteConcept(id: string) {
		actionError = '';
		if (!confirm('Delete this knowledge concept?')) return;
		try {
			await apiRequest(`/api/kb/concepts/${id}`, { method: 'DELETE' });
			concepts = concepts.filter((concept) => concept.id !== id);
			if (editingConceptId === id) editingConceptId = null;
		} catch (err) {
			console.error('Failed to delete concept', err);
			actionError = errorMessage(err, 'Failed to delete the concept.');
		}
	}

	async function deletePattern(id: string) {
		actionError = '';
		if (!confirm('Delete this pattern?')) return;
		try {
			await apiRequest(`/api/kb/patterns/${id}`, { method: 'DELETE' });
			patterns = patterns.filter((pattern) => pattern.id !== id);
			if (editingPatternId === id) editingPatternId = null;
			// Deleting an active FAQ lets an excluded one become active: refresh the statuses.
			await load(true);
		} catch (err) {
			console.error('Failed to delete pattern', err);
			actionError = errorMessage(err, 'Failed to delete the pattern.');
		}
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
		showAddKnowledgeModal = false;
	}

	async function handleIngestionResult(result: Awaited<ReturnType<KnowledgeIngestionController['start']>>) {
		if (result.status !== 'complete') return;
		pasteResult = { added: result.conceptsAdded, patternsAdded: result.patternsAdded };
		pasteText = '';
		showAddKnowledgeModal = false;
		await load(true);
	}

	async function resumeLatestIngestion() {
		try {
			const result = await ingestion.resumeLatest();
			if (result) {
				await handleIngestionResult(result);
				if (result.status === 'review_required' || ingestion.phase === 'review') {
					showAddKnowledgeModal = true;
				}
			}
		} catch (error: any) {
			pasteResult = { error: error.message || 'Failed to resume knowledge ingestion' };
		}
	}

	async function compilePaste() {
		if (!pasteText.trim()) return;
		pasteResult = null;
		try {
			const res = await ingestion.start(pasteText);
			await handleIngestionResult(res);
		} catch (error: any) {
			pasteResult = { error: error.message || 'Failed to compile' };
		}
	}

	async function publishIngestion() {
		pasteResult = null;
		try {
			const res = await ingestion.publish();
			await handleIngestionResult(res);
		} catch (error: any) {
			pasteResult = { error: error.message || 'Failed to publish knowledge' };
		}
	}

	async function reviewSuggestion(id: string, action: 'approve' | 'reject') {
		actionError = '';
		try {
			await apiRequest(`/api/kb/suggestions/${id}/${action}`, { method: 'POST', body: { reviewed_by: reviewerID } });
			suggestions = suggestions.filter((suggestion) => suggestion.id !== id);
			if (action === 'approve') await load(true);
		} catch (err) {
			console.error('Failed to review suggestion', err);
			actionError = errorMessage(err, `Failed to ${action} the suggestion.`);
		}
	}

	async function triggerMining() {
		mining = true;
		miningResult = null;
		actionError = '';
		try {
			miningResult = await apiRequest('/api/kb/mine/trigger', { method: 'POST' });
			await load(true);
		} catch (err) {
			console.error('Failed to trigger mining', err);
			actionError = errorMessage(err, 'Failed to run the chat audit.');
		} finally {
			mining = false;
		}
	}

	let reviewReady = $derived(ingestion.phase === 'review' && !showAddKnowledgeModal);

	function formatDate(iso?: string | null) {
		if (!iso) return 'Never';
		return new Date(iso).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
	}

	function toggleConceptExpansion(id: string) {
		expandedConcepts[id] = !expandedConcepts[id];
	}
</script>

<div class="flex-1 flex flex-col overflow-hidden bg-white">
	<!-- Top Level Header -->
	<header class="px-6 pt-5 pb-3 border-b border-slate-100 shrink-0 space-y-3.5 bg-white">
		<!-- Row 1: Title, Subtitle & Primary Actions -->
		<div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
			<div>
				<h1 class="text-2xl font-medium text-slate-900 tracking-tight">Knowledge base</h1>
				<p class="text-xs text-slate-500 mt-0.5">
					Manage business facts, policies, and deterministic answers for your AI assistant.
				</p>
			</div>

			<div class="flex flex-wrap items-center gap-2">
				<!-- Add Knowledge Action -->
				<Button
					variant="primary"
					size="sm"
					onclick={() => (showAddKnowledgeModal = true)}
					class="shadow-2xs"
					title={ingestion.busy && !showAddKnowledgeModal ? 'Adding knowledge in background… (click to view)' : reviewReady ? 'Your knowledge is ready to review' : 'Add knowledge'}
				>
					{#if reviewReady}
						<CheckIcon class="w-3.5 h-3.5" />
						<span data-testid="review-ready">Review ready</span>
					{:else if ingestion.busy && !showAddKnowledgeModal}
						<span data-testid="add-knowledge-spinner" class="inline-block h-3.5 w-3.5 animate-spin rounded-full border-2 border-current border-t-transparent" aria-label="Ingesting knowledge" role="status"></span>
						<span>Adding knowledge…</span>
					{:else}
						<PlusIcon class="w-3.5 h-3.5" />
						<span>Add knowledge</span>
					{/if}
				</Button>

				<!-- Purge Action -->
				<Button
					variant="danger"
					size="sm"
					onclick={purgeKnowledgeBase}
					disabled={purging || ingestion.phase !== 'idle'}
					busy={purging}
					class="shadow-2xs"
					title={ingestion.phase !== 'idle'
						? 'Finish or discard the knowledge currently being added before purging'
						: 'Permanently remove all concepts and deterministic patterns'}
				>
					<TrashIcon class="w-3.5 h-3.5" />
					<span>Purge knowledge base</span>
				</Button>
			</div>
		</div>

		<!-- Row 2: Sub-navigation & Workspace Governance Controls -->
		<div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 pt-0.5">
			<!-- Segmented Sub-Tabs -->
			<nav class="inline-flex p-1 bg-slate-100/90 rounded-xl border border-slate-200/60 self-start shadow-2xs" aria-label="Knowledge sections">
				{#each [{ key: 'concepts', label: 'KB Concepts', count: filteredConcepts.length, hint: 'Business facts & policies' }, { key: 'patterns', label: 'Patterns', count: filteredPatterns.length, hint: 'Exact Q&A triggers' }, { key: 'suggestions', label: 'AI Suggestions', count: filteredSuggestions.length, hint: 'Mined from chats' }] as tab}
					<button
						onclick={() => (activeTab = tab.key as typeof activeTab)}
						title={tab.hint}
						class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-all cursor-pointer {activeTab === tab.key ? 'bg-white text-slate-900 shadow-2xs font-medium' : 'text-slate-500 hover:text-slate-800'}"
					>
						<span>{tab.label}</span>
						<span class="px-1.5 py-0.5 rounded-md text-[10px] font-medium {activeTab === tab.key ? 'bg-slate-100 text-slate-800' : 'bg-slate-200/60 text-slate-500'}">
							{tab.count}
						</span>
						{#if tab.key === 'suggestions' && filteredSuggestions.length > 0}
							<span class="w-1.5 h-1.5 rounded-full bg-orange-500 ml-0.5"></span>
						{/if}
					</button>
				{/each}
			</nav>

			<!-- Right: Global Auto-reply Switch -->
			<div class="flex items-center gap-2.5 self-start sm:self-auto">
				<!-- Toggle Switch Component -->
				<button
					type="button"
					role="switch"
					aria-label="Global AI auto-reply default"
					aria-checked={autoReplyEnabled && providerConfigured}
					onclick={onToggleAI}
					disabled={!canManageAI || togglingAI || (!providerConfigured && !autoReplyEnabled)}
					class="h-8 flex items-center gap-2 px-2.5 rounded-xl border border-slate-200 bg-white hover:bg-slate-50 text-xs font-medium text-slate-700 transition shadow-2xs cursor-pointer disabled:cursor-default disabled:opacity-60 active:scale-[0.98]"
					title={!providerConfigured ? 'Configure an AI provider in Settings before enabling automatic replies' : 'New customer chats inherit this setting unless overridden in chat'}
				>
					<span
						class="relative inline-flex h-4 w-7 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out {autoReplyEnabled && providerConfigured ? 'bg-emerald-500' : 'bg-slate-300'}"
					>
						<span
							class="inline-block h-3 w-3 transform rounded-full bg-white shadow transition duration-200 ease-in-out {autoReplyEnabled && providerConfigured ? 'translate-x-3' : 'translate-x-0'}"
						></span>
					</span>
					<span>Global AI auto-reply</span>
				</button>
			</div>
		</div>

		<!-- Feedback Notices -->
		{#if miningResult}
			<div class="px-3.5 py-2.5 bg-blue-50/90 border border-blue-200 rounded-xl text-xs text-blue-900 flex items-center justify-between gap-2 shadow-2xs">
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
			<div class="px-3.5 py-2.5 bg-emerald-50/90 border border-emerald-200 rounded-xl text-xs text-emerald-900 flex items-center justify-between gap-2 shadow-2xs">
				<div class="flex items-center gap-2">
					<CheckIcon class="w-4 h-4 text-emerald-600 shrink-0" />
					<span>Knowledge base purged — {purgeResult.concepts} concept{purgeResult.concepts !== 1 ? 's' : ''} and {purgeResult.patterns} pattern{purgeResult.patterns !== 1 ? 's' : ''} removed.</span>
				</div>
				<button type="button" onclick={() => (purgeResult = null)} class="text-emerald-500 hover:text-emerald-700 p-0.5 rounded cursor-pointer" aria-label="Dismiss banner">
					<XMarkIcon class="w-4 h-4" />
				</button>
			</div>
		{:else if purgeError}
			<div class="px-3.5 py-2.5 bg-rose-50/90 border border-rose-200 rounded-xl text-xs text-rose-900 flex items-center justify-between gap-2 shadow-2xs">
				<div class="flex items-center gap-2">
					<XMarkIcon class="w-4 h-4 text-rose-600 shrink-0" />
					<span>{purgeError}</span>
				</div>
				<button type="button" onclick={() => (purgeError = '')} class="text-rose-500 hover:text-rose-700 p-0.5 rounded cursor-pointer" aria-label="Dismiss banner">
					<XMarkIcon class="w-4 h-4" />
				</button>
			</div>
		{/if}
		{#if loadError || actionError}
			<div class="px-3.5 py-2.5 bg-rose-50/90 border border-rose-200 rounded-xl text-xs text-rose-900 flex items-center justify-between gap-2 shadow-2xs" role="alert" data-testid="knowledge-error">
				<div class="flex items-center gap-2">
					<XMarkIcon class="w-4 h-4 text-rose-600 shrink-0" />
					<span>{actionError || loadError}</span>
				</div>
				<button type="button" onclick={() => { if (actionError) actionError = ''; else loadError = ''; }} class="text-rose-500 hover:text-rose-700 p-0.5 rounded cursor-pointer" aria-label="Dismiss banner">
					<XMarkIcon class="w-4 h-4" />
				</button>
			</div>
		{/if}
		{#if pasteResult?.added !== undefined}
			<div class="px-3.5 py-2.5 bg-emerald-50/90 border border-emerald-200 rounded-xl text-xs text-emerald-900 flex items-center justify-between gap-2 shadow-2xs">
				<div class="flex items-center gap-2">
					<CheckIcon class="w-4 h-4 text-emerald-600 shrink-0" />
					<span>{pasteResult.added} concept{pasteResult.added !== 1 ? 's' : ''} and {pasteResult.patternsAdded ?? 0} pattern{pasteResult.patternsAdded !== 1 ? 's' : ''} added</span>
				</div>
				<button type="button" onclick={() => (pasteResult = null)} class="text-emerald-500 hover:text-emerald-700 p-0.5 rounded cursor-pointer" aria-label="Dismiss banner">
					<XMarkIcon class="w-4 h-4" />
				</button>
			</div>
		{:else if pasteResult?.error && !showAddKnowledgeModal}
			<div class="px-3.5 py-2.5 bg-rose-50/90 border border-rose-200 rounded-xl text-xs text-rose-900 flex items-center justify-between gap-2 shadow-2xs">
				<div class="flex items-center gap-2">
					<XMarkIcon class="w-4 h-4 text-rose-600 shrink-0" />
					<span>{pasteResult.error}</span>
				</div>
				<button type="button" onclick={() => (pasteResult = null)} class="text-rose-500 hover:text-rose-700 p-0.5 rounded cursor-pointer" aria-label="Dismiss banner">
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
			<!-- Concepts List -->
			<div class="flex-1 overflow-y-auto px-6 py-4">
				{#if filteredConcepts.length === 0}
					<div class="flex flex-col items-center justify-center py-16 text-center max-w-md mx-auto">
						<div class="w-12 h-12 rounded-2xl bg-slate-100 border border-slate-200/60 flex items-center justify-center text-slate-400 mb-3 shadow-2xs">
							<BookOpenIcon class="w-6 h-6" />
						</div>
						<div class="text-sm font-medium text-slate-800">
							{loadError ? 'Knowledge concepts could not be loaded' : searchQuery.trim() ? 'No matching knowledge concepts' : 'No knowledge concepts found'}
						</div>
						<div class="text-xs text-slate-500 mt-1">
							{searchQuery.trim() ? 'Try adjusting your search terms' : 'Click "Add knowledge" above to extract concepts with AI'}
						</div>
					</div>
				{:else}
					<div class="grid grid-cols-1 md:grid-cols-2 gap-4 items-start">
						{#each filteredConcepts as concept (concept.id)}
							{#if editingConceptId === concept.id}
								<ConceptEditModal
									concept={concept}
									onSave={(updated) => {
										concepts = concepts.map((c) => (c.id === concept.id ? { ...c, ...updated } : c));
										editingConceptId = null;
									}}
									onCancel={() => (editingConceptId = null)}
								/>
							{:else}
								<ConceptCard
									concept={concept}
									expanded={expandedConcepts[concept.id]}
									onToggleExpand={() => toggleConceptExpansion(concept.id)}
									onEdit={() => (editingConceptId = concept.id)}
									onDelete={() => deleteConcept(concept.id)}
								/>
							{/if}
						{/each}
					</div>
				{/if}
			</div>
		</div>
	{:else if activeTab === 'patterns'}
		<div class="flex-1 overflow-y-auto px-6 py-4">
			{#if faqLimit && faqLimit.excluded > 0}
				<div role="status" class="mb-4 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-xs text-amber-900">
					The AI uses at most {faqLimit.max_active} approved FAQs. {faqLimit.excluded} newer FAQ{faqLimit.excluded !== 1 ? 's are' : ' is'}
					not being used. Delete or merge FAQs to make room; the ones marked "Not used" below are the ones left out.
				</div>
			{/if}
			{#if filteredPatterns.length === 0}
				<div class="flex flex-col items-center justify-center py-16 text-center max-w-md mx-auto">
					<div class="w-12 h-12 rounded-2xl bg-blue-50 border border-blue-100 flex items-center justify-center text-blue-500 mb-3 shadow-2xs">
						<ChatBubbleLeftRightIcon class="w-6 h-6" />
					</div>
					<div class="text-sm font-medium text-slate-800">
						{searchQuery.trim() ? 'No matching answer patterns' : 'No deterministic answer patterns yet'}
					</div>
					<div class="text-xs text-slate-500 mt-1">
						{searchQuery.trim() ? 'Try adjusting your search terms' : 'Organize business knowledge or run an AI audit to create common question patterns'}
					</div>
					{#if !searchQuery.trim()}
						<div class="mt-4">
							<Button variant="secondary" size="xs" onclick={() => (activeTab = 'concepts')} class="shadow-2xs">
								<span>Add knowledge in KB Concepts</span>
							</Button>
						</div>
					{/if}
				</div>
			{:else}
				<div class="grid grid-cols-1 md:grid-cols-2 gap-4 items-start">
					{#each filteredPatterns as pattern (pattern.id)}
						{#if editingPatternId === pattern.id}
							<PatternEditModal
								pattern={pattern}
								onSave={(updated) => {
									patterns = patterns.map((p) => (p.id === pattern.id ? { ...p, ...updated } : p));
									editingPatternId = null;
								}}
								onCancel={() => (editingPatternId = null)}
							/>
						{:else}
							<PatternCard
								pattern={pattern}
								onEdit={() => (editingPatternId = pattern.id)}
								onDelete={() => deletePattern(pattern.id)}
							/>
						{/if}
					{/each}
				</div>
			{/if}
		</div>
	{:else}
		<!-- AI Suggestions Tab -->
		<div class="flex-1 overflow-y-auto px-6 py-4">
			{#if filteredSuggestions.length === 0}
				<div class="flex flex-col items-center justify-center py-16 text-center max-w-md mx-auto">
					<div class="w-12 h-12 rounded-2xl bg-orange-50 border border-orange-100 flex items-center justify-center text-orange-500 mb-3 shadow-2xs">
						<SparklesIcon class="w-6 h-6" />
					</div>
					<div class="text-sm font-medium text-slate-800">
						{searchQuery.trim() ? 'No matching suggestions' : 'No suggestions pending review'}
					</div>
					<div class="text-xs text-slate-500 mt-1">
						{searchQuery.trim() ? 'Try adjusting your search terms' : 'When AI audits find knowledge gaps in conversations, recommendations will appear here'}
					</div>
					{#if !searchQuery.trim()}
						<div class="mt-4">
							<Button variant="secondary" size="xs" onclick={triggerMining} disabled={mining} busy={mining} class="shadow-2xs">
								<SparklesIcon class="w-3.5 h-3.5 text-blue-600" />
								<span>Analyze recent chats</span>
							</Button>
						</div>
					{/if}
				</div>
			{:else}
				<div class="grid grid-cols-1 md:grid-cols-2 gap-4 items-start">
					{#each filteredSuggestions as suggestion (suggestion.id)}
						<SuggestionCard
							suggestion={suggestion}
							onApprove={() => reviewSuggestion(suggestion.id, 'approve')}
							onReject={() => reviewSuggestion(suggestion.id, 'reject')}
						/>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>

<svelte:window onkeydown={(e) => { if (showAddKnowledgeModal && e.key === 'Escape') showAddKnowledgeModal = false; }} />

{#if showAddKnowledgeModal}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 p-4"
		role="dialog"
		aria-modal="true"
		aria-label="Add knowledge"
		onclick={(e) => {
			if (e.target === e.currentTarget) {
				showAddKnowledgeModal = false;
			}
		}}
	>
		<div
			class="w-full max-w-2xl max-h-[90vh] overflow-y-auto bg-white rounded-2xl border border-slate-200 p-6 shadow-xl space-y-4"
		>
			<div class="flex items-center justify-between pb-2 border-b border-slate-100">
				<div>
					<h3 class="text-sm font-medium text-slate-900">
						{ingestion.phase === 'review' ? 'Review structured knowledge' : 'Add knowledge'}
					</h3>
					<p class="text-xs text-slate-500 mt-0.5">
						{ingestion.phase === 'review'
							? 'Review concepts and deterministic patterns before adding them to your knowledge base.'
							: 'Paste business facts, policies, FAQs, or pricing to automatically extract structured concepts.'}
					</p>
				</div>
				<button
					type="button"
					onclick={() => (showAddKnowledgeModal = false)}
					aria-label="Close dialog"
					class="w-7 h-7 rounded-lg hover:bg-slate-100 flex items-center justify-center text-slate-400 hover:text-slate-600 transition cursor-pointer shrink-0"
				>
					<XMarkIcon class="w-4 h-4" />
				</button>
			</div>

			{#if ingestion.phase === 'review'}
				<div class="space-y-3">
					{#if pasteResult?.error}
						<div class="flex items-center gap-1.5 text-xs text-rose-600 font-medium">
							<XMarkIcon class="w-4 h-4" />
							<span>{pasteResult.error}</span>
						</div>
					{/if}
					<div class="max-h-[60vh] overflow-y-auto pr-1">
						<IngestionReview bind:concepts={ingestion.concepts} bind:patterns={ingestion.patterns} />
					</div>
					<div class="flex items-center justify-end gap-2 pt-3 border-t border-slate-100">
						<Button variant="secondary" size="sm" onclick={discardIngestion} disabled={ingestion.busy}>
							Discard
						</Button>
						<Button variant="primary" size="sm" onclick={publishIngestion} disabled={ingestion.busy} busy={ingestion.busy}>
							Add selected to Knowledge Base
						</Button>
					</div>
				</div>
			{:else}
				<KnowledgeComposer
					bind:value={pasteText}
					busy={ingestion.busy}
					phase={ingestion.phase}
					result={pasteResult}
					onSubmit={compilePaste}
				/>
			{/if}
		</div>
	</div>
{/if}
