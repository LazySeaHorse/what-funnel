<script lang="ts">
	import UserAvatar from '../UserAvatar.svelte';
	import { Popover } from '$lib/components/ui';

	let {
		users = [],
		assignedUserIds = [],
		onToggle
	}: {
		users?: any[];
		assignedUserIds?: string[];
		onToggle: (userID: string) => void | Promise<void>;
	} = $props();
	let open = $state(false);

	function getUserDisplayName(user?: any): string {
		return user?.name?.trim() || user?.username?.trim() || user?.email?.trim() || 'User';
	}
</script>

<div class="space-y-1.5">
	<span class="font-medium text-slate-700">Assigned to</span>
	<div class="flex items-center gap-2">
		{#each assignedUserIds as id (id)}
			{@const user = users.find((item) => item.id === id)}
			<UserAvatar name={getUserDisplayName(user)} avatar={user?.avatar_url || ''} size="md" class="ring-2 ring-white" />
		{/each}
		{#if assignedUserIds.length === 0}<span class="text-xs text-slate-400">Unassigned</span>{/if}
		<Popover bind:open width="w-52">
			{#snippet trigger()}
				<button
					type="button"
					onclick={() => (open = !open)}
					title="Assign conversation"
					aria-label="Assign conversation"
					class="w-8 h-8 rounded-full border border-dashed border-slate-300 text-slate-400 hover:text-slate-600 hover:border-slate-400 active:scale-95 flex items-center justify-center text-sm transition cursor-pointer"
				>
					+
				</button>
			{/snippet}

			<div class="px-3 py-1.5 text-[10px] font-medium text-slate-400 uppercase tracking-wider border-b border-slate-100">Assign team member</div>
			{#each users as user (user.id)}
				{@const assigned = assignedUserIds.includes(user.id)}
				{@const displayName = getUserDisplayName(user)}
				<button
					type="button"
					onclick={() => void onToggle(user.id)}
					title={displayName}
					class="w-full flex items-center justify-between px-3 py-2 text-left hover:bg-slate-50 font-medium cursor-pointer {assigned ? 'text-blue-600 bg-blue-50/50' : 'text-slate-700'}"
				>
					<span class="truncate">{displayName}</span>{#if assigned}<span>✓</span>{/if}
				</button>
			{/each}
		</Popover>
	</div>
</div>
