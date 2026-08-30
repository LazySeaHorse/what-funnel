<script lang="ts">
	import ChannelBadge from '../ChannelBadge.svelte';
	import LeadStateBadge from '../LeadStateBadge.svelte';
	import UserAvatar from '../UserAvatar.svelte';
	import {
		ArrowDownIcon,
		UsersIcon,
		ChevronLeftIcon,
		ChevronRightIcon
	} from '@fvilers/heroicons-svelte/24/outline';

	let {
		leads = [],
		totalLeadsCount = 0,
		selectedLeadId = null,
		selectedRowIds = [],
		onSelectLead = () => {},
		onToggleCheckbox = () => {},
		onToggleAllCheckboxes = () => {}
	}: {
		leads: any[];
		totalLeadsCount?: number;
		selectedLeadId: string | null;
		selectedRowIds: string[];
		onSelectLead: (lead: any) => void;
		onToggleCheckbox: (id: string, e: MouseEvent) => void;
		onToggleAllCheckboxes: (e: MouseEvent) => void;
	} = $props();

	let pageSize = $state(10);
	let currentPage = $state(1);

	const totalPages = $derived(Math.max(1, Math.ceil(leads.length / pageSize)));
	const startIndex = $derived(leads.length === 0 ? 0 : (currentPage - 1) * pageSize);
	const endIndex = $derived(Math.min(leads.length, startIndex + pageSize));
	const paginatedLeads = $derived(leads.slice(startIndex, endIndex));
	const showingStart = $derived(leads.length === 0 ? 0 : startIndex + 1);
	const showingEnd = $derived(endIndex);

	$effect(() => {
		if (currentPage > totalPages) {
			currentPage = Math.max(1, totalPages);
		}
	});

	function getPageNumbers(current: number, total: number): (number | '...')[] {
		if (total <= 1) return [1];
		if (total <= 5) {
			return Array.from({ length: total }, (_, i) => i + 1);
		}
		if (current <= 3) {
			return [1, 2, 3, 4, '...', total];
		}
		if (current >= total - 2) {
			return [1, '...', total - 3, total - 2, total - 1, total];
		}
		return [1, '...', current - 1, current, current + 1, '...', total];
	}

	const pageNumbers = $derived(getPageNumbers(currentPage, totalPages));

	const allChecked = $derived(
		paginatedLeads.length > 0 && paginatedLeads.every((l) => selectedRowIds.includes(l.id))
	);
</script>

