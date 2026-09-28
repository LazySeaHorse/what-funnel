<script lang="ts">
  import { onMount } from "svelte";
  import { apiRequest } from "$lib/api";
  import type { WorkspaceState } from "$lib/workspace.svelte";
  import ChannelBadge from "$lib/components/ChannelBadge.svelte";
  import ChannelConnectionModal, { type Connection } from "$lib/components/channels/ChannelConnectionModal.svelte";
  import { ChatBubbleLeftRightIcon } from "@fvilers/heroicons-svelte/24/outline";
  import { Button } from "$lib/components/ui";

  let { workspace }: { workspace?: WorkspaceState } = $props();
  let connections = $state<Connection[]>([]);
  let activeConnection = $state<Connection | null>(null);
  let selectedProvider = $state<Connection["provider"]>("whatsapp");
  let showDialog = $state(false);
  let loading = $state(true);
  let deletingID = $state<string | null>(null);
  let error = $state("");
  let notice = $state("");

  const availableChannels = [
    { id: "whatsapp", name: "WhatsApp", provider: "whatsapp" as const },
    { id: "instagram", name: "Instagram" },
    { id: "messenger", name: "Facebook Messenger" },
    { id: "telegram", name: "Telegram", provider: "telegram" as const },
  ];

  async function refreshConnections() {
    const result = await apiRequest("/channel-connections");
    connections = Array.isArray(result) ? result : [];
  }

  onMount(() => {
    let cancelled = false;
    void (async () => {
      try {
        await refreshConnections();
      } catch (reason: any) {
        if (!cancelled) error = reason?.message || "Failed to load channel connections.";
      } finally {
        if (!cancelled) loading = false;
      }
    })();
    return () => { cancelled = true; };
  });

  function openDialog(provider: Connection["provider"]) {
    selectedProvider = provider;
    activeConnection = null;
    error = "";
    notice = "";
    showDialog = true;
  }

  function closeDialog() {
    showDialog = false;
    activeConnection = null;
    void refreshConnections();
  }

  function handleConnectionSuccess(_connection: Connection) {
    void refreshConnections();
    void workspace?.refreshChannels();
  }

  function handleConnectionChange(_connection: Connection) {
    void refreshConnections();
  }

  async function unlink(connection: Connection) {
    const confirmMessage = (connection.state === "error" || connection.state === "pending")
      ? `Remove ${connection.label}?`
      : `Unlink ${connection.label}? Its WhatFunnel chats and messages will be permanently deleted.`;
    if (!confirm(confirmMessage)) return;
    deletingID = connection.channel_id; error = ""; notice = "";
    try {
      await apiRequest(`/channel-connections/${connection.channel_id}`, { method: "DELETE" });
      await refreshConnections();
      await workspace?.refreshChannels();
      notice = `${connection.label} was unlinked.`;
    } catch (reason: any) {
      error = reason?.message || `Failed to unlink ${connection.provider === "telegram" ? "Telegram" : "WhatsApp"}.`;
    } finally { deletingID = null; }
  }

  function statusLabel(state: Connection["state"]) {
    return ({ pending: "Starting", awaiting_scan: "Scan QR", connecting: "Connecting",
      connected: "Connected", disconnected: "Disconnected", error: "Needs attention" } as Record<string, string>)[state] || state;
  }
</script>

<svelte:window onkeydown={(event) => event.key === "Escape" && closeDialog()} />

