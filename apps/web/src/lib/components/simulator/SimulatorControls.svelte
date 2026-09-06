<script lang="ts">
  import UserAvatar from "$lib/components/UserAvatar.svelte";
  import ChannelBadge from "$lib/components/ChannelBadge.svelte";
  import { SIMULATOR_PLATFORMS } from "./fixtures";
  import type { SimulatorController } from "./SimulatorController.svelte";

  let { controller }: { controller: SimulatorController } = $props();
  let showAddContact = $state(false);
  let newContactName = $state("");

  function addContact() {
    if (!newContactName.trim()) return;
    controller.addContact(newContactName);
    newContactName = "";
    showAddContact = false;
  }
</script>

<div class="lg:col-span-4 flex flex-col gap-4 overflow-y-auto pr-1">
  <div
    class="p-4 bg-white rounded-2xl border border-slate-200/80 shadow-2xs space-y-3 shrink-0"
  >
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-2">
        <span class="w-2 h-2 rounded-full bg-blue-600"></span>
        <span class="text-xs font-semibold text-slate-900 tracking-tight"
          >Simulate Customer</span
        >
      </div>
      <button
        onclick={() => (showAddContact = !showAddContact)}
        class="text-[11px] font-medium text-blue-600 hover:text-blue-700 transition cursor-pointer flex items-center gap-1"
      >
        <span>{showAddContact ? "Cancel" : "+ New Persona"}</span>
      </button>
    </div>

    {#if showAddContact}
      <div
        class="p-3 bg-slate-50 rounded-xl border border-slate-200/80 space-y-2.5"
      >
        <span
          class="text-[10px] font-medium uppercase tracking-wider text-slate-400"
          >Add Test Customer</span
        >
        <input
          type="text"
          bind:value={newContactName}
          placeholder="Customer name (e.g. Eleanor Vance)..."
          class="w-full px-3 py-2 text-xs bg-white rounded-lg border border-slate-200/90 text-slate-900 placeholder-slate-400 focus:outline-none focus:border-blue-500 transition"
        />
        <button
          onclick={addContact}
          disabled={!newContactName.trim()}
          class="w-full py-1.5 bg-blue-600 hover:bg-blue-700 text-white rounded-lg font-medium text-xs transition disabled:opacity-50 cursor-pointer shadow-2xs"
          >Save Customer Persona</button
        >
      </div>
    {/if}

    <div class="grid grid-cols-2 gap-2">
      {#each controller.testContacts as contact (contact.id)}
        {@const isSelected = controller.selectedContactID === contact.id}
        <button
          onclick={() => controller.selectContact(contact.id)}
          class="px-2.5 py-2 rounded-xl text-left border transition flex items-center gap-2.5 cursor-pointer {isSelected
            ? 'bg-blue-50/80 border-blue-300 text-blue-900 shadow-2xs'
            : 'bg-white border-slate-200/80 text-slate-700 hover:bg-slate-50/80'}"
        >
          <UserAvatar name={contact.name} size="sm" />
          <div class="min-w-0 flex-1">
            <div class="font-medium text-xs truncate">{contact.name}</div>
            <div class="text-[10px] text-slate-400 truncate">
              {contact.platform}
            </div>
          </div>
        </button>
      {/each}
    </div>
  </div>

  <div
    class="p-4 bg-white rounded-2xl border border-slate-200/80 shadow-2xs space-y-3 shrink-0"
  >
    <div class="flex items-center justify-between">
      <span class="text-xs font-semibold text-slate-900 tracking-tight"
        >Channel Platform</span
      >
      <span class="text-[10px] text-slate-400 font-mono"
        >{controller.selectedChannelID
          ? `${controller.selectedChannelID.slice(0, 8)}...`
          : "Connecting..."}</span
      >
    </div>
    <div class="grid grid-cols-2 gap-2">
      {#each SIMULATOR_PLATFORMS as platform (platform.key)}
        {@const isSelected = controller.selectedPlatform === platform.key}
        <button
          onclick={() => controller.selectPlatform(platform.key)}
          class="px-3 py-2.5 rounded-xl border font-medium text-xs transition flex items-center gap-2.5 cursor-pointer {isSelected
            ? 'bg-slate-900 text-white border-slate-900 shadow-2xs'
            : 'bg-white border-slate-200/80 text-slate-700 hover:bg-slate-50'}"
        >
          <ChannelBadge channel={platform.key} size="xs" showTooltip={false} />
          <div class="text-left">
            <span class="block text-xs font-medium leading-none"
              >{platform.label}</span
            >
            <span class="text-[9px] opacity-70">matrix_{platform.key}</span>
          </div>
        </button>
      {/each}
    </div>
  </div>

  <div
    class="p-4 bg-white rounded-2xl border border-slate-200/80 shadow-2xs space-y-3.5 flex-1"
  >
    <div class="flex items-center justify-between">
      <span class="text-xs font-semibold text-slate-900 tracking-tight"
        >Test Scenarios & Presets</span
      >
      <span class="text-[10px] text-slate-400">Click to dispatch</span>
    </div>
    <div class="space-y-3">
      {#each controller.presetCategories as category}
        <div class="space-y-1.5">
          <div class="flex items-center justify-between">
            <span class="text-[11px] font-medium text-slate-600"
              >{category.label}</span
            >
            <span
              class="px-1.5 py-0.5 rounded text-[9px] font-medium border {category.badgeColor}"
              >{category.stageTag}</span
            >
          </div>
          <div class="flex flex-col gap-1.5">
            {#each category.prompts as preset}
              <button
                onclick={() => controller.sendMessage(preset)}
                disabled={controller.isSending}
                class="w-full text-left px-3 py-2 bg-slate-50 hover:bg-blue-50/70 hover:text-blue-700 hover:border-blue-200 border border-slate-200/70 rounded-xl text-xs text-slate-700 transition leading-snug disabled:opacity-50 cursor-pointer shadow-2xs active:scale-[0.99]"
                >{preset}</button
              >
            {/each}
          </div>
        </div>
      {/each}
    </div>
  </div>
</div>
