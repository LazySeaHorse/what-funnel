<script lang="ts">
  import { ExclamationTriangleIcon } from "@fvilers/heroicons-svelte/24/outline";
  import { Modal, Button } from "$lib/components/ui";

  let {
    workspaceName,
    saving,
    onDelete,
    onClose,
  }: {
    workspaceName: string;
    saving: boolean;
    onDelete: () => void;
    onClose: () => void;
  } = $props();

  let confirmation = $state("");
</script>

<Modal ariaLabelledby="delete-workspace-title" onclose={onClose} showClose={false}>
  {#snippet header()}
    <div class="flex items-center gap-3 text-red-600">
      <div
        class="w-10 h-10 rounded-xl bg-red-50 flex items-center justify-center shrink-0"
      >
        <ExclamationTriangleIcon class="w-5 h-5" />
      </div>
      <div>
        <h3
          id="delete-workspace-title"
          class="text-sm font-medium text-slate-900"
        >
          Delete Workspace
        </h3>
        <p class="text-xs text-slate-500">
          This will permanently delete all leads, messages, and settings.
        </p>
      </div>
    </div>
  {/snippet}

  <div class="text-xs text-slate-600 space-y-2">
    <p>
      Please type <span class="font-medium text-slate-900 select-all"
        >{workspaceName}</span
      > to confirm:
    </p>
    <input
      type="text"
      bind:value={confirmation}
      placeholder={workspaceName}
      class="wf-input focus:border-red-500 focus:ring-red-100"
    />
  </div>

  {#snippet footer()}
    <Button variant="ghost" onclick={onClose}>Cancel</Button>
    <Button
      variant="danger"
      busy={saving}
      disabled={confirmation !== workspaceName || saving}
      onclick={onDelete}
    >
      Delete permanently
    </Button>
  {/snippet}
</Modal>
