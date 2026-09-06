<script lang="ts">
	import { ExclamationTriangleIcon } from '@fvilers/heroicons-svelte/24/outline';
	let { workspaceName, saving, onDelete, onClose }: { workspaceName: string; saving: boolean; onDelete: () => void; onClose: () => void } = $props();
	let confirmation = $state('');
</script>

<div class="wf-modal-backdrop">
	<div class="wf-modal" role="dialog" aria-modal="true" aria-labelledby="delete-workspace-title">
		<div class="flex items-center gap-3 text-red-600"><div class="w-10 h-10 rounded-xl bg-red-50 flex items-center justify-center shrink-0"><ExclamationTriangleIcon class="w-5 h-5" /></div><div><h3 id="delete-workspace-title" class="text-sm font-medium text-slate-900">Delete Workspace</h3><p class="text-xs text-slate-500">This will permanently delete all leads, messages, and settings.</p></div></div>
		<div class="text-xs text-slate-600 space-y-2"><p>Please type <span class="font-medium text-slate-900 select-all">{workspaceName}</span> to confirm:</p><input type="text" bind:value={confirmation} placeholder={workspaceName} class="wf-input focus:border-red-500 focus:ring-red-100" /></div>
		<div class="flex items-center justify-end gap-3 pt-2"><button onclick={onClose} class="px-4 py-2 text-xs font-medium text-slate-600 hover:bg-slate-100 rounded-xl">Cancel</button><button disabled={confirmation !== workspaceName || saving} onclick={onDelete} class="wf-button-danger">Delete permanently</button></div>
	</div>
</div>
