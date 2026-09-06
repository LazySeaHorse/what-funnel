<script lang="ts">
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
    <div class="space-y-1.5">
      <label for="inputCategory" class="block font-medium text-slate-700"
        >Business category</label
      ><input
        id="inputCategory"
        type="text"
        bind:value={form.businessCategory}
        class="wf-input"
      />
    </div>
    <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
      <div class="space-y-1.5">
        <label for="inputPhone" class="block font-medium text-slate-700"
          >Phone number</label
        ><input
          id="inputPhone"
          type="text"
          bind:value={form.businessPhone}
          class="wf-input"
        />
      </div>
      <div class="space-y-1.5">
        <label for="inputEmail" class="block font-medium text-slate-700"
          >Public email</label
        ><input
          id="inputEmail"
          type="email"
          bind:value={form.businessEmail}
          class="wf-input"
        />
      </div>
    </div>
    <div class="space-y-1.5">
      <label for="inputAddress" class="block font-medium text-slate-700"
        >Address / Location</label
      ><input
        id="inputAddress"
        type="text"
        bind:value={form.businessAddress}
        class="wf-input"
      />
    </div>
    <div class="space-y-1.5">
      <label for="inputHours" class="block font-medium text-slate-700"
        >Operating hours</label
      ><input
        id="inputHours"
        type="text"
        bind:value={form.businessHours}
        class="wf-input"
      />
    </div>
    <div class="pt-4 flex justify-end">
      <button
        onclick={onSave}
        disabled={saving}
        class="wf-button-primary px-5 py-2.5">Save profile</button
      >
    </div>
  </div>
</div>
