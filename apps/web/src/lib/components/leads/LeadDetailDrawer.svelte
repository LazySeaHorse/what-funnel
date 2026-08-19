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
		onClose: () => void;
		onOpenChat: (convoId: string) => void;
		onChangeState: (stateKey: string) => void;
		onToggleAssignee: (userId: string) => void;
		onAddTag: (tag: string) => void;
		onRemoveTag: (tag: string) => void;
		onSaveNote: (text: string) => void;
	} = $props();

	let leadDrawerTab = $state<'overview' | 'details' | 'notes' | 'activity'>('overview');
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

	const availableStates = $derived(
		pipelineStates && pipelineStates.length > 0 ? pipelineStates : defaultStates
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
		if (!timestamp) return '';
		const d = new Date(timestamp);
		return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	}
</script>

<div class="w-[340px] xl:w-[380px] bg-white flex flex-col shrink-0 min-h-0 h-full overflow-y-auto p-5 space-y-5 border-l border-slate-100">
	<!-- Top Action Buttons -->
	<div class="flex items-center justify-between shrink-0">
		<button
			onclick={() => onOpenChat(lead.convoId)}
			class="flex items-center gap-1.5 px-3.5 py-1.5 bg-blue-600 hover:bg-blue-700 text-white rounded-xl text-xs font-medium transition cursor-pointer shadow-xs active:scale-[0.98]"
		>
			<span>Open Chat</span>
			<svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
				<path stroke-linecap="round" stroke-linejoin="round" d="M14 5l7 7m0 0l-7 7m7-7H3" />
			</svg>
		</button>

		<button
			onclick={onClose}
			title="Close details"
			aria-label="Close details"
			class="w-7 h-7 rounded-lg hover:bg-slate-100 flex items-center justify-center text-slate-400 hover:text-slate-600 transition"
		>
			<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
				<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
			</svg>
		</button>
	</div>

	<!-- Lead Profile Header Card -->
	<div class="flex items-center justify-between pb-1">
		<div class="flex items-center gap-3.5 min-w-0">
			<UserAvatar name={lead.name} avatar={lead.avatar} size="xl" channel={lead.channel} />

			<div class="min-w-0">
				<h2 class="text-sm font-semibold text-slate-900 truncate leading-tight">{lead.name}</h2>
				<div class="flex items-center gap-1.5 text-xs text-slate-400 mt-1">
					<ChannelBadge channel={lead.channel} size="xs" showTooltip={false} />
					<span class="capitalize">{lead.channel}</span>
					{#if lead.handle}
						<span>•</span>
						<span class="truncate">{lead.handle}</span>
					{/if}
				</div>
			</div>
		</div>
	</div>

	<!-- Sub Navigation Tabs -->
	<div class="flex items-center gap-4 border-b border-slate-100 text-xs">
		{#each [
			{ key: 'overview', label: 'Overview' },
			{ key: 'details', label: 'Details' },
			{ key: 'notes', label: 'Notes' },
			{ key: 'activity', label: 'Activity' }
		] as tab}
			<button
				onclick={() => leadDrawerTab = tab.key as any}
				class="pb-2.5 font-medium transition relative {leadDrawerTab === tab.key ? 'text-blue-600' : 'text-slate-400 hover:text-slate-700'}"
			>
				<span>{tab.label}</span>
				{#if leadDrawerTab === tab.key}
					<span class="absolute bottom-0 left-0 right-0 h-0.5 bg-blue-600 rounded-full"></span>
				{/if}
			</button>
		{/each}
	</div>

	<!-- Lead State Selector -->
	<div class="space-y-1.5 relative">
		<div class="text-[11px] font-medium text-slate-500">Lead State</div>
		<button
			onclick={() => showStateDropdown = !showStateDropdown}
			class="w-full flex items-center justify-between p-2.5 rounded-xl border border-amber-200/80 bg-amber-50/40 text-xs font-medium text-amber-800 hover:bg-amber-50 transition cursor-pointer"
		>
			<LeadStateBadge stateKey={lead.stateKey} label={lead.stateLabel} size="sm" class="border-0 bg-transparent p-0" />
			<svg class="w-4 h-4 text-amber-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
				<path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
			</svg>
		</button>

		{#if showStateDropdown}
			<div class="absolute top-full left-0 right-0 mt-1 bg-white rounded-xl border border-slate-200 shadow-xl py-1 z-50 text-xs space-y-0.5">
				{#each availableStates as opt}
					<button
						onclick={() => { showStateDropdown = false; onChangeState(opt.key); }}
						class="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-slate-50 font-medium {lead.stateKey === opt.key ? 'text-blue-600 bg-blue-50/50' : 'text-slate-700'}"
					>
						<LeadStateBadge stateKey={opt.key} label={opt.label} size="xs" class="border-0 bg-transparent p-0" />
					</button>
				{/each}
			</div>
		{/if}
	</div>

	<!-- Assigned to -->
	<div class="space-y-1.5 relative">
		<div class="text-[11px] font-medium text-slate-500">Assigned to</div>
		<div class="flex items-center gap-2">
			{#if lead.assignees && lead.assignees.length > 0}
				{#each lead.assignees as usr}
					<UserAvatar name={usr.name} avatar={usr.avatar} size="md" />
				{/each}
			{:else}
				<span class="text-xs text-slate-400 italic">Unassigned</span>
			{/if}
			<button
				onclick={() => showAssignDropdown = !showAssignDropdown}
				class="w-8 h-8 rounded-full border border-dashed border-slate-300 hover:border-slate-400 hover:bg-slate-50 flex items-center justify-center text-slate-500 transition cursor-pointer"
				title="Assign user"
				aria-label="Assign user"
			>
				<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
					<path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
				</svg>
			</button>
		</div>

		{#if showAssignDropdown}
			<div class="absolute top-full left-0 mt-1 w-52 bg-white rounded-xl border border-slate-200 shadow-xl py-1 z-50 text-xs">
				<div class="px-3 py-1.5 text-[10px] font-medium text-slate-400 uppercase tracking-wider border-b border-slate-100">
					Assign Team Member
				</div>
				{#each users as u}
					{@const isAssigned = (assignedUserIds || []).includes(u.id)}
					<button
						onclick={() => onToggleAssignee(u.id)}
						class="w-full flex items-center justify-between px-3 py-2 text-left hover:bg-slate-50 font-medium {isAssigned ? 'text-blue-600 bg-blue-50/50' : 'text-slate-700'}"
					>
						<div class="flex items-center gap-2 truncate">
							<UserAvatar name={u.name || u.email} size="xs" />
							<span class="truncate">{u.name || u.email}</span>
						</div>
						{#if isAssigned}
							<svg class="w-4 h-4 text-blue-600 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
								<path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
							</svg>
						{/if}
					</button>
				{/each}
			</div>
		{/if}
	</div>

	<!-- Tags -->
	<div class="space-y-1.5">
		<div class="text-[11px] font-medium text-slate-500">Tags</div>
		<div class="flex flex-wrap items-center gap-1.5">
			{#each (lead.tags || []) as tag}
				<span class="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg bg-blue-50/70 border border-blue-200/60 text-blue-700 text-xs font-medium">
					<span>{tag}</span>
					<button onclick={() => onRemoveTag(tag)} class="text-blue-400 hover:text-blue-600 text-[10px]" aria-label="Remove tag {tag}">×</button>
				</span>
			{/each}

			{#if showTagInput}
				<div class="flex items-center gap-1">
					<input
						type="text"
						bind:value={tagInputText}
						placeholder="Tag name..."
						onkeydown={(e) => { if (e.key === 'Enter') submitTag(); }}
						class="w-24 px-2 py-0.5 text-xs bg-white border border-blue-400 rounded-lg outline-none"
					/>
					<button onclick={submitTag} class="px-1.5 py-0.5 bg-blue-600 text-white rounded text-[11px]">Add</button>
				</div>
			{:else}
				<button
					onclick={() => showTagInput = true}
					class="w-6 h-6 rounded-lg border border-dashed border-slate-300 hover:border-slate-400 hover:bg-slate-50 flex items-center justify-center text-slate-500 transition cursor-pointer"
					title="Add tag"
					aria-label="Add tag"
				>
					<svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
					</svg>
				</button>
			{/if}
		</div>
	</div>

	<!-- AI Summary Box -->
	<div class="space-y-1.5">
		<div class="flex items-center gap-1 text-[11px] font-medium text-slate-800">
			<span>AI Summary</span>
			<span class="text-blue-600 font-bold">✦</span>
		</div>
		
		<div class="p-3.5 bg-slate-50/90 rounded-xl border border-slate-200/60 space-y-2 text-xs text-slate-600 leading-relaxed">
			<ul class="space-y-1">
				{#each lead.aiSummary.bullets as bullet}
					<li class="flex items-start gap-1.5">
						<span class="text-slate-400 mt-1">•</span>
						<span>{bullet}</span>
					</li>
				{/each}
			</ul>

			<div class="border-t border-slate-200/60 pt-2 mt-2">
				<div class="text-[11px] font-medium text-blue-600 leading-tight">Suggested next step</div>
				<div class="text-xs text-slate-700 font-normal mt-0.5">{lead.aiSummary.suggestedNextStep}</div>
			</div>
		</div>
	</div>

	<!-- Notes -->
	<div class="space-y-1.5">
		<div class="flex items-center justify-between">
			<div class="text-[11px] font-medium text-slate-500">Notes</div>
			{#if !showAddNoteInput}
				<button onclick={() => showAddNoteInput = true} class="text-[11px] text-blue-600 font-medium hover:underline">+ Add note</button>
			{/if}
		</div>

		{#if showAddNoteInput}
			<div class="space-y-2 p-2.5 bg-slate-50/80 rounded-xl border border-slate-200">
				<textarea
					bind:value={noteInputText}
					placeholder="Add an internal note about this lead..."
					rows="2"
					class="w-full p-2 text-xs bg-white border border-slate-200 rounded-lg outline-none focus:border-blue-500 resize-none"
				></textarea>
				<div class="flex items-center justify-end gap-1.5">
					<button onclick={() => { showAddNoteInput = false; noteInputText = ''; }} class="px-2.5 py-1 text-xs text-slate-500 hover:bg-slate-100 rounded-lg">Cancel</button>
					<button onclick={submitNote} class="px-3 py-1 text-xs bg-blue-600 text-white font-medium rounded-lg hover:bg-blue-700">Save</button>
				</div>
			</div>
		{/if}

		{#if notes && notes.length > 0}
			<div class="space-y-2 max-h-36 overflow-y-auto">
				{#each notes as note}
					<div class="p-2.5 bg-slate-50/70 rounded-xl border border-slate-200/60 text-xs space-y-1">
						<div class="flex items-center justify-between text-[10px] text-slate-400">
							<span class="font-medium text-slate-600">{note.author_name || note.user_email || 'Team Member'}</span>
							<span class="tabular-nums">{formatTime(note.created_at)}</span>
						</div>
						<p class="text-slate-700 leading-relaxed">{note.body || note.text}</p>
					</div>
				{/each}
			</div>
		{:else if !showAddNoteInput}
			<div class="p-3 bg-slate-50/70 rounded-xl border border-slate-200/60 text-xs text-slate-400 italic">
				No notes yet. Click "+ Add note" to add an internal note.
			</div>
		{/if}
	</div>

	<!-- Contact info -->
	<div class="space-y-2 pt-1 border-t border-slate-100">
		<div class="text-[11px] font-medium text-slate-500">Contact info</div>
		
		{#each lead.contactInfo as info}
			<div class="flex items-center justify-between p-2.5 bg-slate-50/50 rounded-xl border border-slate-100 text-xs">
				<div class="flex items-center gap-2.5 min-w-0">
					<ChannelBadge channel={info.type} size="xs" showTooltip={false} />
					<span class="text-slate-800 font-medium truncate">{info.value}</span>
				</div>

				<div class="flex items-center gap-1 text-[11px] text-slate-400">
					<span>{info.label}</span>
				</div>
			</div>
		{/each}
	</div>
</div>
