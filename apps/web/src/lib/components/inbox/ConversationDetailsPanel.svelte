<script lang="ts">
  import { apiRequest } from "$lib/api";
  import type { InboxState } from "$lib/store.svelte";
  import type { UICapabilities } from "$lib/ui-capabilities";
  import {
    formatTime,
    getChannelLabel,
    getContactName,
  } from "$lib/inbox/presentation";
  import {
    ChevronDownIcon,
    SparklesIcon,
  } from "@fvilers/heroicons-svelte/24/outline";

  let {
    inbox,
    capabilities,
    pipelineStates,
    onSimulate,
  }: {
    inbox: InboxState;
    capabilities: UICapabilities;
    pipelineStates: any[];
    onSimulate: () => void;
  } = $props();
  let tab = $state<"lead" | "details" | "activity">("lead");
  let showStates = $state(false);
  let showAssignees = $state(false);
  let showTagInput = $state(false);
  let newTag = $state("");
  let notes = $state<any[]>([]);
  let history = $state<any[]>([]);
  let loading = $state(false);
  let requestVersion = 0;

  $effect(() => {
    const leadID = capabilities.leadTracking
      ? inbox.activeConvo?.lead?.id
      : null;
    showStates = false;
    showAssignees = false;
    if (leadID) void loadDetails(leadID);
    else {
      notes = [];
      history = [];
    }
  });

  async function loadDetails(leadID: string) {
    const version = ++requestVersion;
    loading = true;
    try {
      const [nextNotes, nextHistory] = await Promise.all([
        apiRequest(`/leads/${leadID}/notes`),
        apiRequest(`/leads/${leadID}/history`),
      ]);
      if (version === requestVersion) {
        notes = nextNotes;
        history = nextHistory;
      }
    } finally {
      if (version === requestVersion) loading = false;
    }
  }

  async function changeState(stateKey: string) {
    const leadID = inbox.activeConvo?.lead?.id;
    if (!leadID) return;
    const lead = await apiRequest(`/leads/${leadID}/state`, {
      method: "PATCH",
      body: { state_key: stateKey },
    });
    if (inbox.activeConvo?.lead?.id === leadID)
      inbox.activeConvo.lead.current_state_key = lead.current_state_key;
    showStates = false;
    await inbox.loadConversations();
  }

  async function addTag() {
    const lead = inbox.activeConvo?.lead;
    const tag = newTag.trim();
    if (!lead || !tag || lead.tags?.includes(tag)) return;
    const updated = await apiRequest(`/leads/${lead.id}/tags`, {
      method: "PATCH",
      body: { tags: [...(lead.tags || []), tag] },
    });
    lead.tags = updated.tags;
    newTag = "";
    showTagInput = false;
  }

  function toggleAssignment(userID: string) {
    const conversation = inbox.activeConvo;
    if (!conversation) return;
    const current = conversation.assigned_user_ids || [];
    inbox.assignConversation(
      conversation.id,
      current.includes(userID)
        ? current.filter((id: string) => id !== userID)
        : [...current, userID],
    );
  }
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
  <div class="p-4 space-y-5 flex-1">
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
      <div class="space-y-1.5 relative">
        <span class="text-xs font-medium text-slate-500">Lead stage</span
        ><button
          type="button"
          onclick={() => (showStates = !showStates)}
          aria-label="Change lead stage"
          class="lead-state-badge w-full flex items-center justify-between p-2.5 bg-amber-50/60 rounded-xl border border-amber-200/80"
          ><span class="text-xs font-medium text-amber-700"
            >{inbox.activeConvo.lead?.current_state_key || "New Lead"}</span
          ><ChevronDownIcon class="w-4 h-4" /></button
        >{#if showStates}<div
            class="absolute top-full left-0 right-0 mt-1 bg-white rounded-xl border z-50"
          >
            {#each pipelineStates as state}<button
                onclick={() => changeState(state.key)}
                aria-label={`Set lead stage to ${state.label}`}
                class="w-full text-left px-3 py-1.5 text-xs hover:bg-slate-50"
                >{state.label}</button
              >{/each}
          </div>{/if}
      </div>
      {#if capabilities.manageAssignments}<div class="space-y-1.5 relative">
          <span class="text-xs font-medium text-slate-500">Assigned to</span>
          <div class="flex items-center gap-2">
            {#if !inbox.activeConvo.assigned_user_ids?.length}<span
                class="text-xs text-slate-400">Unassigned</span
              >{:else}{#each inbox.activeConvo.assigned_user_ids as id}<span
                  class="w-7 h-7 rounded-full bg-blue-600 text-white flex items-center justify-center text-[10px]"
                  >{(inbox.users.find((user) => user.id === id)?.email || "U")
                    .charAt(0)
                    .toUpperCase()}</span
                >{/each}{/if}<button
              type="button"
              onclick={() => (showAssignees = !showAssignees)}
              aria-label="Assign conversation"
              title="Assign conversation"
              class="w-7 h-7 rounded-full border border-dashed">+</button
            >
          </div>
          {#if showAssignees}<div
              class="absolute top-full left-0 w-52 bg-white rounded-xl border z-50 py-1"
            >
              <div
                class="px-3 py-1 text-[11px] font-medium text-slate-400 uppercase"
              >
                Assign team member
              </div>
              {#each inbox.users as user}<button
                  type="button"
                  onclick={() => toggleAssignment(user.id)}
                  class="w-full px-3 py-1.5 text-left text-xs"
                  >{user.name || user.email}</button
                >{/each}
            </div>{/if}
        </div>{/if}
      <div class="space-y-1.5">
        <span class="text-xs font-medium text-slate-500">Tags</span>
        <div class="flex flex-wrap gap-1.5">
          {#if inbox.activeConvo.lead?.tags?.length}{#each inbox.activeConvo.lead.tags as tag}<span
                class="px-2.5 py-1 rounded-lg bg-purple-50 text-purple-600 text-xs"
                >{tag}</span
              >{/each}{:else}<span class="text-xs text-slate-400">No tags</span
            >{/if}{#if showTagInput}<input
              type="text"
              bind:value={newTag}
              aria-label="Tag name"
              onkeydown={(event) => event.key === "Enter" && addTag()}
              class="w-20 px-2 text-xs border rounded"
            /><button onclick={addTag} aria-label="Save tag">✓</button
            >{:else}<button
              onclick={() => (showTagInput = true)}
              title="Add tag"
              class="w-6 h-6 rounded-lg border border-dashed">+</button
            >{/if}
        </div>
      </div>
      <div class="space-y-1.5">
        <span class="text-xs font-medium text-slate-500">Notes</span
        >{#if loading}<div class="p-3 bg-slate-50 text-xs">
            Loading notes…
          </div>{:else if !notes.length}<div
            class="p-3 bg-slate-50 text-xs text-slate-400"
          >
            No notes found. Enter an internal note to add one.
          </div>{:else}{#each notes as note}<div
              class="p-3 rounded-2xl border text-xs"
            >
              {note.body}
              <div class="text-[10px] text-slate-400">
                {formatTime(note.created_at)}
              </div>
            </div>{/each}{/if}
      </div>
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
    {:else if !history.length}<div
        class="text-slate-400 text-xs p-4 text-center"
      >
        No stage history recorded.
      </div>{:else}{#each history as item}<div
          class="text-xs border-l-2 border-blue-200 pl-3"
        >
          <b>Lead stage changed to {item.to_state}</b>
          <div class="text-slate-400">{formatTime(item.created_at)}</div>
        </div>{/each}{/if}
  </div>
</aside>
