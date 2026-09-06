<script lang="ts">
  import { onMount } from "svelte";
  import { apiRequest } from "$lib/api";
  import type { InboxState } from "$lib/store.svelte";
  import type { WorkspaceState } from "$lib/workspace.svelte";
  import { decodeWorkspaceSettings } from "$lib/workspace-settings";
  import {
    CheckIcon,
    ExclamationCircleIcon,
  } from "@fvilers/heroicons-svelte/24/outline";
  import AIProviderSettings from "./AIProviderSettings.svelte";
  import BusinessProfileSection from "./BusinessProfileSection.svelte";
  import ChannelsSettings from "./ChannelsSettings.svelte";
  import DeleteWorkspaceDialog from "./DeleteWorkspaceDialog.svelte";
  import GeneralSettingsSection from "./GeneralSettingsSection.svelte";
  import PipelineSettings from "./PipelineSettings.svelte";
  import SettingsInfoPanel from "./SettingsInfoPanel.svelte";
  import SettingsSidebar from "./SettingsSidebar.svelte";
  import UsersPermissionsSection from "./UsersPermissionsSection.svelte";
  import WorkspacePlanDialog from "./WorkspacePlanDialog.svelte";
  import { normalizeSavedTimeZone } from "./timezones";
  import type { SettingsSection, WorkspaceSettingsForm } from "./types";

  let {
    inbox,
    workspace,
    initialSection = "general",
  }: {
    inbox?: InboxState;
    workspace?: WorkspaceState;
    initialSection?: string;
    onNavigate?: (tab: string) => void;
  } = $props();

  let activeSection = $state<SettingsSection>("general");
  let loading = $state(false);
  let saving = $state(false);
  let successMsg = $state("");
  let errorMsg = $state("");
  let showDeleteModal = $state(false);
  let showPlanModal = $state(false);
  let statusTimer: ReturnType<typeof setTimeout> | null = null;
  let form = $state<WorkspaceSettingsForm>({
    workspaceName: "",
    defaultTimeZone: "UTC",
    language: "English",
    dateFormat: "DD MMM YYYY",
    timeFormat: "12",
    businessCategory: "",
    businessPhone: "",
    businessEmail: "",
    businessAddress: "",
    businessWebsite: "",
    businessHours: "",
    productMode: "full_workspace",
    leadTracking: true,
    unassignedVisible: true,
  });

  let canManageTeam = $derived(workspace?.capabilities.manageTeam ?? false);
  const currentPlan = "Pro Plan";
  const storageUsedGB = 4.2;
  const storageTotalGB = 20;
  const storagePercent = Math.round((storageUsedGB / storageTotalGB) * 100);

  $effect(() => {
    if (
      (!canManageTeam && activeSection === "users_permissions") ||
      (form.productMode !== "full_workspace" && activeSection === "pipeline")
    )
      activeSection = "general";
  });

  onMount(() => {
    if (isSettingsSection(initialSection)) activeSection = initialSection;
    void loadSettings();
    return () => {
      if (statusTimer) clearTimeout(statusTimer);
    };
  });

  async function loadSettings() {
    try {
      loading = !(workspace?.settingsReady ?? false);
      if (workspace) {
        await workspace.loadSettings(inbox?.currentUser);
        applyWorkspaceData(workspace.account);
      } else {
        applyWorkspaceData(await apiRequest("/workspace/account"));
      }
    } catch (error) {
      setStatus(
        "error",
        error instanceof Error
          ? error.message
          : "Failed to load workspace settings.",
      );
    } finally {
      loading = false;
    }
  }

  function applyWorkspaceData(account: any) {
    if (!account) return;
    form.workspaceName = account.name || form.workspaceName;
    form.productMode = account.product_mode || "full_workspace";
    const settings = decodeWorkspaceSettings(account.settings);
    if (settings.timezone)
      form.defaultTimeZone = normalizeSavedTimeZone(settings.timezone);
    if (settings.language) form.language = settings.language;
    if (settings.date_format) form.dateFormat = settings.date_format;
    if (settings.time_format) form.timeFormat = settings.time_format;
    form.businessCategory = settings.business_category || "";
    form.businessPhone = settings.business_phone || "";
    form.businessEmail = settings.business_email || "";
    form.businessAddress = settings.business_address || "";
    form.businessWebsite = settings.business_website || "";
    form.businessHours = settings.business_hours || "";
    form.leadTracking = settings.lead_tracking_enabled !== false;
    form.unassignedVisible =
      settings.unassigned_conversations_visible_to_members !== false;
  }

  async function saveSettings() {
    if (!form.workspaceName.trim())
      return setStatus("error", "Workspace name is required.");
    saving = true;
    successMsg = "";
    errorMsg = "";
    try {
      await apiRequest("/workspace/account", {
        method: "PATCH",
        body: { name: form.workspaceName.trim() },
      });
      await apiRequest("/workspace/account/settings", {
        method: "PATCH",
        body: {
          timezone: form.defaultTimeZone,
          language: form.language,
          date_format: form.dateFormat,
          time_format: form.timeFormat,
          business_category: form.businessCategory,
          business_phone: form.businessPhone,
          business_email: form.businessEmail,
          business_address: form.businessAddress,
          business_website: form.businessWebsite,
          business_hours: form.businessHours,
          ...(form.productMode === "full_workspace"
            ? {
                lead_tracking_enabled: form.leadTracking,
                unassigned_conversations_visible_to_members:
                  form.unassignedVisible,
              }
            : {}),
        },
      });
      await workspace?.refreshAccount();
      setStatus("success", "Settings saved successfully");
    } catch (error) {
      setStatus(
        "error",
        error instanceof Error ? error.message : "Failed to save settings",
      );
    } finally {
      saving = false;
    }
  }

  async function updateProductMode(mode: string) {
    if (mode === form.productMode) return;
    try {
      await apiRequest("/workspace/account/product-mode", {
        method: "PATCH",
        body: { product_mode: mode },
      });
      form.productMode = mode;
      await workspace?.refreshAccount();
      setStatus("success", "Workspace type updated.");
    } catch (error) {
      setStatus(
        "error",
        error instanceof Error
          ? error.message
          : "Failed to update workspace type.",
      );
    }
  }

  async function deleteWorkspace() {
    saving = true;
    try {
      await apiRequest("/workspace/account", {
        method: "DELETE",
        body: { confirmation: form.workspaceName },
      });
      await apiRequest("/auth/logout", { method: "POST" }).catch(() => null);
      window.location.assign("/signup");
    } catch (error) {
      setStatus(
        "error",
        error instanceof Error ? error.message : "Failed to delete workspace.",
      );
      saving = false;
    }
  }

  function setStatus(kind: "success" | "error", message: string) {
    if (kind === "success") {
      successMsg = message;
      errorMsg = "";
      if (statusTimer) clearTimeout(statusTimer);
      statusTimer = setTimeout(() => (successMsg = ""), 3000);
    } else {
      errorMsg = message;
      successMsg = "";
    }
  }

  function isSettingsSection(value: string): value is SettingsSection {
    return [
      "general",
      "business_profile",
      "ai_provider",
      "users_permissions",
      "channels",
      "pipeline",
    ].includes(value);
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key !== "Escape") return;
    showPlanModal = false;
    showDeleteModal = false;
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<div
  class="flex-1 flex flex-col h-full overflow-y-auto bg-white p-8"
  aria-busy={loading}
