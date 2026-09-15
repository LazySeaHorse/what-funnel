<script lang="ts">
	import { generatePassword, type UserCredentials, type WorkspaceUser } from './types';
	import { Modal, Button } from '$lib/components/ui';

	let { user, onclose, onreset }: { user: WorkspaceUser; onclose: () => void; onreset: (password: string) => Promise<UserCredentials> } = $props();
	let password = $state(generatePassword());
	let pending = $state(false);
	let error = $state('');

	async function submit() {
		if (!password.trim()) return;
		pending = true;
		error = '';
		try { await onreset(password.trim()); }
		catch (cause) { error = cause instanceof Error ? cause.message : 'Failed to reset password.'; }
		finally { pending = false; }
	}
</script>

<Modal title="Reset User Password" ariaLabelledby="reset-user-password-title" onclose={onclose}>
	<p class="text-xs text-slate-500">Set a new password for <span class="font-medium text-slate-800">{user.username || user.email}</span>.</p>
	<div class="space-y-1.5 text-xs">
		<div class="flex items-center justify-between">
			<label for="resetPasswordInput" class="font-medium text-slate-700">New Password</label>
			<button type="button" onclick={() => (password = generatePassword())} class="text-[11px] text-blue-600 hover:underline cursor-pointer">Generate</button>
		</div>
		<input id="resetPasswordInput" type="text" bind:value={password} class="wf-input font-mono" />
		{#if error}<p class="text-xs font-medium text-rose-600">{error}</p>{/if}
	</div>

	{#snippet footer()}
		<Button variant="ghost" onclick={onclose}>Cancel</Button>
		<Button variant="primary" onclick={() => void submit()} disabled={pending || !password.trim()} busy={pending}>Set Password</Button>
	{/snippet}
</Modal>
