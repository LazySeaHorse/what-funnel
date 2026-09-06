<script lang="ts">
  import BrandLogo from "$lib/components/BrandLogo.svelte";
  import type { InboxState } from "$lib/store.svelte";
  import type { UICapabilities } from "$lib/ui-capabilities";
  import {
    InboxIcon,
    UsersIcon,
    BoltIcon,
    BookOpenIcon,
    UserIcon,
    DevicePhoneMobileIcon,
    Cog6ToothIcon,
    ChevronDownIcon,
  } from "@fvilers/heroicons-svelte/24/outline";
  import type { DashboardSection } from "./types";

  let {
    inbox,
    capabilities,
    selected,
    accountName,
    onSelect,
    onLogout,
  }: {
    inbox: InboxState;
    capabilities: UICapabilities;
    selected: DashboardSection;
    accountName: string;
    onSelect: (section: DashboardSection) => void;
    onLogout: () => void;
  } = $props();
  let showWorkspaceMenu = $state(false);
  const items = [
    {
      key: "inbox" as const,
      label: "Inbox",
      icon: InboxIcon,
      visible: () => true,
    },
    {
      key: "leads" as const,
      label: "Leads",
      icon: UsersIcon,
      visible: () => capabilities.leadTracking,
    },
    {
      key: "automation" as const,
      label: "Automations",
      icon: BoltIcon,
      visible: () => capabilities.manageAutomation,
    },
    {
      key: "knowledge" as const,
      label: "Knowledge",
      icon: BookOpenIcon,
      visible: () => capabilities.manageKnowledge,
    },
    {
      key: "contacts" as const,
      label: "Contacts",
      icon: UserIcon,
      visible: () => capabilities.viewContacts,
    },
    {
      key: "simulate" as const,
      label: "Simulate",
      icon: DevicePhoneMobileIcon,
      visible: () => capabilities.useSimulator,
    },
    {
      key: "settings" as const,
      label: "Settings",
      icon: Cog6ToothIcon,
      visible: () => true,
    },
  ];
</script>

<aside
  class="hidden lg:flex relative w-56 flex-col justify-between p-4 bg-transparent shrink-0 overflow-hidden"
>
  <div
    class="absolute bottom-0 left-0 right-0 w-full pointer-events-none select-none z-0 overflow-hidden"
  >
    <img
      src="/images/dashboard-sidebar-hero.webp"
      alt=""
      class="w-full h-auto object-cover object-bottom"
      style="mask-image: linear-gradient(to bottom, transparent 0%, black 18%, black 78%, transparent 100%); -webkit-mask-image: linear-gradient(to bottom, transparent 0%, black 18%, black 78%, transparent 100%);"
    />
  </div>
  <div class="relative z-10">
    <button
      type="button"
      class="px-2 pt-1 pb-6"
      onclick={() => onSelect("inbox")}
      aria-label="Go to inbox"><BrandLogo size="sm" /></button
    >
    <nav class="space-y-1">
      {#each items as item (item.key)}
        {#if item.visible()}
          <button
            onclick={() => onSelect(item.key)}
            aria-label={item.key === "simulate" ? "Simulate DEV" : undefined}
            class="w-full flex items-center {item.key === 'simulate'
              ? 'justify-between'
              : ''} gap-3 px-3.5 py-2.5 rounded-xl font-medium text-sm transition-all duration-150 {selected ===
            item.key
              ? item.key === 'simulate'
                ? 'bg-purple-50/80 text-purple-600'
                : 'bg-blue-50/80 text-blue-600'
              : 'text-slate-500 hover:text-slate-900 hover:bg-slate-100/60'}"
          >
            <div class="flex items-center gap-3">
              <item.icon
                class="w-5 h-5 {selected === item.key
                  ? item.key === 'simulate'
                    ? 'text-purple-600'
                    : 'text-blue-600'
                  : 'text-slate-400'}"
              /><span
                >{item.key === "settings" && !capabilities.isManager
                  ? "Preferences"
                  : item.label}</span
              >
            </div>
            {#if item.key === "simulate"}<span
                class="px-1.5 py-0.5 rounded text-[10px] font-medium {selected ===
                'simulate'
                  ? 'bg-purple-600 text-white'
                  : 'bg-purple-100 text-purple-700'}">DEV</span
              >{/if}
          </button>
        {/if}
      {/each}
    </nav>
  </div>
  <div class="relative z-10">
    <button
      type="button"
      class="flex w-full items-center justify-between rounded-xl border border-slate-200 bg-white/90 p-2.5 text-left transition hover:bg-slate-50"
      onclick={() => (showWorkspaceMenu = !showWorkspaceMenu)}
      aria-expanded={showWorkspaceMenu}
      aria-label="Toggle workspace menu"
      ><div class="flex items-center gap-3">
        <div
          class="w-8 h-8 rounded-xl bg-blue-600 text-white font-medium flex items-center justify-center text-sm"
        >
          {accountName.charAt(0).toUpperCase()}
        </div>
        <div>
          <div
            class="text-xs font-medium text-slate-800 leading-tight truncate max-w-[100px]"
          >
            {accountName}
          </div>
          <div class="text-[11px] text-slate-400 leading-tight capitalize">
            {inbox.currentUser?.role || "Loading…"}
          </div>
        </div>
      </div>
      <ChevronDownIcon class="w-4 h-4 text-slate-400 mr-1" /></button
    >
    {#if showWorkspaceMenu}<div
        class="absolute bottom-full left-0 right-0 mb-2 bg-white rounded-xl border border-slate-200 py-1.5 z-50"
      >
        <div class="px-3 py-1 text-[11px] font-medium text-slate-400 uppercase">
          Current account
        </div>
        <div class="px-3 py-1.5 text-xs font-medium text-slate-800">
          {accountName}
        </div>
        <div class="border-t border-slate-100 my-1"></div>
        <button
          onclick={onLogout}
          class="w-full text-left px-3 py-1.5 text-xs font-medium hover:bg-rose-50 text-rose-600"
          >Sign out</button
        >
      </div>{/if}
  </div>
</aside>
