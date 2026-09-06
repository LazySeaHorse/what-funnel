<script lang="ts">
  import type { InboxState } from "$lib/store.svelte";
  import type { UICapabilities } from "$lib/ui-capabilities";
  import {
    formatTime,
    getChannelLabel,
    getContactHandle,
    getContactName,
    getSnippet,
  } from "$lib/inbox/presentation";
  import {
    ChevronDownIcon,
    FunnelIcon,
  } from "@fvilers/heroicons-svelte/24/outline";
  import LeadsView from "./LeadsView.svelte";
  import { getLeadStateInfo } from "./presentation";
  import type { LeadEditor } from "$lib/leads/lead-editor.svelte";

  let {
    inbox,
    leadEditor,
    capabilities,
    pipelineStates,
    searchQuery,
    onOpenChat,
  }: {
    inbox: InboxState;
    leadEditor: LeadEditor;
    capabilities: UICapabilities;
    pipelineStates: any[];
    searchQuery: string;
    onOpenChat: (id: string) => void;
  } = $props();
  let filterTab = $state("all");
  let channelFilter = $state("all");
  let assigneeFilter = $state("all");
  let sort = $state<"newest" | "oldest" | "name">("newest");
  let showSort = $state(false);
  let showFilters = $state(false);
  let selectedLeadID = $state<string | null>(null);
  let showDrawer = $state(true);
  let selectedRowIDs = $state<string[]>([]);

  let activeFilterCount = $derived(
    (channelFilter !== "all" ? 1 : 0) + (assigneeFilter !== "all" ? 1 : 0),
  );
  let leads = $derived.by(() =>
    inbox.conversations
      .filter((conversation) => Boolean(conversation.lead))
      .map((conversation) => {
        const channelType =
          conversation.channel?.type || conversation.channel_type || "";
        const channel =
          ["instagram", "whatsapp", "messenger", "telegram", "webchat"].find(
            (value) => channelType.includes(value),
          ) || "unknown";
        const stateKey = conversation.lead.current_state_key || "unknown";
        const state = getLeadStateInfo(stateKey, pipelineStates);
        return {
          id: conversation.id,
          convoId: conversation.id,
          leadId: conversation.lead.id,
          name: getContactName(conversation),
          avatar: conversation.contact?.avatar_url || "",
          avatarBg: "bg-blue-100 text-blue-700",
          handle:
            getContactHandle(conversation) ||
            conversation.contact?.external_identity ||
            "",
          channel,
          stateKey,
          stateLabel: state.label,
          stateColor: state.color,
          assignees: (conversation.assigned_user_ids || []).map(
            (userID: string) => {
              const user = inbox.users.find((item) => item.id === userID);
              return {
                id: userID,
                name: user?.name || user?.email?.split("@")[0] || "User",
                initials: (user?.name || user?.email || "U")
                  .charAt(0)
                  .toUpperCase(),
                avatar: user?.avatar_url || "",
                bg: "bg-blue-600",
              };
            },
          ),
          assigneesExtra: Math.max(
            0,
            (conversation.assigned_user_ids?.length || 0) - 2,
          ),
          lastMessage: getSnippet(conversation) || "No messages yet",
          updatedAt:
            formatTime(
              conversation.last_message_at || conversation.created_at,
            ) || "Unknown",
          tags: conversation.lead.tags || [],
          contactInfo: [
            {
              type: channel === "whatsapp" ? "phone" : "instagram",
              value:
                getContactHandle(conversation) ||
                conversation.contact?.external_identity ||
                "Not provided",
              label: getChannelLabel(channelType),
            },
          ],
          realConvo: conversation,
        };
      }),
  );
  let filteredLeads = $derived.by(() => {
    let result = leads.filter((lead) => {
      if (
        filterTab !== "all" &&
        !(filterTab === "converted"
          ? ["converted", "closed_won"].includes(lead.stateKey)
          : lead.stateKey === filterTab)
      )
        return false;
      if (channelFilter !== "all" && lead.channel !== channelFilter)
        return false;
      if (assigneeFilter === "unassigned" && lead.assignees.length)
        return false;
      if (
        assigneeFilter !== "all" &&
        assigneeFilter !== "unassigned" &&
        !lead.assignees.some((assignee: any) => assignee.id === assigneeFilter)
      )
        return false;
      const query = searchQuery.trim().toLowerCase();
      return (
        !query ||
        [lead.name, lead.lastMessage, lead.handle].some((value) =>
          value.toLowerCase().includes(query),
        )
      );
    });
    if (sort === "name")
      result = [...result].sort((a, b) => a.name.localeCompare(b.name));
    return result;
  });
  let activeLead = $derived(
    (selectedLeadID
      ? leads.find((lead) => lead.id === selectedLeadID)
      : null) ||
      leads[0] ||
      null,
  );
  let counts = $derived({
    all: leads.length,
    new: leads.filter((lead) => lead.stateKey === "new").length,
    contacted: leads.filter((lead) => lead.stateKey === "contacted").length,
    follow_up: leads.filter((lead) => lead.stateKey === "follow_up").length,
    interested: leads.filter((lead) => lead.stateKey === "interested").length,
    converted: leads.filter((lead) =>
      ["converted", "closed_won"].includes(lead.stateKey),
    ).length,
  });

  async function selectLead(lead: any) {
    selectedLeadID = lead.id;
    selectedRowIDs = [lead.id];
    showDrawer = true;
    void leadEditor.open(lead.realConvo);
    if (lead.convoId) void inbox.selectConversation(lead.convoId);
  }
  function toggleRow(id: string, event: MouseEvent) {
    event.stopPropagation();
    selectedRowIDs = selectedRowIDs.includes(id)
      ? selectedRowIDs.filter((item) => item !== id)
      : [...selectedRowIDs, id];
  }
  function toggleAll(event: MouseEvent) {
    event.stopPropagation();
    selectedRowIDs =
      selectedRowIDs.length === filteredLeads.length
        ? []
        : filteredLeads.map((lead) => lead.id);
  }
