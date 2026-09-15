<script lang="ts">
  import { Modal, Button } from "$lib/components/ui";

  let { onClose }: { onClose: () => void } = $props();
</script>

<Modal title="Workspace Plan" ariaLabelledby="workspace-plan-title" onclose={onClose}>
  <div
    class="p-4 rounded-xl bg-blue-50/60 border border-blue-100 space-y-2 text-xs"
  >
    <div class="flex justify-between font-medium text-slate-900">
      <span>Pro Plan</span><span class="text-blue-600">$49 / mo</span>
    </div>
    <p class="text-slate-600 text-[11px]">
      Unlimited channels, 10 team seats, 20 GB media storage, and autonomous
      AI replies.
    </p>
  </div>

  {#snippet footer()}
    <Button variant="primary" onclick={onClose}>Done</Button>
  {/snippet}
</Modal>
