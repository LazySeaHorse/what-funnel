<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { isAbortError } from '$lib/api';
	import {
		AUDIT_LOG_PAGE_SIZE,
		auditActionLabel,
		auditActorLabel,
		auditMetadataSummary,
		fetchAuditLogs,
		type AuditLogEntry
	} from '$lib/audit-logs';
	import { userDisplayName } from '$lib/workspace.svelte';
	import { Button } from '$lib/components/ui';

	let { users = [], searchQuery = '' }: { users?: any[]; searchQuery?: string } = $props();

	let entries = $state<AuditLogEntry[]>([]);
	let nextCursor = $state<string | null>(null);
	let loading = $state(true);
	let loadingMore = $state(false);
	let error = $state('');
	let expanded = $state<Record<string, boolean>>({});

	let actorFilter = $state('');
	let actionFilter = $state('');
	let fromDate = $state('');
	let toDate = $state('');

	let controller: AbortController | null = null;
	let actionDebounce: ReturnType<typeof setTimeout> | undefined;

	const hasFilters = $derived(!!(actorFilter || actionFilter.trim() || fromDate || toDate));
	const knownActions = $derived([...new Set(entries.map((e) => e.action))].sort());
	const query = $derived(searchQuery.trim().toLowerCase());
	// The header search narrows the rows already loaded; filters query the server.
	const visible = $derived(
		!query
			? entries
			: entries.filter((e) =>
					[auditActionLabel(e.action), e.action, auditActorLabel(e), e.target_type, e.target_id ?? '', JSON.stringify(e.metadata ?? {})]
						.join(' ')
						.toLowerCase()
						.includes(query)
				)
	);

	function dayStart(value: string) {
		const d = new Date(`${value}T00:00:00`);
		return Number.isNaN(d.getTime()) ? undefined : d.toISOString();
	}
	/** Inclusive upper bound (the API's `to` is inclusive): the last millisecond of the chosen day. */
	function dayEnd(value: string) {
		const d = new Date(`${value}T00:00:00`);
		if (Number.isNaN(d.getTime())) return undefined;
		d.setDate(d.getDate() + 1);
		d.setTime(d.getTime() - 1);
		return d.toISOString();
	}

	async function load(more = false) {
		if (more && (!nextCursor || loadingMore)) return;
		controller?.abort();
		const current = new AbortController();
		controller = current;
		error = '';
		if (more) loadingMore = true;
		else loading = true;
		try {
			const page = await fetchAuditLogs(
				{
					limit: AUDIT_LOG_PAGE_SIZE,
					cursor: more ? nextCursor : null,
					actorUserID: actorFilter,
					action: actionFilter,
					from: fromDate ? dayStart(fromDate) : undefined,
					to: toDate ? dayEnd(toDate) : undefined
				},
				current.signal
			);
			if (current.signal.aborted) return;
			entries = more ? [...entries, ...page.items.filter((i) => !entries.some((e) => e.id === i.id))] : page.items;
			nextCursor = page.next_cursor;
		} catch (err) {
			if (isAbortError(err)) return;
			console.error('Failed to load audit logs', err);
			error = err instanceof Error && err.message ? err.message : 'Failed to load audit logs.';
		} finally {
			if (controller === current) {
				loading = false;
				loadingMore = false;
			}
		}
	}

	function applyFilters() {
		clearTimeout(actionDebounce);
		void load();
	}

	function onActionInput() {
		clearTimeout(actionDebounce);
		actionDebounce = setTimeout(() => void load(), 350);
	}

	function clearFilters() {
		actorFilter = actionFilter = fromDate = toDate = '';
		applyFilters();
	}

	function formatWhen(iso: string) {
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return iso;
		return d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
	}

	onMount(() => void load());
	onDestroy(() => {
		controller?.abort();
		clearTimeout(actionDebounce);
	});

	const fieldClass =
		'w-full h-8 px-2.5 bg-slate-50 border border-slate-200 rounded-lg text-xs text-slate-700 focus:outline-none focus:border-blue-400';
	const labelClass = 'block text-[11px] font-medium text-slate-400 uppercase tracking-wider mb-1.5';
</script>