</script>

<div class="flex-1 flex flex-col min-h-0 h-full overflow-hidden bg-white">
  <div
    class="px-6 pt-5 pb-3 flex items-center justify-between shrink-0 bg-white"
  >
    <h1 class="text-2xl font-medium text-slate-900 tracking-tight">Leads</h1>
    <div class="flex items-center gap-2.5 relative">
      <div class="relative">
        <button
          type="button"
          onclick={() => (showFilters = !showFilters)}
          aria-expanded={showFilters}
          aria-label="Filter leads"
          class="flex items-center gap-2 px-3.5 py-1.5 rounded-xl border text-xs font-medium {activeFilterCount
            ? 'bg-blue-50 border-blue-200/90 text-blue-700'
            : 'bg-white border-slate-200/90 text-slate-700'}"
          ><FunnelIcon class="w-3.5 h-3.5" /><span>Filters</span
          >{#if activeFilterCount}<span
              class="w-4 h-4 rounded-full bg-blue-600 text-white text-[10px] flex items-center justify-center"
              >{activeFilterCount}</span
            >{/if}</button
        >
        {#if showFilters}<div
            class="absolute right-0 top-full mt-1.5 w-64 bg-white rounded-xl border border-slate-200 shadow-xl p-3 z-50 text-xs space-y-3"
          >
            <div>
              <label
                for="leads-channel-filter"
                class="block text-[11px] font-medium text-slate-400 uppercase tracking-wider mb-1.5"
                >Channel</label
              ><select
                id="leads-channel-filter"
                bind:value={channelFilter}
                class="w-full h-8 px-2.5 bg-slate-50 border border-slate-200 rounded-lg"
                ><option value="all">All channels</option><option
                  value="whatsapp">WhatsApp</option
                ><option value="instagram">Instagram</option><option
                  value="messenger">Messenger</option
                ><option value="telegram">Telegram</option><option
                  value="webchat">Webchat</option
                ></select
              >
            </div>
            {#if capabilities.manageAssignments}<div>
                <label
                  for="leads-assignee-filter"
                  class="block text-[11px] font-medium text-slate-400 uppercase tracking-wider mb-1.5"
                  >Assignee</label
                ><select
                  id="leads-assignee-filter"
                  bind:value={assigneeFilter}
                  class="w-full h-8 px-2.5 bg-slate-50 border border-slate-200 rounded-lg"
                  ><option value="all">All Assignees</option><option
                    value="unassigned">Unassigned</option
                  >{#each inbox.users as user}<option value={user.id}
                      >{user.name || user.email || "User"}</option
                    >{/each}</select
                >
              </div>{/if}
            <div class="flex justify-between pt-2 border-t">
              <button
                type="button"
                onclick={() => {
                  channelFilter = "all";
                  assigneeFilter = "all";
                  showFilters = false;
                }}
                disabled={!activeFilterCount}>Reset filters</button
              ><button
                type="button"
                onclick={() => (showFilters = false)}
                class="px-3 py-1 bg-blue-600 text-white rounded-lg"
                >Close</button
              >
            </div>
          </div>{/if}
      </div>
      <div class="relative">
        <button
          onclick={() => (showSort = !showSort)}
          class="flex items-center gap-1.5 px-3.5 py-1.5 bg-white rounded-xl border border-slate-200/90 text-xs font-medium"
          ><span
            >Sort: {sort === "newest"
              ? "Newest"
              : sort === "oldest"
                ? "Oldest"
                : "Name"}</span
          ><ChevronDownIcon class="w-3.5 h-3.5" /></button
        >{#if showSort}<div
            class="absolute right-0 top-full mt-1.5 w-36 bg-white rounded-xl border shadow-lg py-1 z-50 text-xs"
          >
            {#each [["newest", "Newest"], ["oldest", "Oldest"], ["name", "Name (A-Z)"]] as option}<button
                onclick={() => {
                  sort = option[0] as typeof sort;
                  showSort = false;
                }}
                class="w-full px-3 py-1.5 text-left">{option[1]}</button
              >{/each}
          </div>{/if}
      </div>
    </div>
  </div>
  <LeadsView
    leads={filteredLeads}
    {counts}
    activeFilter={filterTab}
    selectedLeadId={selectedLeadID}
    selectedRowIds={selectedRowIDs}
    {showDrawer}
    {activeLead}
    editor={leadEditor}
    {pipelineStates}
    users={inbox.users}
    canManageAssignments={capabilities.manageAssignments}
    onSelectFilter={(key) => (filterTab = key)}
    onSelectLead={selectLead}
    onToggleCheckbox={toggleRow}
    onToggleAllCheckboxes={toggleAll}
    onCloseDrawer={() => {
      showDrawer = false;
      selectedLeadID = null;
      selectedRowIDs = [];
      leadEditor.clear();
    }}
    {onOpenChat}
  />
</div>
