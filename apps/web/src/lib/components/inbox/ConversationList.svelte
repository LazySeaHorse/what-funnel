<script lang="ts">
  import type { InboxState } from "$lib/store.svelte";
  import type { UICapabilities } from "$lib/ui-capabilities";
  import {
    formatTime,
    getContactName,
    getSnippet,
  } from "$lib/inbox/presentation";
  import { getLeadStateInfo } from "$lib/components/leads/presentation";
  import LeadStateBadge from "$lib/components/LeadStateBadge.svelte";
  import UserAvatar from "$lib/components/UserAvatar.svelte";
  import {
    AdjustmentsHorizontalIcon,
    CheckIcon,
    InboxIcon,
    MagnifyingGlassIcon,
    XMarkIcon,
  } from "@fvilers/heroicons-svelte/24/outline";

  let {
    inbox,
    capabilities,
    pipelineStates,
    conversations,
    searchQuery = $bindable(),
    counts,
    onChangeFilter,
    onChangeStateFilter,
    onSelect,
  }: {
    inbox: InboxState;
    capabilities: UICapabilities;
    pipelineStates: any[];
    conversations: any[];
    searchQuery: string;
    counts: { all: number; unassigned: number; mine: number };
    onChangeFilter: (filter: "all" | "unassigned" | "mine") => void;
    onChangeStateFilter: (state: string) => void;
    onSelect: (id: string) => void;
  } = $props();
  let showFilterMenu = $state(false);
  let availableStates = $derived(
    pipelineStates.length
      ? pipelineStates
      : [
          { key: "new", label: "New Lead" },
          { key: "contacted", label: "Contacted" },
          { key: "follow_up", label: "Follow-up" },
          { key: "interested", label: "Interested" },
          { key: "converted", label: "Converted" },
        ],
  );
  function changeState(state: string) {
    showFilterMenu = false;
    onChangeStateFilter(state);
  }
</script>

<div
  class="{inbox.activeConvo
    ? 'hidden lg:flex'
    : 'flex'} w-full lg:w-[300px] xl:w-[320px] border-r border-slate-100 flex-col shrink-0 bg-white min-h-0 h-full pb-16 lg:pb-0"
