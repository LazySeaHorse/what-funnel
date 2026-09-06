<script lang="ts">
  import { onMount } from "svelte";
  import { goto } from "$app/navigation";
  import { apiRequest } from "$lib/api";
  import { InboxState } from "$lib/store.svelte";
  import { WorkspaceState } from "$lib/workspace.svelte";
  import { decodeWorkspaceSettings } from "$lib/workspace-settings";
  import AutomationView from "$lib/components/automation/AutomationView.svelte";
  import DashboardHeader from "$lib/components/dashboard/DashboardHeader.svelte";
  import DashboardSidebar from "$lib/components/dashboard/DashboardSidebar.svelte";
  import MobileDashboardNav from "$lib/components/dashboard/MobileDashboardNav.svelte";
  import type { DashboardSection } from "$lib/components/dashboard/types";
  import InboxWorkspace from "$lib/components/inbox/InboxWorkspace.svelte";
  import ContactsView from "$lib/components/inbox/ContactsView.svelte";
  import SimulatorView from "$lib/components/inbox/SimulatorView.svelte";
  import KnowledgeView from "$lib/components/knowledge/KnowledgeView.svelte";
  import LeadsDashboard from "$lib/components/leads/LeadsDashboard.svelte";
  import PersonalPreferences from "$lib/components/settings/PersonalPreferences.svelte";
  import SettingsView from "$lib/components/settings/SettingsView.svelte";

  const inbox = new InboxState();
  const workspace = new WorkspaceState();
  let selectedSection = $state<DashboardSection>("inbox");
  let searchQuery = $state("");
  let accountName = $state("What Funnel Workspace");
  let aiEnabled = $state(false);
  let aiReplyModeDefault = $state<"auto_send" | "draft_only">("draft_only");
  let aiProviderConfigured = $state(false);
  let aiProviderStatusLoaded = $state(false);
  let togglingGlobalAI = $state(false);
  let capabilities = $derived(workspace.capabilities);
  let pipelineStates = $derived(workspace.pipeline?.states || []);
  let aiAutoReplyEnabled = $derived(
    aiEnabled && aiReplyModeDefault === "auto_send",
  );
  let unassignedCount = $derived(
    inbox.conversations.filter(
      (conversation) => !conversation.assigned_user_ids?.length,
    ).length,
  );

  $effect(() => {
    const account = workspace.account;
    if (account) {
      accountName = account.name || "What Funnel Workspace";
      const settings = decodeWorkspaceSettings(account.settings);
      aiEnabled = settings.ai_enabled === true;
      aiReplyModeDefault =
        settings.ai_reply_mode_default === "auto_send"
          ? "auto_send"
          : "draft_only";
    }
    inbox.users = workspace.users;
    inbox.configureCapabilities(capabilities);
    if (!canOpen(selectedSection)) selectedSection = "inbox";
  });

  $effect(() => {
    if (selectedSection === "leads" || selectedSection === "inbox")
      void inbox.loadConversations();
  });

  onMount(() => {
    const handleSimulatedMessage = async () => {
      await inbox.loadConversations();
      if (inbox.activeConvoID) await inbox.loadMessages(true);
    };
    window.addEventListener("dev-message-sent", handleSimulatedMessage);
    void initialize();
    return () => {
      window.removeEventListener("dev-message-sent", handleSimulatedMessage);
      inbox.dispose();
    };
  });

  async function initialize() {
    try {
      await inbox.init();
      if (!inbox.currentUser) return void goto("/login");
      workspace.users = inbox.users;
      await workspace.loadCore(inbox.currentUser);
      inbox.users = workspace.users;
      const requested = new URLSearchParams(window.location.search).get("tab");
      if (requested && isDashboardSection(requested) && canOpen(requested))
        selectedSection = requested;
      if (inbox.conversations[0] && !inbox.activeConvoID)
        await inbox.selectConversation(inbox.conversations[0].id);
      void loadAIProviderStatus();
      if (capabilities.manageWorkspace)
        void workspace.loadSettings(inbox.currentUser).catch(console.error);
    } catch {
      void goto("/login");
    }
  }

  function canOpen(section: DashboardSection) {
    if (section === "leads") return capabilities.leadTracking;
    if (section === "contacts") return capabilities.viewContacts;
    if (section === "automation") return capabilities.manageAutomation;
    if (section === "knowledge") return capabilities.manageKnowledge;
    if (section === "simulate") return capabilities.useSimulator;
    return true;
  }

  function selectSection(section: DashboardSection) {
    if (!canOpen(section)) return;
    selectedSection = section;
    if (section === "inbox" && !inbox.activeConvo)
      inbox.clearConversationSelection();
  }

  async function loadAIProviderStatus() {
    try {
      aiProviderConfigured =
        (await apiRequest("/workspace/account/ai-config/status"))
          ?.configured === true;
    } catch {
      aiProviderConfigured = false;
    } finally {
      aiProviderStatusLoaded = true;
    }
  }

  async function toggleGlobalAutoReply() {
    if (
      !capabilities.manageWorkspace ||
      togglingGlobalAI ||
      (!aiAutoReplyEnabled && !aiProviderConfigured)
    )
      return;
    togglingGlobalAI = true;
    try {
      const mode = aiAutoReplyEnabled ? "draft_only" : "auto_send";
      await apiRequest("/workspace/account/settings", {
        method: "PATCH",
        body: { ai_reply_mode_default: mode },
      });
      aiReplyModeDefault = mode;
      await workspace.refreshAccount();
    } finally {
      togglingGlobalAI = false;
    }
  }

  async function logout() {
    try {
      await apiRequest("/auth/logout", { method: "POST" });
    } finally {
      void goto("/login");
    }
  }

  function isDashboardSection(value: string): value is DashboardSection {
    return [
      "inbox",
      "leads",
      "automation",
      "knowledge",
      "contacts",
      "simulate",
      "settings",
    ].includes(value);
  }
