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
		onSelectFilter: (key: string) => void;
		onSelectLead: (lead: any) => void;
		onToggleCheckbox: (id: string, e: MouseEvent) => void;
		onToggleAllCheckboxes: (e: MouseEvent) => void;
		onCloseDrawer: () => void;
		onOpenChat: (convoId: string) => void;
		onChangeState: (stateKey: string) => void;
		onToggleAssignee: (userId: string) => void;
		onAddTag: (tag: string) => void;
		onRemoveTag: (tag: string) => void;
		onSaveNote: (text: string) => void;
	} = $props();
</script>

<div class="flex-1 flex flex-col min-h-0 overflow-hidden space-y-4">
	<!-- Top Bar / Stage Filters -->
	<LeadStageFilters
		{activeFilter}
		{counts}
		{onSelectFilter}
	/>

	<!-- Main Split Area: Table on Left + Detail Drawer on Right -->
	<div class="flex-1 flex gap-3 min-h-0 overflow-hidden">
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