>
  <div class="p-4 pb-3 border-b border-slate-100 space-y-3">
    <div class="flex items-center justify-between">
      <h1 class="text-xl lg:text-lg font-medium text-slate-900 tracking-tight">
        Inbox
      </h1>
      {#if capabilities.leadTracking}<div class="relative">
          <button
            type="button"
            title="Filter options"
            aria-label="Filter options"
            aria-expanded={showFilterMenu}
            onclick={() => (showFilterMenu = !showFilterMenu)}
            class="p-1.5 rounded-lg {inbox.stateFilter
              ? 'text-blue-600 bg-blue-50'
              : 'text-slate-400 hover:bg-slate-100'}"
            ><AdjustmentsHorizontalIcon class="w-4 h-4" /></button
          >{#if showFilterMenu}<div
              class="absolute right-0 top-full mt-1.5 w-48 bg-white rounded-xl border border-slate-200 shadow-lg py-1.5 z-50 text-xs"
            >
              <div
                class="px-3 py-1 text-[11px] font-medium text-slate-400 uppercase tracking-wider"
              >
                Filter by stage
              </div>
              <button
                type="button"
                onclick={() => changeState("")}
                class="w-full px-3 py-1.5 text-left hover:bg-slate-50 flex justify-between"
                ><span>All stages</span>{#if !inbox.stateFilter}<CheckIcon
                    class="w-3.5 h-3.5 text-blue-600"
                  />{/if}</button
              >{#each availableStates as state (state.key)}<button
                  type="button"
                  onclick={() => changeState(state.key)}
                  class="w-full px-3 py-1.5 text-left hover:bg-slate-50 flex justify-between"
                  ><span
                    >{state.label ||
                      getLeadStateInfo(state.key, pipelineStates).label}</span
                  >{#if inbox.stateFilter === state.key}<CheckIcon
                      class="w-3.5 h-3.5 text-blue-600"
                    />{/if}</button
                >{/each}{#if inbox.stateFilter}<button
                  type="button"
                  onclick={() => changeState("")}
                  class="w-full px-3 py-1.5 text-left border-t text-slate-500"
                  >Clear stage filter</button
                >{/if}
            </div>{/if}
        </div>{/if}
    </div>
    <div class="relative">
      <span
        class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none text-slate-400"
        ><MagnifyingGlassIcon class="w-4 h-4" /></span
      ><input
        type="text"
        bind:value={searchQuery}
        placeholder="Search conversations"
        class="w-full h-9 pl-9 pr-8 bg-slate-50 text-xs font-medium text-slate-700 placeholder-slate-400 rounded-xl border border-slate-200 focus:outline-none focus:border-blue-400"
      />
    </div>
    {#if capabilities.leadTracking}<div
        class="flex items-center gap-1.5 text-xs overflow-x-auto pb-0.5"
      >
        {#each [["all", "All", counts.all], ["unassigned", "Unassigned", counts.unassigned], ["mine", "Mine", counts.mine]] as tab}<button
            onclick={() =>
              onChangeFilter(tab[0] as "all" | "unassigned" | "mine")}
            class="tab-btn px-2.5 py-1 rounded-full font-medium flex items-center gap-1.5 shrink-0 {inbox.filter ===
            tab[0]
              ? 'bg-blue-50 text-blue-600 border border-blue-200/60 active'
              : 'text-slate-500 hover:bg-slate-100'}"
            ><span>{tab[1]}</span><span class="text-[10px]">{tab[2]}</span
            ></button
          >{/each}
      </div>{/if}
    {#if capabilities.leadTracking && inbox.stateFilter}<div
        class="flex items-center gap-1.5 text-[11px] text-blue-700 bg-blue-50/80 px-2.5 py-1 rounded-lg border border-blue-200/60"
      >
        <span class="text-slate-500">Stage:</span><span class="font-medium"
          >{getLeadStateInfo(inbox.stateFilter, pipelineStates).label}</span
        ><button
          type="button"
          onclick={() => changeState("")}
          class="ml-auto"
          title="Clear stage filter"><XMarkIcon class="w-3 h-3" /></button
        >
      </div>{/if}
  </div>
  <div
    class="conversation-list flex-1 overflow-y-auto divide-y divide-slate-50"
  >
    {#if conversations.length === 0}<div class="p-8 text-center space-y-2">
        <div
          class="w-10 h-10 mx-auto rounded-full bg-blue-50 text-blue-600 flex items-center justify-center"
        >
          <InboxIcon class="w-5 h-5" />
        </div>
        <div class="text-xs font-medium text-slate-800">
          No conversations found
        </div>
        <p class="text-[11px] text-slate-400">
          Incoming messages from connected channels appear here.
        </p>
      </div>
    {:else}{#each conversations as conversation (conversation.id)}{@const selected =
          (inbox.pendingConvoID || inbox.activeConvoID) === conversation.id}
        <div
          role="button"
          tabindex="0"
          onclick={() => onSelect(conversation.id)}
          onkeydown={(event) =>
            event.key === "Enter" && onSelect(conversation.id)}
          class="convo-item relative px-4 py-3.5 flex items-start gap-3 cursor-pointer {selected
            ? 'bg-blue-50/50'
            : 'hover:bg-slate-50/80'}"
        >
          {#if selected}<div
              class="absolute left-0 top-0 bottom-0 w-1 bg-blue-600 rounded-r"
            ></div>{/if}<UserAvatar
            name={getContactName(conversation)}
            avatar={conversation.contact?.avatar_url}
            size="lg"
            channel={conversation.channel?.type || conversation.channel_type}
          />
          <div class="flex-1 min-w-0">
            <div class="flex justify-between mb-0.5">
              <span class="font-medium text-xs text-slate-800 truncate"
                >{getContactName(conversation)}</span
              ><span class="text-[10px] text-slate-400"
                >{formatTime(
                  conversation.last_message_at || conversation.created_at,
                )}</span
              >
            </div>
            <p class="text-xs text-slate-500 truncate">
              {getSnippet(conversation)}
            </p>
            <div class="flex justify-between mt-1.5">
              {#if capabilities.leadTracking && conversation.lead?.current_state_key}<LeadStateBadge
                  stateKey={conversation.lead.current_state_key}
                  size="xs"
                />{/if}{#if conversation.unread}<span
                  class="w-2 h-2 rounded-full bg-blue-600"
                ></span>{/if}
            </div>
          </div>
        </div>{/each}{/if}
  </div>
</div>
