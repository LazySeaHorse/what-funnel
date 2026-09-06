<script lang="ts">
	import { generatePassword, type UserCredentials } from './types';

	let {
		accountSlug,
		onclose,
		onadd
	}: {
		accountSlug: string;
		onclose: () => void;
		onadd: (input: { username: string; password: string; role: 'agent' | 'manager' }) => Promise<UserCredentials>;
	} = $props();
	let username = $state('');
	let password = $state(generatePassword());
	let role = $state<'agent' | 'manager'>('agent');
	let pending = $state(false);
	let error = $state('');

	async function submit() {
		if (!username.trim() || !password.trim()) return;
		pending = true;
		error = '';
		try {
			await onadd({ username: username.trim(), password: password.trim(), role });
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Failed to create user.';
		} finally {
			pending = false;
		}
	}
</script>

<div class="wf-modal-backdrop">
	<div class="wf-modal" role="dialog" aria-modal="true" aria-labelledby="add-team-member-title">
		<h3 id="add-team-member-title" class="text-sm font-medium text-slate-900">Add Team Member</h3>
		<div class="space-y-3.5 text-xs">
			<div class="space-y-1">
				<label for="newUsernameInput" class="font-medium text-slate-700">Username</label>
				<input id="newUsernameInput" type="text" bind:value={username} placeholder="e.g. john" class="wf-input" />
				<p class="text-[11px] text-slate-400">Login username: <span class="font-mono">{accountSlug || 'prefix'}-{username || '[username]'}</span></p>
			</div>
			<div class="space-y-1">
				<div class="flex items-center justify-between">
					<label for="newPasswordInput" class="font-medium text-slate-700">Initial password</label>
					<button type="button" onclick={() => (password = generatePassword())} class="text-[11px] text-blue-600 hover:underline cursor-pointer">Generate</button>
				</div>
				<input id="newPasswordInput" type="text" bind:value={password} placeholder="Password" class="wf-input font-mono" />
			</div>
			<div class="space-y-1">
				<label for="newRoleSelect" class="font-medium text-slate-700">Role</label>
				<select id="newRoleSelect" bind:value={role} class="wf-select"><option value="agent">Agent</option><option value="manager">Manager</option></select>
			</div>
			{#if error}<p class="text-xs text-rose-600 font-medium">{error}</p>{/if}
		</div>
		<div class="flex items-center justify-end gap-3 pt-2">
			<button type="button" onclick={onclose} class="px-4 py-2 text-xs font-medium text-slate-600 hover:bg-slate-100 rounded-xl">Cancel</button>
			<button type="button" onclick={() => void submit()} disabled={pending || !username.trim() || !password.trim()} class="wf-button-primary disabled:opacity-50">{pending ? 'Saving...' : 'Add user'}</button>
		</div>
	</div>
</div>
