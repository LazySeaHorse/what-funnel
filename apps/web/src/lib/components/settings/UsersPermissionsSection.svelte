<script lang="ts">
	import { onMount } from 'svelte';
	import { apiRequest } from '$lib/api';
	import type { InboxState } from '$lib/store.svelte';
	import type { WorkspaceState } from '$lib/workspace.svelte';
	import { PlusIcon } from '@fvilers/heroicons-svelte/24/outline';
	import AddUserDialog from './users/AddUserDialog.svelte';
	import DeleteUserDialog from './users/DeleteUserDialog.svelte';
	import ResetUserPasswordDialog from './users/ResetUserPasswordDialog.svelte';
	import UserCredentialsDialog from './users/UserCredentialsDialog.svelte';
	import WorkspaceSlugCard from './users/WorkspaceSlugCard.svelte';
	import WorkspaceUsersTable from './users/WorkspaceUsersTable.svelte';
	import type { UserCredentials, WorkspaceUser } from './users/types';

	let { inbox, workspace, onStatus }: {
		inbox?: InboxState;
		workspace?: WorkspaceState;
		onStatus: (kind: 'success' | 'error', message: string) => void;
	} = $props();

	type UserDialog =
		| { kind: 'closed' }
		| { kind: 'add' }
		| { kind: 'credentials'; credentials: UserCredentials }
		| { kind: 'reset'; user: WorkspaceUser }
		| { kind: 'delete'; user: WorkspaceUser };

	let accountSlug = $state('');
	let users = $state<WorkspaceUser[]>([]);
	let dialog = $state<UserDialog>({ kind: 'closed' });

	onMount(() => { users = workspace?.users ?? []; });

	function closeDialog() {
		dialog = { kind: 'closed' };
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') closeDialog();
	}

	async function refreshUsers() {
		if (workspace) {
			await workspace.refreshUsers();
			users = workspace.users;
		} else {
			users = await apiRequest('/workspace/users');
		}
	}

	async function updateUserRole(userID: string, role: string) {
		try {
			await apiRequest(`/workspace/users/${userID}/role`, { method: 'PUT', body: { role } });
			await refreshUsers();
			onStatus('success', 'User role updated.');
		} catch (error) {
			onStatus('error', error instanceof Error ? error.message : 'Failed to update user role.');
		}
	}

	async function addUser(input: { username: string; password: string; role: 'agent' | 'manager' }): Promise<UserCredentials> {
		const response = await apiRequest('/workspace/users', { method: 'POST', body: input });
		const credentials = {
			username: response.username || input.username,
			role: response.role || input.role,
			plaintextPassword: response.password || input.password
		};
		await refreshUsers();
		dialog = { kind: 'credentials', credentials };
		return credentials;
	}

	async function resetPassword(user: WorkspaceUser, password: string): Promise<UserCredentials> {
		await apiRequest(`/workspace/users/${user.id}/password`, { method: 'PUT', body: { password } });
		const credentials = { username: user.username || user.email || 'user', role: user.role, plaintextPassword: password };
		onStatus('success', 'Password reset successfully.');
		dialog = { kind: 'credentials', credentials };
		return credentials;
	}

	async function deleteUser(user: WorkspaceUser) {
		await apiRequest(`/workspace/users/${user.id}`, { method: 'DELETE' });
		closeDialog();
		await refreshUsers();
		onStatus('success', 'User deleted and conversations unassigned.');
	}

	function resetSelectedPassword(password: string) {
		if (dialog.kind !== 'reset') throw new Error('No user selected for password reset.');
		return resetPassword(dialog.user, password);
	}

	function deleteSelectedUser() {
		if (dialog.kind !== 'delete') throw new Error('No user selected for deletion.');
		return deleteUser(dialog.user);
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h2 class="text-base font-medium text-slate-900">Users & permissions</h2>
			<p class="text-xs text-slate-500 mt-0.5">Manage team members and their workspace access.</p>
		</div>
		<button type="button" onclick={() => (dialog = { kind: 'add' })} class="px-3.5 py-2 bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium rounded-xl flex items-center gap-1.5 transition cursor-pointer shadow-xs"><PlusIcon class="w-3.5 h-3.5" /><span>Add user</span></button>
	</div>
	<WorkspaceSlugCard bind:slug={accountSlug} {onStatus} />
	<WorkspaceUsersTable {users} {accountSlug} currentUserID={inbox?.currentUser?.id} onRoleChange={updateUserRole} onReset={(user) => (dialog = { kind: 'reset', user })} onDelete={(user) => (dialog = { kind: 'delete', user })} />
</div>

{#if dialog.kind === 'add'}
	<AddUserDialog {accountSlug} onclose={closeDialog} onadd={addUser} />
{:else if dialog.kind === 'credentials'}
	<UserCredentialsDialog {accountSlug} credentials={dialog.credentials} onclose={closeDialog} />
{:else if dialog.kind === 'reset'}
	<ResetUserPasswordDialog user={dialog.user} onclose={closeDialog} onreset={resetSelectedPassword} />
{:else if dialog.kind === 'delete'}
	<DeleteUserDialog user={dialog.user} onclose={closeDialog} ondelete={deleteSelectedUser} />
{/if}
