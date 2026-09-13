<script lang="ts">
  import { onMount } from "svelte";
  import { apiRequest } from "$lib/api";
  import type { WorkspaceState } from "$lib/workspace.svelte";
  import ChannelBadge from "$lib/components/ChannelBadge.svelte";
  import { ChatBubbleLeftRightIcon } from "@fvilers/heroicons-svelte/24/outline";

  interface Capabilities {
    media: boolean; replies: boolean; reactions: boolean;
    edits: boolean; deletes: boolean; receipts: boolean;
  }
  interface Connection {
    channel_id: string;
    provider: "whatsapp" | "telegram";
    label: string;
    state: "pending" | "awaiting_scan" | "connecting" | "connected" | "disconnected" | "error";
    detail?: string;
    remote_account_id?: string;
    capabilities: Capabilities;
  }

  let { workspace }: { workspace?: WorkspaceState } = $props();
  let connections = $state<Connection[]>([]);
  let activeConnection = $state<Connection | null>(null);
  let selectedProvider = $state<Connection["provider"]>("whatsapp");
  let label = $state("");
  let credential = $state("");
  let showDialog = $state(false);
  let loading = $state(true);
  let busy = $state(false);
  let deletingID = $state<string | null>(null);
  let qrRefreshToken = $state(Date.now());
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

  async function refreshActive() {
    if (!activeConnection) return;
    const refreshed = await apiRequest(`/channel-connections/${activeConnection.channel_id}`);
    activeConnection = refreshed;
    connections = connections.map((connection) =>
      connection.channel_id === refreshed.channel_id ? refreshed : connection,
    );
    qrRefreshToken = Date.now();
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

  $effect(() => {
    const connection = activeConnection;
    if (!showDialog || !connection || ["connected", "error", "disconnected"].includes(connection.state)) return;
    const timer = window.setInterval(() => void refreshActive().catch(() => {}), 3000);
    return () => window.clearInterval(timer);
  });

  function openDialog(provider: Connection["provider"]) {
    selectedProvider = provider; activeConnection = null; label = ""; credential = ""; error = ""; notice = ""; showDialog = true;
  }
  function closeDialog() {
    showDialog = false; activeConnection = null; label = ""; credential = "";
  }

  async function startConnection() {
    if (!label.trim() || (selectedProvider === "telegram" && !credential.trim())) return;
    busy = true; error = "";
    try {
      activeConnection = await apiRequest("/channel-connections", {
        method: "POST", body: { provider: selectedProvider, label: label.trim(), credential: credential.trim() },
      });
      credential = "";
      await refreshConnections();
      qrRefreshToken = Date.now();
    } catch (reason: any) {
      credential = "";
      error = reason?.message || `Failed to connect ${selectedProvider === "telegram" ? "Telegram" : "WhatsApp"}.`;
    } finally { busy = false; }
  }

  async function unlink(connection: Connection) {
    if (!confirm(`Unlink ${connection.label}? Its WhatFunnel chats and messages will be permanently deleted.`)) return;
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

  async function retry(connection: Connection) {
    selectedProvider = connection.provider;
    activeConnection = connection;
    busy = true; error = "";
    try {
      activeConnection = await apiRequest(`/channel-connections/${connection.channel_id}/retry`, {
        method: "POST", body: credential.trim() ? { credential: credential.trim() } : {},
      });
      credential = "";
      await refreshConnections();
    } catch (reason: any) {
      credential = "";
      error = reason?.message || "Failed to retry this connection.";
    } finally { busy = false; }
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
              <button onclick={() => { selectedProvider = connection.provider; activeConnection = connection; credential = ""; showDialog = true; void refreshActive(); }} class="wf-button px-3 py-2 text-xs text-blue-600">Continue</button>
            {/if}
            <button onclick={() => unlink(connection)} disabled={deletingID === connection.channel_id} class="wf-button px-3 py-2 text-xs text-rose-600 disabled:opacity-50">
              {deletingID === connection.channel_id ? "Unlinking…" : "Unlink"}
            </button>
          </div>
        </div>
      {:else}
        <div class="rounded-xl border border-dashed border-slate-200 p-6 text-center text-xs text-slate-500"><span>No WhatsApp accounts connected yet.</span> <span>No Telegram bots connected yet.</span></div>
      {/each}
    </div>
  {/if}
</div>

{#if showDialog}
  <div class="fixed inset-0 z-[100] flex items-center justify-center bg-slate-950/40 p-4" role="presentation" onclick={(event) => event.currentTarget === event.target && closeDialog()}>
    <div class="w-full max-w-md rounded-2xl bg-white p-5 shadow-2xl" role="dialog" aria-modal="true" aria-label={`Connect ${selectedProvider === "telegram" ? "Telegram" : "WhatsApp"}`}>
      <div class="flex items-start justify-between gap-4">
        <div>
          <h3 class="text-sm font-medium text-slate-900">Connect {selectedProvider === "telegram" ? "Telegram" : "WhatsApp"}</h3>
          <p class="mt-1 text-xs leading-5 text-slate-500">Each connection is isolated and can use a different {selectedProvider === "telegram" ? "bot" : "WhatsApp account"}.</p>
        </div>
        <button aria-label="Close connection dialog" onclick={closeDialog} class="text-lg text-slate-400 hover:text-slate-600">×</button>
      </div>

      {#if !activeConnection}
        <label class="my-5 block text-xs font-medium text-slate-700">
          Account label
          <input bind:value={label} maxlength="80" autocomplete="off" class="wf-input mt-1.5 w-full" placeholder={selectedProvider === "telegram" ? "e.g. Support bot" : "e.g. Sales WhatsApp"} />
          <span class="mt-1.5 block text-[11px] font-normal text-slate-400">This label only appears inside WhatFunnel.</span>
        </label>
        {#if selectedProvider === "telegram"}
          <label class="mb-5 block text-xs font-medium text-slate-700">
            Bot token
            <input bind:value={credential} type="password" autocomplete="new-password" class="wf-input mt-1.5 w-full" placeholder="Token from @BotFather" />
            <span class="mt-1.5 block text-[11px] font-normal text-slate-400">The token is encrypted at rest and is never shown again.</span>
          </label>
        {/if}
        <div class="flex justify-end gap-2">
          <button onclick={closeDialog} class="wf-button px-3 py-2 text-slate-600">Cancel</button>
          <button onclick={startConnection} disabled={busy || !label.trim() || (selectedProvider === "telegram" && !credential.trim())} class="wf-button-primary px-3 py-2">{busy ? "Starting…" : selectedProvider === "telegram" ? "Connect bot" : "Show QR code"}</button>
        </div>
      {:else if activeConnection.provider === "whatsapp" && activeConnection.state === "awaiting_scan"}
        <div class="my-5 space-y-4">
          <div class="mx-auto h-64 w-64 overflow-hidden rounded-xl border border-slate-200 bg-white p-2">
            <img class="h-full w-full object-contain" src={`/api-gateway/channel-connections/${activeConnection.channel_id}/qr?refresh=${qrRefreshToken}`} alt="WhatsApp pairing QR code" />
          </div>
          <p class="rounded-xl border border-blue-100 bg-blue-50 p-3 text-[11px] leading-5 text-blue-800">On your phone, open WhatsApp → Settings → Linked devices → Link a device, then scan this code.</p>
        </div>
        <div class="flex justify-end gap-2">
          <button onclick={() => void refreshActive()} class="wf-button px-3 py-2 text-slate-600">Refresh</button>
          <button onclick={closeDialog} class="wf-button-primary px-3 py-2">I scanned it</button>
        </div>
      {:else if activeConnection.state === "connected"}
        <div class="my-5 rounded-xl border border-emerald-200 bg-emerald-50 p-4 text-xs leading-5 text-emerald-800">{activeConnection.label} is connected. One-to-one messages will appear in the unified inbox.</div>
        <div class="flex justify-end"><button onclick={closeDialog} class="wf-button-primary px-3 py-2">Done</button></div>
      {:else if activeConnection.state === "error" || activeConnection.state === "disconnected"}
        <div class="my-5 rounded-xl border border-rose-200 bg-rose-50 p-4 text-xs leading-5 text-rose-800">{activeConnection.detail || "WhatsApp disconnected this account. Unlink it and pair again."}</div>
        {#if activeConnection.provider === "telegram"}
          <label class="mb-4 block text-xs font-medium text-slate-700">
            Replacement bot token <span class="font-normal text-slate-400">(optional)</span>
            <input bind:value={credential} type="password" autocomplete="new-password" class="wf-input mt-1.5 w-full" />
          </label>
        {/if}
        <div class="flex justify-end gap-2"><button onclick={closeDialog} class="wf-button px-3 py-2">Close</button><button onclick={() => retry(activeConnection!)} disabled={busy} class="wf-button-primary px-3 py-2">{busy ? "Retrying…" : "Retry"}</button></div>
      {:else}
        <div class="my-5 rounded-xl border border-amber-200 bg-amber-50 p-4 text-xs leading-5 text-amber-800">{activeConnection.detail || "Connecting to WhatsApp…"}</div>
        <div class="flex justify-end"><button onclick={() => void refreshActive()} class="wf-button-primary px-3 py-2">Check status</button></div>
      {/if}
    </div>
  </div>
{/if}
