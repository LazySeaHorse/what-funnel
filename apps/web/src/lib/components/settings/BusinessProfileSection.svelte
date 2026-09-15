<script lang="ts">
  import { Input, Button } from "$lib/components/ui";
  import type { WorkspaceSettingsForm } from "./types";
  let {
    form = $bindable(),
    saving,
    onSave,
  }: {
    form: WorkspaceSettingsForm;
    saving: boolean;
    onSave: () => void;
  } = $props();
</script>

<div class="space-y-6">
  <div>
    <h2 class="text-base font-medium text-slate-900">Business profile</h2>
    <p class="text-xs text-slate-500 mt-0.5">
      Details used by AI and customer communications
    </p>
  </div>
  <div class="space-y-4 text-xs">
    <Input
      id="inputCategory"
      label="Business category"
      bind:value={form.businessCategory}
    />
    <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
      <Input
        id="inputPhone"
        label="Phone number"
        bind:value={form.businessPhone}
      />
      <Input
        id="inputEmail"
        type="email"
        label="Public email"
        bind:value={form.businessEmail}
      />
    </div>
    <Input
      id="inputAddress"
      label="Address / Location"
      bind:value={form.businessAddress}
    />
    <Input
      id="inputHours"
      label="Operating hours"
      bind:value={form.businessHours}
    />
    <div class="pt-4 flex justify-end">
      <Button
        variant="primary"
        onclick={onSave}
        disabled={saving}
        busy={saving}
        class="px-5 py-2.5"
      >Save profile</Button>
    </div>
  </div>
</div>