<div class="flex-1 bg-white flex flex-col min-h-0 h-full overflow-hidden">
	<!-- Table Header Row -->
	<div class="grid grid-cols-[40px_minmax(180px,2.4fr)_75px_125px_110px_minmax(160px,2fr)_85px] px-6 py-3 border-b border-slate-100/90 text-xs font-medium text-slate-400 shrink-0 select-none items-center bg-white">
		<div class="flex items-center justify-center">
			<input
				type="checkbox"
				checked={allChecked}
				onclick={onToggleAllCheckboxes}
				class="w-4 h-4 rounded border-slate-300 text-blue-600 focus:ring-0 cursor-pointer accent-blue-600"
			/>
		</div>
		<div>Lead</div>
		<div>Channel</div>
		<div>Lead stage</div>
		<div>Assignee</div>
		<div>Last message</div>
		<div class="flex items-center gap-1 cursor-pointer hover:text-slate-600 justify-start">
			<span>Updated</span>
			<ArrowDownIcon class="w-3.5 h-3.5 text-slate-400" />
		</div>
	</div>

	<!-- Table Rows List -->
	<div class="flex-1 overflow-y-auto min-h-0 px-3 py-1 space-y-1">
		{#each paginatedLeads as lead (lead.id)}
			{@const isSelected = selectedLeadId === lead.id}
			{@const isChecked = selectedRowIds.includes(lead.id)}
			<div
				role="button"
				tabindex="0"
				onclick={() => onSelectLead(lead)}
				onkeydown={(e) => { if (e.key === 'Enter') onSelectLead(lead); }}
				class="grid grid-cols-[40px_minmax(180px,2.4fr)_75px_125px_110px_minmax(160px,2fr)_85px] px-3 py-2.5 items-center text-xs transition cursor-pointer rounded-xl {isSelected ? 'bg-blue-50 border border-blue-200/90' : 'bg-white hover:bg-slate-50/70 border border-transparent'}"
			>
				<!-- Checkbox -->
				<div class="flex items-center justify-center" onclick={(e) => e.stopPropagation()} role="presentation">
					<input
						type="checkbox"
						checked={isChecked}
						onclick={(e) => onToggleCheckbox(lead.id, e)}
						class="w-4 h-4 rounded border-slate-300 text-blue-600 focus:ring-0 cursor-pointer accent-blue-600"
					/>
				</div>

				<!-- Lead: Avatar + Name + Subtitle -->
				<div class="flex items-center gap-3 pr-2 min-w-0">
					<UserAvatar name={lead.name} avatar={lead.avatar} size="lg" />
					<div class="min-w-0 flex-1">
						<div class="font-medium text-slate-900 truncate leading-tight">{lead.name}</div>
						<div class="text-[11px] text-slate-400 truncate leading-tight mt-0.5">{lead.lastMessage}</div>
					</div>
				</div>

				<!-- Channel Icon -->
				<div class="flex items-center">
					<ChannelBadge channel={lead.channel} size="sm" />
				</div>

				<!-- Lead State Pill -->
				<div class="flex items-center">
					<LeadStateBadge stateKey={lead.stateKey} label={lead.stateLabel} size="sm" />
				</div>

				<!-- Assigned to -->
				<div class="flex items-center -space-x-1.5">
					{#if lead.assignees && lead.assignees.length > 0}
						{#each lead.assignees.slice(0, 2) as usr}
							<UserAvatar name={usr.name} avatar={usr.avatar} size="sm" class="ring-2 ring-white" />
						{/each}
						{#if lead.assignees.length > 2 || lead.assigneesExtra > 0}
							<div class="w-6 h-6 rounded-full ring-2 ring-white bg-slate-100 text-slate-600 flex items-center justify-center text-[10px] font-medium shrink-0">
								+{lead.assigneesExtra || (lead.assignees.length - 2)}
							</div>
						{/if}
					{:else}
						<span class="text-slate-300 text-xs italic">Unassigned</span>
					{/if}
				</div>

				<!-- Last Message -->
				<div class="text-slate-600 truncate pr-3 text-xs">
					{lead.lastMessage}
				</div>

				<!-- Updated -->
				<div class="text-xs text-slate-400 whitespace-nowrap tabular-nums">
					{lead.updatedAt}
				</div>
			</div>
		{/each}

		{#if leads.length === 0}
			<div class="py-24 flex flex-col items-center justify-center text-center">
				<div class="w-12 h-12 rounded-2xl bg-slate-100 text-slate-400 flex items-center justify-center mb-3">
					<UsersIcon class="w-6 h-6" />
				</div>
				<div class="text-sm font-medium text-slate-800">No leads found</div>
				<div class="text-xs text-slate-400 mt-1 max-w-sm">The system creates leads automatically when incoming messages arrive on connected channels.</div>
			</div>
		{/if}
	</div>

	<!-- Table Footer Pagination -->
	<div class="px-6 py-3 bg-white border-t border-slate-100 flex items-center justify-between text-xs text-slate-500 shrink-0 select-none">
		<div class="tabular-nums text-slate-500">
			Showing {showingStart} to {showingEnd} of {leads.length} leads
		</div>

		<div class="flex items-center gap-3">
			<div class="flex items-center gap-1">
				<button
					class="w-7 h-7 rounded-lg border border-slate-200 flex items-center justify-center text-slate-400 hover:text-slate-700 hover:bg-slate-50 disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-slate-400 disabled:cursor-not-allowed cursor-pointer"
					aria-label="Previous page"
					disabled={currentPage <= 1}
					onclick={() => { if (currentPage > 1) currentPage--; }}
				>
					<ChevronLeftIcon class="w-3.5 h-3.5" />
				</button>

				{#each pageNumbers as p}
					{#if p === '...'}
						<span class="px-1 text-slate-400 text-xs">...</span>
					{:else}
						<button
							class="w-7 h-7 rounded-lg font-medium flex items-center justify-center transition-colors cursor-pointer {p === currentPage ? 'bg-blue-50 border border-blue-200 text-blue-600' : 'hover:bg-slate-50 text-slate-600'}"
							onclick={() => { currentPage = p; }}
						>
							{p}
						</button>
					{/if}
				{/each}

				<button
					class="w-7 h-7 rounded-lg border border-slate-200 flex items-center justify-center text-slate-400 hover:text-slate-700 hover:bg-slate-50 disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-slate-400 disabled:cursor-not-allowed cursor-pointer"
					aria-label="Next page"
					disabled={currentPage >= totalPages}
					onclick={() => { if (currentPage < totalPages) currentPage++; }}
				>
					<ChevronRightIcon class="w-3.5 h-3.5" />
				</button>
			</div>

			<div class="flex items-center gap-1.5 pl-2">
				<select
					bind:value={pageSize}
					onchange={() => { currentPage = 1; }}
					class="px-2 py-1 rounded-lg border border-slate-200 text-xs text-slate-600 bg-white hover:bg-slate-50 cursor-pointer focus:outline-none focus:ring-1 focus:ring-blue-500"
				>
					<option value={10}>10 / page</option>
					<option value={25}>25 / page</option>
					<option value={50}>50 / page</option>
					<option value={100}>100 / page</option>
				</select>
			</div>
		</div>
	</div>
</div>
