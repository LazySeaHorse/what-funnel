<script lang="ts">
  import { onMount } from "svelte";
  import { apiRequest } from "$lib/api";
  import type { WorkspaceState } from "$lib/workspace.svelte";

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

  <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
    <button onclick={() => openDialog("whatsapp")} class="rounded-xl border border-slate-200 bg-white p-3 text-left hover:border-blue-300">
      <div class="text-xs font-medium text-slate-700">WhatsApp</div>
      <div class="mt-1 text-[10px] font-medium uppercase tracking-wider text-blue-600">Connect WhatsApp</div>
    </button>
    <button onclick={() => openDialog("telegram")} class="rounded-xl border border-slate-200 bg-white p-3 text-left hover:border-blue-300">
      <div class="text-xs font-medium text-slate-700">Telegram Bot API</div>
      <div class="mt-1 text-[10px] font-medium uppercase tracking-wider text-blue-600">Connect Telegram</div>
    </button>
    {#each ["Instagram DMs", "Facebook DMs"] as future}
      <div class="rounded-xl border border-dashed border-slate-200 bg-slate-50 p-3">
        <div class="text-xs font-medium text-slate-600">{future}</div>
        <div class="mt-1 text-[10px] font-medium uppercase tracking-wider text-slate-400">Coming soon</div>
      </div>
    {/each}
  </div>

  <div class="rounded-xl border border-sky-100 bg-sky-50 p-3 text-xs leading-5 text-sky-800">
    Telegram bots cannot start a conversation. A customer must message the bot first in Telegram before you can reply from WhatFunnel.
  </div>

  {#if loading}
    <div role="status" class="py-6 text-xs text-slate-500">Loading connections…</div>
  {:else}
    <div class="space-y-3">
      {#each connections as connection (connection.channel_id)}
        {@const healthy = connection.state === "connected"}
        <div class="flex items-center justify-between gap-4 rounded-xl border border-slate-200 bg-white p-4 shadow-2xs">
          <div class="flex min-w-0 items-center gap-3">
            <div class={`flex h-10 w-10 shrink-0 items-center justify-center rounded-xl text-sm font-bold ${connection.provider === "telegram" ? "bg-sky-50 text-sky-700" : "bg-emerald-50 text-emerald-700"}`}>{connection.provider === "telegram" ? "T" : "W"}</div>
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
