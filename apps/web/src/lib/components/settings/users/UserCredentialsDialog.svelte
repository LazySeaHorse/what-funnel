<script lang="ts">
	import { onDestroy } from 'svelte';
	import { CheckIcon } from '@fvilers/heroicons-svelte/24/outline';
	import type { UserCredentials } from './types';

	let { accountSlug, credentials, onclose }: { accountSlug: string; credentials: UserCredentials; onclose: () => void } = $props();
	let copied = $state(false);
	let copyTimer: ReturnType<typeof setTimeout> | null = null;
	let login = $derived(accountSlug ? `${accountSlug}-${credentials.username}` : credentials.username);
	onDestroy(() => { if (copyTimer) clearTimeout(copyTimer); });

	async function copy() {
		if (!credentials.plaintextPassword) return;
		try {
			await navigator.clipboard.writeText(`Login: ${login}\nPassword: ${credentials.plaintextPassword}`);
			copied = true;
			if (copyTimer) clearTimeout(copyTimer);
			copyTimer = setTimeout(() => (copied = false), 2000);
		} catch {
			// Clipboard access can be unavailable in non-secure browser contexts.
		}
	}
</script>

<div class="wf-modal-backdrop">
	<div class="wf-modal max-w-md" role="dialog" aria-modal="true" aria-labelledby="user-credentials-title">
		<div class="flex items-center gap-3">
			<div class="w-9 h-9 rounded-xl bg-emerald-50 text-emerald-600 flex items-center justify-center"><CheckIcon class="w-5 h-5" /></div>
			<div>
				<h3 id="user-credentials-title" class="text-sm font-medium text-slate-900">User credentials</h3>
				<p class="text-xs text-slate-500">Save these credentials now. The system does not show this password again.</p>
			</div>
		</div>
		<div class="space-y-3 p-4 bg-slate-50 border border-slate-200 rounded-xl text-xs font-mono">
			<div class="flex justify-between"><span>Login username:</span><span>{login}</span></div>
			<div class="flex justify-between"><span>Role:</span><span class="capitalize">{credentials.role}</span></div>
			{#if credentials.plaintextPassword}<div class="flex justify-between"><span>Password:</span><span class="text-blue-700">{credentials.plaintextPassword}</span></div>{/if}
		</div>
		<div class="flex justify-end gap-3">
			{#if credentials.plaintextPassword}<button type="button" onclick={() => void copy()} class="px-4 py-2 text-xs bg-slate-100 rounded-xl">{copied ? 'Copied!' : 'Copy credentials'}</button>{/if}
			<button type="button" onclick={onclose} class="wf-button-primary px-4 py-2">Done</button>
		</div>
	</div>
</div>
