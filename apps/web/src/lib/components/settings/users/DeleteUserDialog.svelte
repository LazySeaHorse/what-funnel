<script lang="ts">
	import type { WorkspaceUser } from './types';

	let { user, onclose, ondelete }: { user: WorkspaceUser; onclose: () => void; ondelete: () => Promise<void> } = $props();
	let pending = $state(false);
	async function remove() {
		pending = true;
		try { await ondelete(); }
		finally { pending = false; }
	}
</script>

<div class="wf-modal-backdrop">
	<div class="wf-modal" role="dialog" aria-modal="true" aria-labelledby="delete-user-modal-title">
		<h3 id="delete-user-modal-title" class="text-sm font-medium text-slate-900">Delete User</h3>
		<p class="text-xs text-slate-600">Are you sure you want to delete <span class="font-medium">{user.username || user.email}</span>? Any conversations currently assigned to them will be unassigned. This action cannot be undone.</p>
		<div class="flex justify-end gap-3">
			<button type="button" onclick={onclose} class="px-4 py-2 text-xs">Cancel</button>
			<button type="button" onclick={() => void remove()} disabled={pending} class="wf-button-danger disabled:opacity-50">{pending ? 'Deleting...' : 'Delete User'}</button>
		</div>
	</div>
</div>
