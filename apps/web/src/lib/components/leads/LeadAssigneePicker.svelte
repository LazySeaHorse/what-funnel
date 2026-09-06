<script lang="ts">
	import UserAvatar from '../UserAvatar.svelte';

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
</script>

<div class="space-y-1.5 relative">
	<span class="font-medium text-slate-700">Assigned to</span>
	<div class="flex items-center gap-2">
		{#each assignedUserIds as id (id)}
			{@const user = users.find((item) => item.id === id)}
			<UserAvatar name={user?.name || user?.email || 'User'} avatar={user?.avatar_url || ''} size="md" class="ring-2 ring-white" />
		{/each}
		{#if assignedUserIds.length === 0}<span class="text-xs text-slate-400">Unassigned</span>{/if}
		<button type="button" onclick={() => (open = !open)} title="Assign conversation" aria-label="Assign conversation" class="w-8 h-8 rounded-full border border-dashed border-slate-300 text-slate-400 hover:text-slate-600 hover:border-slate-400 flex items-center justify-center text-sm transition cursor-pointer">+</button>
	</div>
	{#if open}
		<div class="absolute top-full left-0 mt-1 w-52 bg-white rounded-xl border border-slate-200 shadow-md py-1 z-50 text-xs">
			<div class="px-3 py-1.5 text-[10px] font-medium text-slate-400 uppercase tracking-wider border-b border-slate-100">Assign team member</div>
			{#each users as user (user.id)}
				{@const assigned = assignedUserIds.includes(user.id)}
				<button type="button" onclick={() => void onToggle(user.id)} class="w-full flex items-center justify-between px-3 py-2 text-left hover:bg-slate-50 font-medium cursor-pointer {assigned ? 'text-blue-600 bg-blue-50/50' : 'text-slate-700'}">
					<span class="truncate">{user.name || user.email}</span>{#if assigned}<span>✓</span>{/if}
				</button>
			{/each}
		</div>
	{/if}
</div>
