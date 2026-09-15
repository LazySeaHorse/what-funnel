<script lang="ts">
	import type { WorkspaceUser } from './types';
	import { Modal, Button } from '$lib/components/ui';

	let { user, onclose, ondelete }: { user: WorkspaceUser; onclose: () => void; ondelete: () => Promise<void> } = $props();
	let pending = $state(false);
	async function remove() {
		pending = true;
		try { await ondelete(); }
		finally { pending = false; }
	}
</script>

<Modal title="Delete User" ariaLabelledby="delete-user-modal-title" onclose={onclose}>
	<p class="text-xs text-slate-600 leading-relaxed">Are you sure you want to delete <span class="font-medium text-slate-900">{user.username || user.email}</span>? Any conversations currently assigned to them will be unassigned. This action cannot be undone.</p>

	{#snippet footer()}
		<Button variant="ghost" onclick={onclose}>Cancel</Button>
		<Button variant="danger" onclick={() => void remove()} disabled={pending} busy={pending}>Delete User</Button>
	{/snippet}
</Modal>
