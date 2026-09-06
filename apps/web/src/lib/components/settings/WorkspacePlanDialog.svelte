<script lang="ts">
  let { onClose }: { onClose: () => void } = $props();
</script>

<div class="wf-modal-backdrop">
  <div
    class="wf-modal"
    role="dialog"
    aria-modal="true"
    aria-labelledby="workspace-plan-title"
  >
    <div class="flex items-center justify-between">
      <h3 id="workspace-plan-title" class="text-sm font-medium text-slate-900">
        Workspace Plan
      </h3>
      <button
        onclick={onClose}
        aria-label="Close workspace plan"
        class="text-slate-400 hover:text-slate-600 text-lg font-medium cursor-pointer"
        >×</button
      >
    </div>
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
    <div class="flex justify-end pt-2">
      <button onclick={onClose} class="wf-button-primary">Done</button>
    </div>
  </div>
</div>
