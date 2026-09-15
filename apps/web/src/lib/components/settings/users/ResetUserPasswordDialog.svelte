<script lang="ts">
	import { generatePassword, type UserCredentials, type WorkspaceUser } from './types';
	import { Modal, Button, Input } from '$lib/components/ui';

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
		<Input id="resetPasswordInput" label="New Password" type="text" bind:value={password} class="font-mono">
			{#snippet trailing()}
				<button type="button" onclick={() => (password = generatePassword())} class="text-[11px] text-blue-600 hover:underline cursor-pointer">Generate</button>
			{/snippet}
		</Input>
		{#if error}<p class="text-xs font-medium text-rose-600">{error}</p>{/if}
	</div>

	{#snippet footer()}
		<Button variant="ghost" onclick={onclose}>Cancel</Button>
		<Button variant="primary" onclick={() => void submit()} disabled={pending || !password.trim()} busy={pending}>Set Password</Button>
	{/snippet}
</Modal>
