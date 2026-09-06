<script lang="ts">
  import {
    InboxIcon,
    UsersIcon,
    BoltIcon,
    BookOpenIcon,
    Cog6ToothIcon,
  } from "@fvilers/heroicons-svelte/24/outline";
  import type { UICapabilities } from "$lib/ui-capabilities";
  import type { DashboardSection } from "./types";
  let {
    selected,
    capabilities,
    unassignedCount,
    onSelect,
  }: {
    selected: DashboardSection;
    capabilities: UICapabilities;
    unassignedCount: number;
    onSelect: (section: DashboardSection) => void;
  } = $props();
  const items = [
    { key: "inbox" as const, label: "Inbox", icon: InboxIcon },
    { key: "leads" as const, label: "Leads", icon: UsersIcon },
    { key: "automation" as const, label: "Automate", icon: BoltIcon },
    { key: "knowledge" as const, label: "Knowledge", icon: BookOpenIcon },
    { key: "settings" as const, label: "Settings", icon: Cog6ToothIcon },
  ];
  function visible(key: DashboardSection) {
    if (key === "leads") return capabilities.leadTracking;
    if (key === "automation") return capabilities.manageAutomation;
    if (key === "knowledge") return capabilities.manageKnowledge;
    return true;
  }
</script>

<nav
  class="lg:hidden fixed bottom-0 left-0 right-0 z-40 bg-white/95 backdrop-blur-md border-t border-slate-200/90 py-1.5 px-2 flex items-center justify-around"
>
  {#each items as item (item.key)}{#if visible(item.key)}<a
        href={`#${item.key}`}
        role="button"
        onclick={(event) => {
          event.preventDefault();
          onSelect(item.key);
        }}
        class="flex flex-col items-center justify-center py-1 px-3 rounded-xl transition cursor-pointer {selected ===
        item.key
          ? 'text-blue-600 font-medium'
          : 'text-slate-500 hover:text-slate-800'}"
        ><div class="relative">
          <item.icon
            class="w-5 h-5 {selected === item.key
              ? 'text-blue-600'
              : 'text-slate-400'}"
          />{#if item.key === "inbox" && capabilities.leadTracking && unassignedCount > 0}<span
              class="absolute -top-1 -right-1 w-2 h-2 rounded-full bg-blue-600"
            ></span>{/if}
        </div>
        <span class="text-[11px] mt-0.5"
          >{item.key === "settings" && !capabilities.isManager
            ? "Preferences"
            : item.label}</span
        ></a
      >{/if}{/each}
</nav>
