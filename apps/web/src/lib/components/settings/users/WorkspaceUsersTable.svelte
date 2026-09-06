<script lang="ts">
	import { TrashIcon } from '@fvilers/heroicons-svelte/24/outline';
	import type { WorkspaceUser } from './types';

	let { users, accountSlug, currentUserID, onRoleChange, onReset, onDelete }: {
		users: WorkspaceUser[];
		accountSlug: string;
		currentUserID?: string;
		onRoleChange: (userID: string, role: string) => void | Promise<void>;
		onReset: (user: WorkspaceUser) => void;
		onDelete: (user: WorkspaceUser) => void;
	} = $props();
</script>

<div class="border border-slate-200 rounded-2xl overflow-hidden divide-y divide-slate-100 text-xs bg-white">
	{#each users as user (user.id)}
		<div class="p-4 flex items-center justify-between hover:bg-slate-50/50 transition">
			<div class="flex items-center gap-3 min-w-0">
				<div class="w-8 h-8 rounded-full bg-blue-100 text-blue-700 font-medium flex items-center justify-center text-xs shrink-0">{(user.username || user.name || user.email || 'U').charAt(0).toUpperCase()}</div>
				<div class="min-w-0">
					<div class="font-medium text-slate-800 truncate">{user.username || user.name || user.email?.split('@')[0]}</div>
					<div class="text-[11px] text-slate-400 font-mono truncate">{user.email || (accountSlug ? `${accountSlug}-${user.username}` : user.username)}</div>
				</div>
			</div>
			<div class="flex items-center gap-2.5 shrink-0">
				<select aria-label="Role for {user.username || user.email}" value={user.role} onchange={(event) => void onRoleChange(user.id, event.currentTarget.value)} class="rounded-lg border border-slate-200 bg-white px-2 py-1 text-[11px] font-medium capitalize text-slate-700 focus:border-blue-500 focus:outline-none cursor-pointer"><option value="agent">Agent</option><option value="manager">Manager</option></select>
				<button type="button" class="px-2.5 py-1 text-[11px] font-medium text-slate-600 hover:text-slate-900 bg-slate-100 hover:bg-slate-200 rounded-lg transition cursor-pointer" onclick={() => onReset(user)} title="Reset Password">Reset password</button>
				{#if user.id !== currentUserID}<button type="button" class="p-1 text-slate-400 hover:text-rose-600 rounded-lg hover:bg-rose-50 transition cursor-pointer" onclick={() => onDelete(user)} title="Delete user"><TrashIcon class="w-4 h-4" /></button>{/if}
			</div>
		</div>
	{/each}
</div>
