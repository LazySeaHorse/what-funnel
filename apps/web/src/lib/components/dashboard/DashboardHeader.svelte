<script lang="ts">
  import { MagnifyingGlassIcon } from "@fvilers/heroicons-svelte/24/outline";
  import type { InboxState } from "$lib/store.svelte";
  import type { UICapabilities } from "$lib/ui-capabilities";
  import type { DashboardSection } from "./types";
  let {
    inbox,
    capabilities,
    selected,
    searchQuery = $bindable(),
  }: {
    inbox: InboxState;
    capabilities: UICapabilities;
    selected: DashboardSection;
    searchQuery: string;
  } = $props();
  let placeholder = $derived(
    selected === "settings"
      ? "Search settings..."
      : selected === "knowledge"
        ? "Search knowledge..."
        : selected === "leads"
          ? "Search leads..."
          : selected === "automation"
            ? "Search anything..."
            : "Search conversations...",
  );
</script>

<header
  class="hidden lg:flex h-16 px-6 sm:px-8 border-b border-slate-100 items-center justify-between shrink-0"
>
  <div class="flex items-center gap-3">
    <div class="relative w-80 sm:w-96">
      <span
        class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-400"
        ><MagnifyingGlassIcon class="w-4 h-4" /></span
      ><input
        type="text"
        bind:value={searchQuery}
        {placeholder}
        class="w-full h-10 pl-10 pr-10 bg-white text-xs font-medium text-slate-700 placeholder-slate-400 rounded-xl border border-slate-200 focus:outline-none focus:border-blue-400 cursor-text transition"
      /><span
        class="absolute inset-y-0 right-0 pr-3 flex items-center pointer-events-none"
        ><kbd
          class="text-[11px] font-medium text-slate-400 bg-slate-50 px-2 py-0.5 rounded-md border border-slate-200"
          >/</kbd
        ></span
      >
    </div>
    {#if inbox.wsStatus === "disconnected"}<span
        data-testid="offline-indicator"
        class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-rose-50 text-rose-700 border border-rose-200/80 text-xs font-medium shadow-2xs shrink-0 animate-pulse"
        title="Disconnected from real-time server updates"
        ><span class="w-1.5 h-1.5 rounded-full bg-rose-500"></span><span
          >Offline</span
        ></span
      >{/if}
  </div>
  {#if capabilities.showOperatorIdentity}<div
      data-testid="operator-identity"
      class="h-10 flex items-center gap-2.5 px-3.5 bg-white rounded-xl border border-slate-200 text-left"
    >
      <div
        class="w-6 h-6 rounded-lg bg-blue-50 text-blue-600 border border-blue-100/80 flex items-center justify-center text-xs font-medium"
      >
        {(
          inbox.currentUser?.username ||
          inbox.currentUser?.name ||
          inbox.currentUser?.email ||
          "U"
        )
          .charAt(0)
          .toUpperCase()}
      </div>
      <div class="flex flex-col">
        <span class="text-xs font-medium text-slate-800 leading-tight"
          >{inbox.currentUser?.username ||
            inbox.currentUser?.name ||
            inbox.currentUser?.email?.split("@")[0] ||
            "User"}</span
        ><span
          class="text-[10px] text-slate-400 font-medium capitalize leading-tight"
          >{inbox.currentUser?.role || "Agent"}</span
        >
      </div>
    </div>{/if}
</header>