</script>

<svelte:head
  ><title>What Funnel - Omni Channel Lead Management</title></svelte:head
>

<div
  class="flex h-screen w-full bg-slate-50 overflow-hidden text-slate-800 font-sans"
>
  <DashboardSidebar
    {inbox}
    {capabilities}
    selected={selectedSection}
    {accountName}
    onSelect={selectSection}
    onLogout={() => void logout()}
  />
  <main
    class="flex-1 flex flex-col h-full min-h-0 overflow-hidden bg-white lg:border-l border-slate-200/80"
  >
    <DashboardHeader
      {inbox}
      {capabilities}
      selected={selectedSection}
      bind:searchQuery
    />
    {#if selectedSection === "inbox"}
      <InboxWorkspace
        {inbox}
        {capabilities}
        {pipelineStates}
        bind:searchQuery
        {aiEnabled}
        {aiProviderConfigured}
        {aiAutoReplyEnabled}
        onSimulate={() => selectSection("simulate")}
      />
    {:else if selectedSection === "leads"}
      <LeadsDashboard
        {inbox}
        {capabilities}
        {pipelineStates}
        {searchQuery}
        onOpenChat={(id) => {
          void inbox.selectConversation(id);
          selectedSection = "inbox";
        }}
      />
    {:else if selectedSection === "automation"}
      <AutomationView
        autoReplyEnabled={aiAutoReplyEnabled}
        providerConfigured={aiProviderConfigured}
        providerStatusLoaded={aiProviderStatusLoaded}
      />
    {:else if selectedSection === "knowledge"}
      <KnowledgeView
        {searchQuery}
        reviewerID={inbox.currentUser?.user_id || inbox.currentUser?.id || ""}
        autoReplyEnabled={aiAutoReplyEnabled}
        providerConfigured={aiProviderConfigured}
        canManageAI={capabilities.manageWorkspace}
        togglingAI={togglingGlobalAI}
        onToggleAI={() => void toggleGlobalAutoReply()}
      />
    {:else if selectedSection === "contacts"}
      <ContactsView
        conversations={inbox.conversations}
        onOpenConversation={(id) => {
          void inbox.selectConversation(id);
          selectedSection = "inbox";
        }}
      />
    {:else if selectedSection === "simulate"}
      <SimulatorView onBack={() => selectSection("inbox")} />
    {:else if selectedSection === "settings"}
      <div class="flex-1 flex flex-col min-h-0 overflow-y-auto pb-16 lg:pb-0">
        {#if capabilities.manageWorkspace}<SettingsView
            {inbox}
            {workspace}
            initialSection="general"
          />{:else}<PersonalPreferences {inbox} {workspace} />{/if}
      </div>
    {/if}
  </main>
  {#if !inbox.activeConvo || selectedSection !== "inbox"}<MobileDashboardNav
      selected={selectedSection}
      {capabilities}
      {unassignedCount}
      onSelect={selectSection}
    />{/if}
</div>
