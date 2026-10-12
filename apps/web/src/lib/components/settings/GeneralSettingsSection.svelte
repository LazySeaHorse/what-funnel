<script lang="ts">
  import { ChevronDownIcon } from "@fvilers/heroicons-svelte/24/outline";
  import WorkspaceTypeSelector from "$lib/components/WorkspaceTypeSelector.svelte";
  import { Input, Button } from "$lib/components/ui";
  import { formatTimeZoneLabel, supportedTimeZones } from "$lib/timezones";
  import type { AIMode } from "$lib/ai-reply-mode";
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

  const aiModeOptions: { value: AIMode; title: string; description: string }[] = [
    {
      value: "off",
      title: "No AI replies",
      description: "Your team replies manually. The AI stays out of conversations.",
    },
    {
      value: "suggest",
      title: "Suggestions only",
      description: "The AI drafts replies for your team to review before sending.",
    },
    {
      value: "auto",
      title: "Full AI",
      description: "The AI sends eligible replies automatically.",
    },
  ];
</script>

<div class="space-y-6">
  <h2 class="text-base font-medium text-slate-900">General</h2>
  <div class="space-y-5 text-xs">
    <Input
      id="inputWorkspaceName"
      label="Workspace name"
      bind:value={form.workspaceName}
      placeholder="Enter workspace name"
    />
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
    <fieldset class="space-y-2 border-t border-slate-100 pt-5">
      <legend class="block font-medium text-slate-700">AI replies</legend>
      <div class="grid gap-2">
        {#each aiModeOptions as option (option.value)}
          <label
            class="flex cursor-pointer items-start gap-3 rounded-xl border p-3 {form.aiMode ===
            option.value
              ? 'border-blue-500 bg-blue-50/40'
              : 'border-slate-200 bg-white hover:bg-slate-50'}"
          >
            <input
              type="radio"
              name="aiMode"
              value={option.value}
              bind:group={form.aiMode}
              class="mt-0.5 w-4 h-4 border-slate-300 focus:ring-blue-500 accent-blue-600"
            />
            <span>
              <span class="block font-medium text-slate-800">{option.title}</span>
              <span class="block text-slate-500">{option.description}</span>
            </span>
          </label>
        {/each}
      </div>
      <p class="text-slate-400">
        New conversations inherit this setting. Members can override it in
        their personal preferences, and you can change it per conversation in
        the inbox.
      </p>
    </fieldset>
    <div class="space-y-1.5 border-t border-slate-100 pt-5">
      <label for="textAiGreeting" class="block font-medium text-slate-700"
        >AI greeting message</label
      >
      <textarea
        id="textAiGreeting"
        bind:value={form.aiGreetingText}
        rows="2"
        maxlength="500"
        placeholder="Hi! Thanks for reaching out. How can we help you today?"
        class="w-full bg-white border border-slate-200 rounded-xl p-3 text-xs text-slate-700 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 leading-relaxed transition shadow-2xs"
      ></textarea>
      <p class="text-slate-400">
        Sent when a customer's first message is only a greeting such as "hi" or
        "anyone there?". Leave empty to use the default shown above. In
        draft-only mode it is suggested instead of sent.
      </p>
    </div>
    <div class="space-y-1.5">
      <label class="flex items-start gap-2 cursor-pointer select-none"
        ><input
          id="checkAiRagAutoSend"
          type="checkbox"
          bind:checked={form.aiRagAutoSend}
          class="mt-0.5 w-4 h-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500 accent-blue-600"
        /><span class="text-slate-700"
          ><span class="font-medium">Send AI-written answers automatically</span>
          <span class="block text-slate-400"
            >By default, answers the AI composes from your knowledge base are
            only suggested as drafts. Approved FAQ replies and the greeting are
            sent according to your auto-reply mode either way.</span
          ></span
        ></label
      >
    </div>
    <div class="space-y-2 border-t border-slate-100 pt-5">
      <span class="block font-medium text-slate-700">Workspace type</span>
      <WorkspaceTypeSelector
        value={form.productMode}
        name="workspace-mode"
        disabled={saving}
        onchange={onChangeProductMode}
      />
    </div>
    <div class="pt-6 flex justify-end">
      <Button
        variant="primary"
        onclick={onSave}
        disabled={saving}
        busy={saving}
        class="px-5 py-2.5"
      >Save changes</Button>
    </div>
  </div>
</div>
