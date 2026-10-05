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
  import { fade } from "svelte/transition";
  import { Button, Tabs } from "$lib/components/ui";
  import {
    EMPTY_SUMMARY_STATE,
    formatSummaryTimestamp,
    isMissingSummaryValue,
  } from "$lib/inbox/summary";

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

  const summaryID = $derived(inbox.activeConvo?.id ?? null);
  const summary = $derived(summaryID ? (inbox.summaries[summaryID] ?? null) : null);
  const summaryState = $derived(
    (summaryID ? inbox.summaryStates[summaryID] : null) ?? EMPTY_SUMMARY_STATE,
  );

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
  <div class="px-2 pt-3">
    <Tabs
      tabs={[
        { key: "lead", label: "Lead" },
        { key: "details", label: "Details" },
        { key: "activity", label: "Activity" }
      ]}
      bind:activeTab={tab}
      class="justify-around"
    />
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
      <div in:fade={{ duration: 120 }} class="space-y-5">
        {#if editor.error}<p class="rounded-md bg-red-50 px-3 py-2 text-xs text-red-700" role="alert" data-testid="lead-editor-error">{editor.error}</p>{/if}
        <LeadStagePicker stateKey={editor.lead?.current_state_key || "new"} states={pipelineStates} onchange={(key) => editor.changeStage(key)} />
        {#if capabilities.manageAssignments}
          <LeadAssigneePicker users={inbox.users} assignedUserIds={editor.conversation?.assigned_user_ids ?? []} onToggle={(id) => editor.toggleAssignee(id)} />
        {/if}
        <LeadTagsEditor tags={editor.lead?.tags ?? []} onadd={(tag) => editor.addTag(tag)} onremove={(tag) => editor.removeTag(tag)} />
        <LeadNotesEditor notes={editor.notes} loading={editor.loading} expanded onadd={(body) => editor.addNote(body)} />
        {#if capabilities.useConversationSummary}
        <div class="space-y-2">
          <div class="flex items-center gap-1.5 text-xs">
            <SparklesIcon class="w-3.5 h-3.5 text-purple-600" />AI assist
            <span class="text-slate-400">(Beta)</span>
          </div>
            <div class="space-y-2" data-testid="conversation-summary">
              {#if summary}
                <div class="rounded-xl bg-slate-50 p-3 space-y-2" aria-busy={summaryState.generating}>
                  <dl class="space-y-2">
                    {#each summary.fields as field (field.key)}
                      <div data-testid="summary-field-{field.key}">
                        <dt class="text-[11px] font-medium text-slate-500">{field.label}</dt>
                        <dd class="text-xs whitespace-pre-line break-words {isMissingSummaryValue(field.value) ? 'text-slate-400' : 'text-slate-800'}">{field.value}</dd>
                      </div>
                    {/each}
                  </dl>
                  <div class="flex items-center justify-between gap-2 text-[11px] text-slate-400">
                    <span data-testid="summary-generated-at">Generated {formatSummaryTimestamp(summary.generated_at)}</span>
                    {#if summary.stale}
                      <span class="rounded-full bg-amber-50 px-2 py-0.5 text-amber-700 border border-amber-200" data-testid="summary-stale">Out of date</span>
                    {/if}
                  </div>
                </div>
              {/if}
              {#if summaryState.error}
                <p class="rounded-md bg-red-50 px-3 py-2 text-xs text-red-700" role="alert" data-testid="summary-error">{summaryState.error}</p>
              {/if}
              <Button
                variant="secondary"
                size="md"
                class="w-full"
                busy={summaryState.generating}
                disabled={summaryState.loading}
                onclick={() => inbox.requestSummary(summaryID)}
              >
                {summaryState.generating ? "Summarizing…" : summary ? "Regenerate summary" : "Summarize conversation"}
              </Button>
            </div>
        </div>
        {/if}
      </div>
    {:else if tab === "details"}<div
        in:fade={{ duration: 120 }}
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
        in:fade={{ duration: 120 }}
        class="text-slate-400 text-xs p-4 text-center"
      >
        No stage history recorded.
      </div>{:else}<div in:fade={{ duration: 120 }} class="space-y-2">{#each editor.history as item}<div
          class="text-xs border-l-2 border-blue-200 pl-3"
        >
          <b>Lead stage changed to {item.to_state}</b>
          <div class="text-slate-400">{formatTime(item.created_at)}</div>
        </div>{/each}</div>{/if}
  </div>
</aside>
