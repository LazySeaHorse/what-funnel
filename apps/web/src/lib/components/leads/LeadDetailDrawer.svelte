<script lang="ts">
	import ChannelBadge from '../ChannelBadge.svelte';
	import LeadStateBadge from '../LeadStateBadge.svelte';
	import UserAvatar from '../UserAvatar.svelte';

	let {
		lead,
		pipelineStates = [],
		users = [],
		notes = [],
		assignedUserIds = [],
		canManageAssignments = false,
		onClose = () => {},
		onOpenChat = () => {},
		onChangeState = () => {},
		onToggleAssignee = () => {},
		onAddTag = () => {},
		onRemoveTag = () => {},
		onSaveNote = () => {}
	}: {
		lead: any;
		pipelineStates?: any[];
		users?: any[];
		notes?: any[];
		assignedUserIds?: string[];
		canManageAssignments?: boolean;
		onClose: () => void;
		onOpenChat: (convoId: string) => void;
		onChangeState: (stateKey: string) => void;
		onToggleAssignee: (userId: string) => void;
		onAddTag: (tag: string) => void;
		onRemoveTag: (tag: string) => void;
		onSaveNote: (text: string) => void;
	} = $props();

	let activeTab = $state<'overview' | 'details' | 'notes' | 'activity'>('overview');
	let showStateDropdown = $state(false);
	let showAssignDropdown = $state(false);
	let showTagInput = $state(false);
	let tagInputText = $state('');
	let showAddNoteInput = $state(false);
	let noteInputText = $state('');

	const defaultStates = [
		{ key: 'new', label: 'New Lead' },
		{ key: 'contacted', label: 'Contacted' },
		{ key: 'follow_up', label: 'Follow-up' },
		{ key: 'interested', label: 'Interested' },
		{ key: 'converted', label: 'Converted' }
	];

	const availableStates = $derived(pipelineStates.length > 0 ? pipelineStates : defaultStates);
	const channelName = $derived(
		lead.channel ? lead.channel.charAt(0).toUpperCase() + lead.channel.slice(1) : 'Unknown channel'
	);

	function submitTag() {
		if (!tagInputText.trim()) return;
		onAddTag(tagInputText.trim());
		tagInputText = '';
		showTagInput = false;
	}

	function submitNote() {
		if (!noteInputText.trim()) return;
		onSaveNote(noteInputText.trim());
		noteInputText = '';
		showAddNoteInput = false;
	}

	function formatTime(timestamp?: string | number): string {
		return timestamp ? new Date(timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '';
	}
</script>

<aside class="lead-panel w-[320px] xl:w-[350px] bg-white flex flex-col shrink-0 overflow-y-auto min-h-0 h-full border-l border-slate-100 select-none">
	<!-- Top Drawer Actions -->
	<div class="px-5 pt-4 pb-2 flex items-center justify-between">
		<button
			onclick={() => onOpenChat(lead.convoId)}
			class="wf-button-primary px-3.5 py-1.5"
		>
			<svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
				<path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
			</svg>
			<span>Add lead</span>
		</button>

		<button
			onclick={onClose}
			title="Close details"
			aria-label="Close details"
			class="w-7 h-7 rounded-lg hover:bg-slate-100 flex items-center justify-center text-slate-400 hover:text-slate-600 transition cursor-pointer"
		>
			<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
				<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
			</svg>
		</button>
	</div>

	<!-- Contact Header Card -->
	<div class="px-5 py-3 flex items-start gap-3.5">
		<div class="relative">
			<UserAvatar name={lead.name} avatar={lead.avatar} size="2xl" />
			<div class="absolute -bottom-1 -right-1">
				<ChannelBadge channel={lead.channel} size="xs" showTooltip={false} />
			</div>
		</div>

		<div class="flex-1 min-w-0">
			<div class="flex items-center justify-between">
				<h2 class="font-medium text-sm text-slate-900 leading-tight truncate">{lead.name}</h2>
				<button class="text-slate-400 hover:text-slate-600 p-1 cursor-pointer" aria-label="More options for {lead.name}">
					<svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
						<circle cx="12" cy="5" r="1.5" />
						<circle cx="12" cy="12" r="1.5" />
						<circle cx="12" cy="19" r="1.5" />
					</svg>
				</button>
			</div>
			<div class="flex items-center gap-1.5 mt-1 text-xs text-slate-400 min-w-0">
				<ChannelBadge channel={lead.channel} size="xs" showTooltip={false} />
				<span class="truncate">{channelName} • {lead.handle || lead.name.toLowerCase().replace(/\s+/g, '_')}</span>
			</div>
		</div>
	</div>

	<!-- Drawer Nav Tabs -->
	<div class="flex items-center justify-between border-b border-slate-100 text-xs font-medium text-slate-400 pt-2 px-5">
		{#each [{ key: 'overview', label: 'Overview' }, { key: 'details', label: 'Details' }, { key: 'notes', label: 'Notes' }, { key: 'activity', label: 'Activity' }] as tab}
			{@const isActive = activeTab === tab.key}
			<button
				onclick={() => activeTab = tab.key as typeof activeTab}
				class="pb-2.5 px-1.5 transition cursor-pointer {isActive ? 'text-blue-600 font-medium border-b-2 border-blue-600' : 'hover:text-slate-700'}"
			>
				{tab.label}
			</button>
		{/each}
	</div>

	<!-- Drawer Content -->
	<div class="p-5 space-y-4 flex-1 text-xs">
		{#if activeTab === 'overview'}
			<!-- Lead State Section -->
			<div class="space-y-1.5 relative">
				<span class="font-medium text-slate-700">Lead State</span>
				<button
					onclick={() => showStateDropdown = !showStateDropdown}
					class="w-full flex items-center justify-between p-2.5 bg-amber-50/50 rounded-xl border border-amber-200/80 cursor-pointer hover:bg-amber-50 transition text-left"
				>
					<LeadStateBadge stateKey={lead.stateKey} label={lead.stateLabel} size="sm" class="border-0 bg-transparent p-0" />
					<svg class="w-3.5 h-3.5 text-amber-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
					</svg>
				</button>

				{#if showStateDropdown}
					<div class="absolute top-full left-0 right-0 mt-1 bg-white rounded-xl border border-slate-200 shadow-md py-1 z-50">
						{#each availableStates as state}
							<button
								onclick={() => { showStateDropdown = false; onChangeState(state.key); }}
								class="w-full text-left px-3 py-1.5 text-xs font-medium hover:bg-slate-50 text-slate-700 flex items-center gap-2 cursor-pointer"
							>
								<LeadStateBadge stateKey={state.key} label={state.label} size="xs" class="border-0 bg-transparent p-0" />
							</button>
						{/each}
					</div>
				{/if}
			</div>

			{#if canManageAssignments}
			<!-- Assigned to Section -->
			<div class="space-y-1.5 relative">
				<span class="font-medium text-slate-700">Assigned to</span>
				<div class="flex items-center gap-2">
					{#if lead.assignees && lead.assignees.length > 0}
						{#each lead.assignees as usr}
							<UserAvatar name={usr.name} avatar={usr.avatar} size="md" class="ring-2 ring-white" />
						{/each}
					{/if}
					<button
						onclick={() => showAssignDropdown = !showAssignDropdown}
						title="Add assignee"
						aria-label="Add assignee"
						class="w-8 h-8 rounded-full border border-dashed border-slate-300 text-slate-400 hover:text-slate-600 hover:border-slate-400 flex items-center justify-center text-sm transition cursor-pointer"
					>
						+
					</button>
				</div>

				{#if showAssignDropdown}
					<div class="absolute top-full left-0 mt-1 w-52 bg-white rounded-xl border border-slate-200 shadow-md py-1 z-50 text-xs">
						<div class="px-3 py-1.5 text-[10px] font-medium text-slate-400 uppercase tracking-wider border-b border-slate-100">Assign Team Member</div>
						{#each users as user}
							{@const isAssigned = assignedUserIds.includes(user.id)}
							<button
								onclick={() => onToggleAssignee(user.id)}
								class="w-full flex items-center justify-between px-3 py-2 text-left hover:bg-slate-50 font-medium cursor-pointer {isAssigned ? 'text-blue-600 bg-blue-50/50' : 'text-slate-700'}"
							>
								<span class="truncate">{user.name || user.email}</span>
								{#if isAssigned}<span>✓</span>{/if}
							</button>
						{/each}
					</div>
				{/if}
			</div>
			{/if}

			<!-- Tags Section -->
			<div class="space-y-1.5">
				<span class="font-medium text-slate-700">Tags</span>
				<div class="flex flex-wrap items-center gap-1.5">
					{#each lead.tags || [] as tag}
						<span class="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg bg-violet-50 text-violet-600 text-xs font-medium border border-violet-200">
							{tag}
							<button onclick={() => onRemoveTag(tag)} aria-label="Remove tag {tag}" class="text-violet-600/60 hover:text-violet-600 cursor-pointer">×</button>
						</span>
					{/each}
					{#if showTagInput}
						<input
							aria-label="Tag name"
							bind:value={tagInputText}
							onkeydown={(e) => e.key === 'Enter' && submitTag()}
							placeholder="Tag..."
							class="w-20 px-2 py-1 text-xs border border-blue-200 rounded-lg focus:outline-none"
						/>
						<button aria-label="Save tag" onclick={submitTag} class="text-xs font-medium text-blue-600 px-1 cursor-pointer">✓</button>
					{:else}
						<button
							onclick={() => showTagInput = true}
							title="Add tag"
							aria-label="Add tag"
							class="w-7 h-7 rounded-lg border border-dashed border-slate-300 text-slate-400 hover:text-slate-600 hover:border-slate-400 flex items-center justify-center text-xs transition cursor-pointer"
						>
							+
						</button>
					{/if}
				</div>
			</div>

			<!-- Notes Section -->
			<div class="space-y-1.5">
				<div class="flex items-center justify-between">
					<span class="font-medium text-slate-700">Notes</span>
				</div>
				{#if notes.length > 0}
					<div class="bg-slate-50 border border-slate-100 rounded-xl p-3.5 flex items-start justify-between gap-2">
						<div class="text-slate-600 leading-relaxed flex-1">{notes[0].body || notes[0].text}</div>
						<button onclick={() => showAddNoteInput = !showAddNoteInput} class="text-slate-400 hover:text-slate-600 shrink-0 p-0.5 cursor-pointer" title="Edit note" aria-label="Edit note">
							<svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
								<path stroke-linecap="round" stroke-linejoin="round" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
							</svg>
						</button>
					</div>
				{:else}
					<p class="rounded-xl border border-slate-100 bg-slate-50 p-3.5 text-slate-400">No notes yet.</p>
				{/if}
				{#if showAddNoteInput}
					<div class="p-3 bg-slate-50 rounded-xl space-y-2">
						<textarea
							bind:value={noteInputText}
							rows="2"
							placeholder="Add an internal note..."
							class="w-full p-2 text-xs bg-white border border-slate-200 rounded-lg outline-none focus:border-blue-500 resize-none"
						></textarea>
						<div class="flex justify-end gap-2">
							<button onclick={() => { showAddNoteInput = false; noteInputText = ''; }} class="text-xs text-slate-500 cursor-pointer">Cancel</button>
							<button onclick={submitNote} class="px-2.5 py-1 bg-blue-600 text-white rounded-lg text-xs cursor-pointer">Save</button>
						</div>
					</div>
				{/if}
			</div>

			<!-- Contact info Section -->
			<div class="space-y-2">
				<span class="font-medium text-slate-700">Contact info</span>
				
				<div class="space-y-1.5">
					{#each lead.contactInfo || [] as contact}
					<div class="flex items-center justify-between p-2.5 bg-slate-50 border border-slate-100 rounded-xl">
						<div class="flex items-center gap-2 min-w-0">
							<ChannelBadge channel={lead.channel} size="xs" showTooltip={false} />
							<span class="text-slate-700 font-medium truncate">{contact.value}</span>
						</div>
						<div class="flex items-center gap-1 text-slate-400 text-[11px]">
							<span>{contact.label}</span>
							<svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
								<path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
							</svg>
						</div>
					</div>
					{:else}
						<p class="rounded-xl border border-slate-100 bg-slate-50 p-3 text-slate-400">No contact identity available.</p>
					{/each}
				</div>
			</div>

		{:else if activeTab === 'details'}
			<div class="p-3.5 bg-slate-50 rounded-xl space-y-2.5 text-xs">
				<div class="flex justify-between gap-4 py-1 border-b border-slate-200/60">
					<span class="text-slate-400">Display Name</span>
					<span class="font-medium text-slate-800 text-right truncate">{lead.name}</span>
				</div>
				<div class="flex justify-between gap-4 py-1 border-b border-slate-200/60">
					<span class="text-slate-400">Channel</span>
					<span class="font-medium text-slate-800 capitalize">{channelName}</span>
				</div>
				<div class="flex justify-between gap-4 py-1 border-b border-slate-200/60">
					<span class="text-slate-400">Identity</span>
					<span class="font-medium text-slate-800 text-right truncate">{lead.handle || 'N/A'}</span>
				</div>
				<div class="flex justify-between gap-4 py-1">
					<span class="text-slate-400">Lead status</span>
					<span class="font-medium text-slate-800">{lead.stateLabel}</span>
				</div>
			</div>

		{:else if activeTab === 'notes'}
			<div class="space-y-3">
				<div class="flex items-center justify-between">
					<span class="font-medium text-slate-700">Internal Notes</span>
					<button onclick={() => showAddNoteInput = true} class="text-[11px] text-blue-600 font-medium hover:underline cursor-pointer">+ Add note</button>
				</div>
				{#if showAddNoteInput}
					<div class="p-3 bg-slate-50 rounded-xl space-y-2">
						<textarea bind:value={noteInputText} rows="2" placeholder="Add an internal note..." class="w-full p-2 text-xs bg-white border border-slate-200 rounded-lg outline-none focus:border-blue-500 resize-none"></textarea>
						<div class="flex justify-end gap-2">
							<button onclick={() => { showAddNoteInput = false; noteInputText = ''; }} class="text-xs text-slate-500 cursor-pointer">Cancel</button>
							<button onclick={submitNote} class="px-2.5 py-1 bg-blue-600 text-white rounded-lg text-xs cursor-pointer">Save</button>
						</div>
					</div>
				{/if}
				{#if notes.length === 0}
					<div class="p-3 bg-slate-50 rounded-xl text-xs text-slate-400">No notes yet. Add an internal note for your team.</div>
				{:else}
					<div class="space-y-2">
						{#each notes as note}
							<div class="p-3 rounded-xl bg-white border border-slate-200/80 text-xs text-slate-600 leading-relaxed">
								{note.body || note.text}
								<div class="text-[10px] text-slate-400 mt-1">{formatTime(note.created_at)}</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>

		{:else}
			<div class="space-y-3 text-xs">
				<div class="border-l-2 border-blue-200 ml-2 pl-3 space-y-4">
					<div class="relative">
						<span class="absolute -left-[19px] top-1 w-2.5 h-2.5 rounded-full bg-blue-600 ring-2 ring-white"></span>
						<div class="font-medium text-slate-800">Lead is currently {lead.stateLabel}</div>
						<div class="text-[11px] text-slate-400">Last updated {lead.updatedAt || 'recently'}</div>
					</div>
				</div>
			</div>
		{/if}
	</div>
</aside>