<div class="space-y-6" aria-busy={loading}>
  <div class="flex items-center justify-between gap-4">
    <div>
      <h2 class="text-base font-medium text-slate-900">Messaging accounts</h2>
      <p class="mt-1 text-xs text-slate-500">Connect multiple WhatsApp accounts and Telegram bots to one unified inbox.</p>
    </div>
  </div>

  {#if notice}<div role="status" class="rounded-xl border border-emerald-200 bg-emerald-50 p-3 text-xs text-emerald-700">{notice}</div>{/if}
  {#if error}<div role="alert" class="rounded-xl border border-rose-200 bg-rose-50 p-3 text-xs text-rose-700">{error}</div>{/if}

  <div class="space-y-3">
    {#each availableChannels as ch}
      <div class="flex items-center justify-between p-3.5 sm:p-4 bg-white border border-slate-200 rounded-xl hover:border-slate-300 transition">
        <div class="flex items-center gap-3">
          <ChannelBadge channel={ch.id} size="md" showTooltip={false} />
          <span class="text-sm font-medium text-slate-800">{ch.name}</span>
        </div>

        <div>
          {#if ch.provider}
            <button
              type="button"
              aria-label={`Connect ${ch.name}`}
              class="px-3.5 py-1.5 rounded-lg border border-slate-200 hover:border-slate-300 bg-white hover:bg-slate-50 text-slate-700 text-xs font-medium transition cursor-pointer shadow-xs"
              onclick={() => openDialog(ch.provider)}
            >
              Connect
            </button>
          {:else}
            <span class="px-2.5 py-1 text-[11px] font-medium text-slate-400 bg-slate-100 rounded-lg">Coming soon</span>
          {/if}
        </div>
      </div>
    {/each}

    <!-- Web Chat coming soon item -->
    <div class="flex items-center justify-between p-3.5 sm:p-4 bg-slate-50/50 border border-slate-200/60 rounded-xl opacity-75">
      <div class="flex items-center gap-3">
        <div class="w-8 h-8 rounded-xl bg-white border border-slate-200 flex items-center justify-center shrink-0 text-slate-400">
          <ChatBubbleLeftRightIcon class="w-4 h-4" />
        </div>
        <span class="text-sm font-medium text-slate-600">Web Chat</span>
      </div>
      <span class="px-2.5 py-1 text-[11px] font-medium text-slate-400 bg-slate-100 rounded-lg">Not available</span>
    </div>
  </div>

  {#if loading}
    <div role="status" class="py-6 text-xs text-slate-500">Loading connections…</div>
  {:else}
    <div class="space-y-3 pt-2">
      <h3 class="text-sm font-medium text-slate-800">Connected accounts</h3>
      {#each connections as connection (connection.channel_id)}
        {@const healthy = connection.state === "connected"}
        <div class="flex items-center justify-between gap-4 rounded-xl border border-slate-200 bg-white p-4 shadow-2xs">
          <div class="flex min-w-0 items-center gap-3">
            <ChannelBadge channel={connection.provider} size="md" showTooltip={false} />
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <span class="truncate text-sm font-medium text-slate-800">{connection.label}</span>
                <span class={`rounded-md px-2 py-0.5 text-[10px] font-medium ${healthy ? "bg-emerald-50 text-emerald-700" : "bg-amber-50 text-amber-700"}`}>{statusLabel(connection.state)}</span>
              </div>
              <p class="mt-0.5 truncate text-[11px] text-slate-400">{connection.remote_account_id || connection.detail || (connection.provider === "telegram" ? "Telegram bot" : "WhatsApp")}</p>
            </div>
          </div>
          <div class="flex items-center gap-2">
            {#if !healthy}
              <Button
                variant="ghost"
                size="xs"
                class="text-blue-600 hover:text-blue-700"
                onclick={() => { selectedProvider = connection.provider; activeConnection = connection; showDialog = true; }}
              >
                {connection.state === "error" ? "Reconnect" : "Continue"}
              </Button>
            {/if}
            <Button
              variant="ghost"
              size="xs"
              class="text-rose-600 hover:text-rose-700"
              onclick={() => unlink(connection)}
              disabled={deletingID === connection.channel_id}
              busy={deletingID === connection.channel_id}
            >
              {deletingID === connection.channel_id ? "Unlinking…" : "Unlink"}
            </Button>
          </div>
        </div>
      {:else}
        <div class="rounded-xl border border-dashed border-slate-200 p-6 text-center text-xs text-slate-500"><span>No WhatsApp accounts connected yet.</span> <span>No Telegram bots connected yet.</span></div>
      {/each}
    </div>
  {/if}
</div>

{#if showDialog}
  <ChannelConnectionModal
    provider={selectedProvider}
    initialConnection={activeConnection}
    onclose={closeDialog}
    onsuccess={handleConnectionSuccess}
    onchange={handleConnectionChange}
  />
{/if}
