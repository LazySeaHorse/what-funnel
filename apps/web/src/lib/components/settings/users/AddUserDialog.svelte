<script lang="ts">
	import { generatePassword, type UserCredentials } from './types';
	import { Modal, Button } from '$lib/components/ui';

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

<Modal title="Add Team Member" ariaLabelledby="add-team-member-title" onclose={onclose}>
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

	{#snippet footer()}
		<Button variant="ghost" onclick={onclose}>Cancel</Button>
		<Button
			variant="primary"
			onclick={() => void submit()}
			disabled={pending || !username.trim() || !password.trim()}
			busy={pending}
		>
			Add user
		</Button>
	{/snippet}
</Modal>
