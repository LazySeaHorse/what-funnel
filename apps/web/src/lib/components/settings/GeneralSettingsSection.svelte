<script lang="ts">
  import { ChevronDownIcon } from "@fvilers/heroicons-svelte/24/outline";
  import { formatTimeZoneLabel, supportedTimeZones } from "./timezones";
  import type { WorkspaceSettingsForm } from "./types";

  let {
    form = $bindable(),
    saving,
    onSave,
    onChangeProductMode,
  }: {
    form: WorkspaceSettingsForm;
    saving: boolean;
    onSave: () => void;
    onChangeProductMode: (mode: string) => void;
  } = $props();
</script>

<div class="space-y-6">
  <h2 class="text-base font-medium text-slate-900">General</h2>
  <div class="space-y-5 text-xs">
    <div class="space-y-1.5">
      <label for="inputWorkspaceName" class="block font-medium text-slate-700"
        >Workspace name</label
      ><input
        id="inputWorkspaceName"
        type="text"
        bind:value={form.workspaceName}
        class="wf-input"
        placeholder="Enter workspace name"
      />
    </div>
    <div class="space-y-1.5">
      <label for="selectTimeZone" class="block font-medium text-slate-700"
        >Default time zone</label
      >
      <div class="relative">
        <select
          id="selectTimeZone"
          bind:value={form.defaultTimeZone}
          class="wf-select"
          >{#if !supportedTimeZones.includes(form.defaultTimeZone)}<option
              value={form.defaultTimeZone}
              >{formatTimeZoneLabel(form.defaultTimeZone)}</option
            >{/if}{#each supportedTimeZones as zone}<option value={zone}
              >{formatTimeZoneLabel(zone)}</option
            >{/each}</select
        >
        <div
          class="absolute inset-y-0 right-0 flex items-center pr-3 pointer-events-none text-slate-400"
        >
          <ChevronDownIcon class="w-4 h-4" />
        </div>
      </div>
    </div>
    <div class="space-y-1.5">
      <label for="selectLanguage" class="block font-medium text-slate-700"
        >Language</label
      >
      <div class="relative">
        <select id="selectLanguage" bind:value={form.language} class="wf-select"
          ><option value="English">English</option><option value="Spanish"
            >Spanish (Español)</option
          ><option value="French">French (Français)</option><option
            value="German">German (Deutsch)</option
          ><option value="Italian">Italian (Italiano)</option><option
            value="Portuguese">Portuguese (Português)</option
          ><option value="Japanese">Japanese (日本語)</option><option
            value="Chinese">Chinese (Simplified)</option
          ><option value="Arabic">Arabic (العربية)</option></select
        >
        <div
          class="absolute inset-y-0 right-0 flex items-center pr-3 pointer-events-none text-slate-400"
        >
          <ChevronDownIcon class="w-4 h-4" />
        </div>
      </div>
    </div>
    <div class="space-y-1.5">
      <label for="selectDateFormat" class="block font-medium text-slate-700"
        >Date format</label
      >
      <div class="relative">
        <select
          id="selectDateFormat"
          bind:value={form.dateFormat}
          class="wf-select"
          ><option value="DD MMM YYYY">DD MMM YYYY (e.g. 20 May 2024)</option
          ><option value="MM/DD/YYYY">MM/DD/YYYY (e.g. 05/20/2024)</option
          ><option value="DD/MM/YYYY">DD/MM/YYYY (e.g. 20/05/2024)</option
          ><option value="YYYY-MM-DD">YYYY-MM-DD (e.g. 2024-05-20)</option
          ></select
        >
        <div
          class="absolute inset-y-0 right-0 flex items-center pr-3 pointer-events-none text-slate-400"
        >
          <ChevronDownIcon class="w-4 h-4" />
        </div>
      </div>
    </div>
    <div class="space-y-2 pt-1">
      <span class="block font-medium text-slate-700">Time format</span>
      <div class="flex items-center gap-6">
        <label class="flex items-center gap-2 cursor-pointer select-none"
          ><input
            type="radio"
            name="timeFormat"
            value="12"
            bind:group={form.timeFormat}
            class="w-4 h-4 text-blue-600 border-slate-300 focus:ring-blue-500 accent-blue-600"
          /><span class="text-xs text-slate-700 font-medium"
            >12 hour (1:30 PM)</span
          ></label
        >
        <label class="flex items-center gap-2 cursor-pointer select-none"
          ><input
            type="radio"
            name="timeFormat"
            value="24"
            bind:group={form.timeFormat}
            class="w-4 h-4 text-blue-600 border-slate-300 focus:ring-blue-500 accent-blue-600"
          /><span class="text-xs text-slate-700 font-medium"
            >24 hour (13:30)</span
          ></label
        >
      </div>
    </div>
    <div class="space-y-2 border-t border-slate-100 pt-5">
      <span class="block font-medium text-slate-700">Workspace type</span>
      <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
        <label
          class="flex cursor-pointer items-center gap-2 rounded-xl border p-3 text-xs {form.productMode ===
          'full_workspace'
            ? 'border-blue-500 bg-blue-50/50'
            : 'border-slate-200'}"
          ><input
            type="radio"
            name="workspace-mode"
            checked={form.productMode === "full_workspace"}
            onchange={() => onChangeProductMode("full_workspace")}
          /><span
            ><span class="block font-medium text-slate-800">Full workspace</span
            >Inbox and lead tracking</span
          ></label
        >
        <label
          class="flex cursor-pointer items-center gap-2 rounded-xl border p-3 text-xs {form.productMode ===
          'chatbot_only'
            ? 'border-blue-500 bg-blue-50/50'
            : 'border-slate-200'}"
          ><input
            type="radio"
            name="workspace-mode"
            checked={form.productMode === "chatbot_only"}
            onchange={() => onChangeProductMode("chatbot_only")}
          /><span
            ><span class="block font-medium text-slate-800">Chatbot only</span
            >Automated replies only</span
          ></label
        >
      </div>
    </div>
    <div class="pt-6 flex justify-end">
      <button
        onclick={onSave}
        disabled={saving}
        class="wf-button-primary px-5 py-2.5"
        >{saving ? "Saving..." : "Save changes"}</button
      >
    </div>
  </div>
</div>
