<script lang="ts">
	import LeadStageFilters from './LeadStageFilters.svelte';
	import LeadsTable from './LeadsTable.svelte';
	import LeadDetailDrawer from './LeadDetailDrawer.svelte';

	let {
		leads = [],
		counts = {
			all: 0,
			new: 0,
			contacted: 0,
			follow_up: 0,
			interested: 0,
			converted: 0
		},
		activeFilter = 'all',
		selectedLeadId = null,
		selectedRowIds = [],
		showDrawer = false,
		activeLead = null,
		pipelineStates = [],
		users = [],
		notes = [],
		assignedUserIds = [],
		canManageAssignments = false,
		onSelectFilter = () => {},
		onSelectLead = () => {},
		onToggleCheckbox = () => {},
		onToggleAllCheckboxes = () => {},
		onCloseDrawer = () => {},
		onOpenChat = () => {},
		onChangeState = () => {},
		onToggleAssignee = () => {},
		onAddTag = () => {},
		onRemoveTag = () => {},
		onSaveNote = () => {}
	}: {
		leads: any[];
		counts: {
			all: number;
			new: number;
			contacted: number;
			follow_up: number;
			interested: number;
			converted: number;
		};
		activeFilter: string;
		selectedLeadId: string | null;
		selectedRowIds: string[];
		showDrawer: boolean;
		activeLead: any;
		pipelineStates?: any[];
		users?: any[];
		notes?: any[];
		assignedUserIds?: string[];
		canManageAssignments?: boolean;
		onSelectFilter: (key: string) => void;
		onSelectLead: (lead: any) => void;
		onToggleCheckbox: (id: string, e: MouseEvent) => void;
		onToggleAllCheckboxes: (e: MouseEvent) => void;
		onCloseDrawer: () => void;
		onOpenChat: (convoId: string) => void;
		onChangeState: (leadId: string, stateKey: string) => void;
		onToggleAssignee: (conversationId: string, userId: string) => void;
		onAddTag: (leadId: string, tag: string) => void;
		onRemoveTag: (leadId: string, tag: string) => void;
		onSaveNote: (leadId: string, text: string) => void;
	} = $props();
</script>

<div class="flex-1 flex flex-col min-h-0 h-full overflow-hidden bg-white">
	<!-- Top Sub-Header: Filter Pills/Cards -->
	<div class="px-6 pb-3 flex items-center justify-between shrink-0 bg-white">
		<LeadStageFilters
			{activeFilter}
			{counts}
			{onSelectFilter}
		/>
	</div>

	<!-- Main Split Area: Full-height Left Table + Full-height Right Detail Drawer -->
	<div class="flex-1 flex min-h-0 h-full overflow-hidden">
		<LeadsTable
			{leads}
			totalLeadsCount={counts.all}
			{selectedLeadId}
			{selectedRowIds}
			{onSelectLead}
			{onToggleCheckbox}
			{onToggleAllCheckboxes}
		/>

		{#if showDrawer && activeLead}
			<LeadDetailDrawer
				lead={activeLead}
				{pipelineStates}
				{users}
				{notes}
				{assignedUserIds}
				{canManageAssignments}
				onClose={onCloseDrawer}
				{onOpenChat}
				{onChangeState}
				{onToggleAssignee}
				{onAddTag}
				{onRemoveTag}
				{onSaveNote}
			/>
		{/if}
	</div>
</div>
