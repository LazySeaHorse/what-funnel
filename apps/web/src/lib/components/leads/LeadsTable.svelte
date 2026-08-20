<script lang="ts">
	import ChannelBadge from '../ChannelBadge.svelte';
	import LeadStateBadge from '../LeadStateBadge.svelte';
	import UserAvatar from '../UserAvatar.svelte';

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
		totalLeadsCount: number;
		selectedLeadId: string | null;
		selectedRowIds: string[];
		onSelectLead: (lead: any) => void;
		onToggleCheckbox: (id: string, e: MouseEvent) => void;
		onToggleAllCheckboxes: (e: MouseEvent) => void;
	} = $props();

	const allChecked = $derived(
		leads.length > 0 && selectedRowIds.length === leads.length
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
		<div>Lead State</div>
		<div>Assigned to</div>
		<div>Last Message</div>
		<div class="flex items-center gap-1 cursor-pointer hover:text-slate-600 justify-start">
			<span>Updated</span>
			<svg class="w-3.5 h-3.5 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
				<path stroke-linecap="round" stroke-linejoin="round" d="M19 14l-7 7m0 0l-7-7m7 7V3" />
			</svg>
		</div>
	</div>

	<!-- Table Rows List -->
	<div class="flex-1 overflow-y-auto min-h-0 px-3 py-1 space-y-1">
		{#each leads as lead (lead.id)}
			{@const isSelected = selectedLeadId === lead.id}
			{@const isChecked = selectedRowIds.includes(lead.id)}
			<div
				role="button"
				tabindex="0"
				onclick={() => onSelectLead(lead)}
				onkeydown={(e) => { if (e.key === 'Enter') onSelectLead(lead); }}
				class="grid grid-cols-[40px_minmax(180px,2.4fr)_75px_125px_110px_minmax(160px,2fr)_85px] px-3 py-2.5 items-center text-xs transition cursor-pointer rounded-xl {isSelected ? 'bg-[#F4F8FE] border border-blue-200/90' : 'bg-white hover:bg-slate-50/70 border border-transparent'}"
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
						<div class="font-semibold text-slate-900 truncate leading-tight">{lead.name}</div>
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
					<svg class="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
					</svg>
				</div>
				<div class="text-sm font-semibold text-slate-800">No leads in this view</div>
				<div class="text-xs text-slate-400 mt-1 max-w-sm">Leads are automatically created as incoming inquiries arrive across your connected channels.</div>
			</div>
		{/if}
	</div>

	<!-- Table Footer Pagination -->
	<div class="px-6 py-3 bg-white border-t border-slate-100 flex items-center justify-between text-xs text-slate-500 shrink-0 select-none">
		<div class="tabular-nums text-slate-500">
			Showing {leads.length > 0 ? 1 : 0} to {leads.length} of {totalLeadsCount} leads
		</div>

		<div class="flex items-center gap-3">
			<div class="flex items-center gap-1">
				<button class="w-7 h-7 rounded-lg border border-slate-200 flex items-center justify-center text-slate-400 hover:text-slate-700 hover:bg-slate-50 cursor-pointer" aria-label="Previous page">
					<svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M15 19l-7-7 7-7" />
					</svg>
				</button>
				<button class="w-7 h-7 rounded-lg bg-blue-50 border border-blue-200 text-blue-600 font-semibold flex items-center justify-center cursor-pointer">1</button>
				<button class="w-7 h-7 rounded-lg hover:bg-slate-50 text-slate-600 flex items-center justify-center cursor-pointer">2</button>
				<button class="w-7 h-7 rounded-lg hover:bg-slate-50 text-slate-600 flex items-center justify-center cursor-pointer">3</button>
				<span class="px-1 text-slate-400 text-xs">...</span>
				<button class="w-7 h-7 rounded-lg hover:bg-slate-50 text-slate-600 flex items-center justify-center cursor-pointer">13</button>
				<button class="w-7 h-7 rounded-lg border border-slate-200 flex items-center justify-center text-slate-400 hover:text-slate-700 hover:bg-slate-50 cursor-pointer" aria-label="Next page">
					<svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
					</svg>
				</button>
			</div>

			<div class="flex items-center gap-1.5 pl-2">
				<button class="flex items-center gap-1 px-2.5 py-1 rounded-lg border border-slate-200 text-xs text-slate-600 hover:bg-slate-50 cursor-pointer">
					<span>10 / page</span>
					<svg class="w-3 h-3 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
					</svg>
				</button>
			</div>
		</div>
	</div>
</div>