<div class="flex-1 flex flex-col overflow-hidden bg-white" data-testid="logs-view">
	<header class="px-6 pt-5 pb-3 border-b border-slate-100 shrink-0 space-y-3.5 bg-white">
		<div>
			<h1 class="text-2xl font-medium text-slate-900 tracking-tight">Logs</h1>
		</div>
		<form
			class="grid grid-cols-2 lg:grid-cols-5 gap-3 items-end"
			onsubmit={(e) => {
				e.preventDefault();
				applyFilters();
			}}
		>
			{#if users.length > 0}
				<div>
					<label for="logs-actor-filter" class={labelClass}>Actor</label>
					<select id="logs-actor-filter" bind:value={actorFilter} onchange={applyFilters} class={fieldClass}>
						<option value="">Everyone</option>
						{#each users as user (user.id)}
							<option value={user.id}>{userDisplayName(user)}</option>
						{/each}
					</select>
				</div>
			{/if}
			<div>
				<label for="logs-action-filter" class={labelClass}>Action</label>
				<input
					id="logs-action-filter"
					type="text"
					list="logs-action-options"
					placeholder="e.g. lead.state_changed"
					bind:value={actionFilter}
					oninput={onActionInput}
					class={fieldClass}
				/>
				<datalist id="logs-action-options">
					{#each knownActions as action (action)}<option value={action}>{auditActionLabel(action)}</option>{/each}
				</datalist>
			</div>
			<div>
				<label for="logs-from-filter" class={labelClass}>From</label>
				<input id="logs-from-filter" type="date" bind:value={fromDate} max={toDate || undefined} onchange={applyFilters} class={fieldClass} />
			</div>
			<div>
				<label for="logs-to-filter" class={labelClass}>To</label>
				<input id="logs-to-filter" type="date" bind:value={toDate} min={fromDate || undefined} onchange={applyFilters} class={fieldClass} />
			</div>
			{#if hasFilters}
				<div>
					<Button variant="ghost" size="sm" onclick={clearFilters}>Clear filters</Button>
				</div>
			{/if}
		</form>
	</header>

	<div class="flex-1 min-h-0 overflow-y-auto px-6 py-4 pb-20 lg:pb-4">
		{#if error}
			<div role="alert" data-testid="logs-error" class="px-3.5 py-2.5 border rounded-xl text-xs flex items-center justify-between gap-2 bg-rose-50/90 border-rose-200 text-rose-900">
				<span>{error}</span>
				<Button variant="secondary" size="xs" onclick={() => void load()}>Retry</Button>
			</div>
		{/if}

		{#if loading}
			<div class="py-16 flex justify-center" role="status" aria-label="Loading logs">
				<span class="inline-block h-5 w-5 animate-spin rounded-full border-2 border-blue-600 border-t-transparent"></span>
			</div>
		{:else if visible.length === 0 && !error}
			<div data-testid="logs-empty" class="py-16 text-center text-sm text-slate-500">
				{hasFilters || query ? 'No log entries match your filters.' : 'No activity has been logged yet.'}
			</div>
		{:else if visible.length > 0}
			<ul class="divide-y divide-slate-100 border border-slate-200 rounded-xl overflow-hidden" data-testid="logs-list">
				{#each visible as entry (entry.id)}
					{@const summary = auditMetadataSummary(entry.metadata)}
					{@const hasMetadata = Object.keys(entry.metadata ?? {}).length > 0}
					<li class="px-4 py-3 text-xs bg-white" data-testid="log-entry">
						<div class="flex flex-col md:flex-row md:items-start gap-1.5 md:gap-4">
							<time class="md:w-40 shrink-0 text-slate-500" datetime={entry.created_at}>{formatWhen(entry.created_at)}</time>
							<div class="md:w-44 shrink-0 font-medium text-slate-800 truncate" title={entry.actor_email ?? undefined}>
								{auditActorLabel(entry)}
							</div>
							<div class="flex-1 min-w-0 space-y-1">
								<div class="flex flex-wrap items-center gap-2">
									<span class="font-medium text-slate-900">{auditActionLabel(entry.action)}</span>
									<span class="px-1.5 py-0.5 rounded bg-slate-100 text-slate-600 text-[10px] font-medium">{entry.target_type}</span>
									{#if entry.target_id}<span class="text-[11px] text-slate-400 font-mono truncate max-w-48" title={entry.target_id}>{entry.target_id}</span>{/if}
								</div>
								{#if summary}<p class="text-slate-500 break-words">{summary}</p>{/if}
								{#if hasMetadata}
									<button
										type="button"
										class="text-[11px] font-medium text-blue-600 hover:text-blue-700 cursor-pointer"
										aria-expanded={!!expanded[entry.id]}
										onclick={() => (expanded[entry.id] = !expanded[entry.id])}
									>
										{expanded[entry.id] ? 'Hide details' : 'Show details'}
									</button>
									{#if expanded[entry.id]}
										<pre class="mt-1 p-2.5 bg-slate-50 border border-slate-200 rounded-lg text-[11px] text-slate-700 overflow-x-auto whitespace-pre-wrap break-all">{JSON.stringify(entry.metadata, null, 2)}</pre>
									{/if}
								{/if}
							</div>
						</div>
					</li>
				{/each}
			</ul>
			{#if nextCursor}
				<div class="pt-4 flex justify-center">
					<Button variant="secondary" size="sm" busy={loadingMore} disabled={loadingMore} onclick={() => void load(true)}>
						Load more
					</Button>
				</div>
			{/if}
		{/if}
	</div>
</div>