>
  <div class="mb-7">
    <h1 class="text-2xl font-medium text-slate-900 tracking-tight font-sans">
      Settings
    </h1>
    <p class="text-xs text-slate-500 mt-1">
      Manage your workspace and preferences.
    </p>
  </div>
  {#if successMsg}<div
      class="mb-5 px-4 py-3 bg-emerald-50 border border-emerald-200 text-emerald-700 text-xs rounded-xl flex items-center justify-between"
    >
      <div class="flex items-center gap-2">
        <CheckIcon class="w-4 h-4" /><span>{successMsg}</span>
      </div>
      <button
        onclick={() => (successMsg = "")}
        aria-label="Dismiss success message">×</button
      >
    </div>{/if}
  {#if errorMsg}<div
      class="mb-5 px-4 py-3 bg-rose-50 border border-rose-200 text-rose-700 text-xs rounded-xl flex items-center justify-between"
    >
      <div class="flex items-center gap-2">
        <ExclamationCircleIcon class="w-4 h-4" /><span>{errorMsg}</span>
      </div>
      <button onclick={() => (errorMsg = "")} aria-label="Dismiss error message"
        >×</button
      >
    </div>{/if}
  {#if loading}
    <div role="status" class="py-10 text-xs text-slate-500">
      Loading workspace settings…
    </div>
  {:else}
    <div class="grid grid-cols-12 gap-8 items-start">
      <SettingsSidebar
        bind:activeSection
        productMode={form.productMode}
        {canManageTeam}
      />
      <div
        class="col-span-12 md:col-span-9 lg:col-span-6"
        role="tabpanel"
        id={`settings-panel-${activeSection}`}
      >
        {#if activeSection === "general"}<GeneralSettingsSection
            bind:form
            {saving}
            onSave={() => void saveSettings()}
            onChangeProductMode={(mode) => void updateProductMode(mode)}
          />
        {:else if activeSection === "business_profile"}<BusinessProfileSection
            bind:form
            {saving}
            onSave={() => void saveSettings()}
          />
        {:else if activeSection === "ai_provider"}<AIProviderSettings />
        {:else if activeSection === "users_permissions"}<UsersPermissionsSection
            {inbox}
            {workspace}
            onStatus={setStatus}
          />
        {:else if activeSection === "channels"}<ChannelsSettings {workspace} />
        {:else if activeSection === "pipeline"}<PipelineSettings
            {workspace}
          />{/if}
      </div>
      <SettingsInfoPanel
        {currentPlan}
        {storageUsedGB}
        {storageTotalGB}
        {storagePercent}
        onManagePlan={() => (showPlanModal = true)}
        onDelete={() => (showDeleteModal = true)}
      />
    </div>
  {/if}
</div>

{#if showPlanModal}<WorkspacePlanDialog
    onClose={() => (showPlanModal = false)}
  />{/if}
{#if showDeleteModal}<DeleteWorkspaceDialog
    workspaceName={form.workspaceName}
    {saving}
    onDelete={() => void deleteWorkspace()}
    onClose={() => (showDeleteModal = false)}
  />{/if}
