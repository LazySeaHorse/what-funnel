<script lang="ts">
	import { generatePassword, type UserCredentials, type WorkspaceUser } from './types';

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

<div class="wf-modal-backdrop">
	<div class="wf-modal" role="dialog" aria-modal="true" aria-labelledby="reset-user-password-title">
		<h3 id="reset-user-password-title" class="text-sm font-medium text-slate-900">Reset User Password</h3>
		<p class="text-xs text-slate-500">Set a new password for <span class="font-medium">{user.username || user.email}</span>.</p>
		<div class="space-y-1 text-xs">
			<div class="flex justify-between"><label for="resetPasswordInput">New Password</label><button type="button" onclick={() => (password = generatePassword())} class="text-blue-600">Generate</button></div>
			<input id="resetPasswordInput" type="text" bind:value={password} class="wf-input font-mono" />
			{#if error}<p class="text-rose-600">{error}</p>{/if}
		</div>
		<div class="flex justify-end gap-3">
			<button type="button" onclick={onclose} class="px-4 py-2 text-xs">Cancel</button>
			<button type="button" onclick={() => void submit()} disabled={pending || !password.trim()} class="wf-button-primary disabled:opacity-50">{pending ? 'Updating...' : 'Set Password'}</button>
		</div>
	</div>
</div>
