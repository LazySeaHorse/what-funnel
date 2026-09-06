<script lang="ts">
	import ChannelBadge from '../ChannelBadge.svelte';
	import UserAvatar from '../UserAvatar.svelte';
	import LeadStagePicker from './LeadStagePicker.svelte';
	import LeadAssigneePicker from './LeadAssigneePicker.svelte';
	import LeadTagsEditor from './LeadTagsEditor.svelte';
	import LeadNotesEditor from './LeadNotesEditor.svelte';
	import type { LeadEditor } from '$lib/leads/lead-editor.svelte';
	import {
		PlusIcon,
		XMarkIcon,
		EllipsisVerticalIcon,
		ChevronDownIcon,
	} from '@fvilers/heroicons-svelte/24/outline';

	let {
		lead,
		editor,
		pipelineStates = [],
		users = [],
		canManageAssignments = false,
		onClose = () => {},
		onOpenChat = () => {}
	}: {
		lead: any;
		editor: LeadEditor;
		pipelineStates?: any[];
		users?: any[];
		canManageAssignments?: boolean;
		onClose: () => void;
		onOpenChat: (convoId: string) => void;
	} = $props();

	let activeTab = $state<'overview' | 'details' | 'notes' | 'activity'>('overview');

	const defaultStates = [
		{ key: 'new', label: 'New Lead' },
		{ key: 'contacted', label: 'Contacted' },
		{ key: 'follow_up', label: 'Follow-up' },
		{ key: 'interested', label: 'Interested' },
		{ key: 'converted', label: 'Converted' }
	];

	const channelName = $derived(
		lead.channel ? lead.channel.charAt(0).toUpperCase() + lead.channel.slice(1) : 'Unknown channel'
	);

</script>

<aside class="lead-panel w-[320px] xl:w-[350px] bg-white flex flex-col shrink-0 overflow-y-auto min-h-0 h-full border-l border-slate-100 select-none">
	<!-- Top Drawer Actions -->
	<div class="px-5 pt-4 pb-2 flex items-center justify-between">
		<button
			onclick={() => onOpenChat(lead.convoId)}
			class="wf-button-primary px-3.5 py-1.5"
		>
			<PlusIcon class="w-3.5 h-3.5" />
			<span>Add lead</span>
		</button>

		<button
			onclick={onClose}
			title="Close details"
			aria-label="Close details"
			class="w-7 h-7 rounded-lg hover:bg-slate-100 flex items-center justify-center text-slate-400 hover:text-slate-600 transition cursor-pointer"
		>
			<XMarkIcon class="w-4 h-4" />
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
					<EllipsisVerticalIcon class="w-4 h-4" />
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
			<LeadStagePicker stateKey={lead.stateKey} stateLabel={lead.stateLabel} states={pipelineStates.length ? pipelineStates : defaultStates} onchange={(key) => editor.changeStage(key)} />

			{#if canManageAssignments}
				<LeadAssigneePicker {users} assignedUserIds={editor.conversation?.assigned_user_ids ?? []} onToggle={(id) => editor.toggleAssignee(id)} />
			{/if}

			<LeadTagsEditor tags={editor.lead?.tags ?? lead.tags ?? []} onadd={(tag) => editor.addTag(tag)} onremove={(tag) => editor.removeTag(tag)} />
			<LeadNotesEditor notes={editor.notes} loading={editor.loading} onadd={(body) => editor.addNote(body)} />

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
							<ChevronDownIcon class="w-3 h-3" />
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
					<span class="text-slate-400">Display name</span>
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
					<span class="text-slate-400">Lead stage</span>
					<span class="font-medium text-slate-800">{lead.stateLabel}</span>
				</div>
			</div>

		{:else if activeTab === 'notes'}
			<LeadNotesEditor notes={editor.notes} loading={editor.loading} expanded onadd={(body) => editor.addNote(body)} />

		{:else}
			<div class="space-y-3 text-xs">
				<div class="border-l-2 border-blue-200 ml-2 pl-3 space-y-4">
					<div class="relative">
						<span class="absolute -left-[19px] top-1 w-2.5 h-2.5 rounded-full bg-blue-600 ring-2 ring-white"></span>
						<div class="font-medium text-slate-800">Lead stage: {lead.stateLabel}</div>
						<div class="text-[11px] text-slate-400">Updated: {lead.updatedAt || 'recently'}</div>
					</div>
				</div>
			</div>
		{/if}
	</div>
</aside>
