<script lang="ts">
  import type { InboxState } from "$lib/store.svelte";
  import type { UICapabilities } from "$lib/ui-capabilities";
  import type { LeadEditor } from "$lib/leads/lead-editor.svelte";
  import LeadStagePicker from "$lib/components/leads/LeadStagePicker.svelte";
  import LeadAssigneePicker from "$lib/components/leads/LeadAssigneePicker.svelte";
  import LeadTagsEditor from "$lib/components/leads/LeadTagsEditor.svelte";
  import LeadNotesEditor from "$lib/components/leads/LeadNotesEditor.svelte";
  import {
    formatTime,
    getChannelLabel,
    getContactName,
  } from "$lib/inbox/presentation";
  import {
    SparklesIcon,
  } from "@fvilers/heroicons-svelte/24/outline";

  let {
    inbox,
    editor,
    capabilities,
    pipelineStates,
    onSimulate,
  }: {
    inbox: InboxState;
    editor: LeadEditor;
    capabilities: UICapabilities;
    pipelineStates: any[];
    onSimulate: () => void;
  } = $props();
  let tab = $state<"lead" | "details" | "activity">("lead");

  $effect(() => {
    const conversation = capabilities.leadTracking ? inbox.activeConvo : null;
    if (conversation?.lead?.id) void editor.open(conversation);
    else editor.clear();
  });
</script>

<aside
  class="lead-panel hidden lg:flex w-[300px] xl:w-[320px] bg-white flex-col shrink-0 overflow-y-auto min-h-0 h-full"
  aria-label="Conversation details"
>
  <div
    class="flex items-center justify-around border-b border-slate-100 text-xs font-medium text-slate-400 pt-3 px-2"
  >
    {#each ["lead", "details", "activity"] as key}<button
        onclick={() => (tab = key as typeof tab)}
        class="pb-2.5 px-4 capitalize {tab === key
          ? 'text-blue-600 border-b-2 border-blue-600'
          : 'hover:text-slate-700'}">{key}</button
      >{/each}
  </div>
  <div class="p-4 space-y-5 flex-1 text-xs">
    {#if !inbox.activeConvo}<div
        class="p-6 text-center text-xs text-slate-400 space-y-2"
      >
        <p>Select a conversation to view details.</p>
        <p>
          To simulate customer inquiries, click <button
            onclick={onSimulate}
            class="text-purple-600 font-medium underline">Simulate</button
          >.
        </p>
      </div>
    {:else if tab === "lead"}
      <LeadStagePicker stateKey={editor.lead?.current_state_key || "new"} states={pipelineStates} onchange={(key) => editor.changeStage(key)} />
      {#if capabilities.manageAssignments}
        <LeadAssigneePicker users={inbox.users} assignedUserIds={editor.conversation?.assigned_user_ids ?? []} onToggle={(id) => editor.toggleAssignee(id)} />
      {/if}
      <LeadTagsEditor tags={editor.lead?.tags ?? []} onadd={(tag) => editor.addTag(tag)} onremove={(tag) => editor.removeTag(tag)} />
      <LeadNotesEditor notes={editor.notes} loading={editor.loading} expanded onadd={(body) => editor.addNote(body)} />
      <div class="space-y-2">
        <div class="flex items-center gap-1.5 text-xs">
          <SparklesIcon class="w-3.5 h-3.5 text-purple-600" />AI assist
          <span class="text-slate-400">(Beta)</span>
        </div>
        <button
          class="w-full py-2 rounded-xl border border-blue-200 text-blue-600 text-xs"
          >Summarize conversation</button
        >
      </div>
    {:else if tab === "details"}<div
        class="p-3.5 bg-slate-50 rounded-xl space-y-2.5 text-xs"
      >
        <div class="flex justify-between">
          <span>Display name</span><b>{getContactName(inbox.activeConvo)}</b>
        </div>
        <div class="flex justify-between">
          <span>Channel</span><b
            >{getChannelLabel(
              inbox.activeConvo.channel_type || inbox.activeConvo.channel?.type,
            )}</b
          >
        </div>
        <div class="flex justify-between">
          <span>Identity</span><b
            >{inbox.activeConvo.contact?.external_identity || "N/A"}</b
          >
        </div>
        <div class="flex justify-between">
          <span>Status</span><b>{inbox.activeConvo.status}</b>
        </div>
      </div>
    {:else if !editor.history.length}<div
        class="text-slate-400 text-xs p-4 text-center"
      >
        No stage history recorded.
      </div>{:else}{#each editor.history as item}<div
          class="text-xs border-l-2 border-blue-200 pl-3"
        >
          <b>Lead stage changed to {item.to_state}</b>
          <div class="text-slate-400">{formatTime(item.created_at)}</div>
        </div>{/each}{/if}
  </div>
</aside>
