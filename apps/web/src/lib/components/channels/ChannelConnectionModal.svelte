<script lang="ts">
  import { untrack } from "svelte";
  import { apiRequest, ApiError } from "$lib/api";
  import { Modal, Button, Input } from "$lib/components/ui";

  export interface ConnectionCapabilities {
    media: boolean;
    replies: boolean;
    reactions: boolean;
    edits: boolean;
    deletes: boolean;
    receipts: boolean;
  }

  export interface Connection {
    channel_id: string;
    provider: "whatsapp" | "telegram";
    label: string;
    state: "pending" | "awaiting_scan" | "connecting" | "connected" | "disconnected" | "error";
    detail?: string;
    remote_account_id?: string;
    capabilities: ConnectionCapabilities;
  }

  let {
    provider = "whatsapp",
    initialConnection = null,
    onclose,
    onsuccess,
    onchange
  }: {
    provider: "whatsapp" | "telegram";
    initialConnection?: Connection | null;
    onclose: () => void;
    onsuccess?: (connection: Connection) => void;
    onchange?: (connection: Connection) => void;
  } = $props();

  let activeConnection = $state<Connection | null>(null);
  let selectedProvider = $state<Connection["provider"]>("whatsapp");
  let label = $state("");
  let credential = $state("");
  let busy = $state(false);
  let qrRefreshToken = $state(Date.now());
  let error = $state("");

  $effect(() => {
    activeConnection = initialConnection ?? null;
    selectedProvider = initialConnection ? initialConnection.provider : provider;
  });

  // Poll key: only changes when a different connection starts or stops needing
  // polls, so the poll's own updates to activeConnection do not recreate the timer.
  let pollChannelID = $derived(
    activeConnection && !["connected", "error", "disconnected"].includes(activeConnection.state)
      ? activeConnection.channel_id
      : null
  );

  $effect(() => {
    const channelID = pollChannelID;
    if (!channelID) return;
    let inFlight = false;
    const timer = window.setInterval(async () => {
      if (inFlight) return;
      inFlight = true;
      try {
        await untrack(() => refreshActive(channelID));
      } catch {
        // Transient poll failure; the next tick retries.
      } finally {
        inFlight = false;
      }
    }, 3000);
    return () => window.clearInterval(timer);
  });

  async function refreshActive(expectedChannelID?: string) {
    if (!activeConnection) return;
    const channelID = activeConnection.channel_id;
    if (expectedChannelID && expectedChannelID !== channelID) return;
    const refreshed = await apiRequest(`/channel-connections/${channelID}`);
    if (activeConnection?.channel_id !== channelID) return;
    activeConnection = refreshed;
    qrRefreshToken = Date.now();
    onchange?.(refreshed);
    if (refreshed.state === "connected") {
      onsuccess?.(refreshed);
    }
  }

  // Adapter failures come back as a non-2xx response whose body still carries the
  // stored connection (in error state), so the Retry affordance stays available.
  function adoptFailedConnection(reason: unknown) {
    const connection = reason instanceof ApiError ? (reason.data?.connection as Connection | undefined) : undefined;
    if (!connection?.channel_id) return;
    activeConnection = connection;
    onchange?.(connection);
  }

  async function startConnection() {
    if (!label.trim() || (selectedProvider === "telegram" && !credential.trim())) return;
    busy = true;
    error = "";
    try {
      const created = await apiRequest("/channel-connections", {
        method: "POST",
        body: { provider: selectedProvider, label: label.trim(), credential: credential.trim() },
      });
      activeConnection = created;
      credential = "";
      qrRefreshToken = Date.now();
      onchange?.(created);
      if (created.state === "connected") {
        onsuccess?.(created);
      }
    } catch (reason: any) {
      credential = "";
      adoptFailedConnection(reason);
      error = reason?.message || `Failed to connect ${selectedProvider === "telegram" ? "Telegram" : "WhatsApp"}.`;
    } finally {
      busy = false;
    }
  }

  async function retry(connection: Connection) {
    selectedProvider = connection.provider;
    activeConnection = connection;
    busy = true;
    error = "";
    try {
      const retried = await apiRequest(`/channel-connections/${connection.channel_id}/retry`, {
        method: "POST",
        body: credential.trim() ? { credential: credential.trim() } : {},
      });
      activeConnection = retried;
      credential = "";
      qrRefreshToken = Date.now();
      onchange?.(retried);
      if (retried.state === "connected") {
        onsuccess?.(retried);
      }
    } catch (reason: any) {
      credential = "";
      adoptFailedConnection(reason);
      error = reason?.message || "Failed to retry this connection.";
    } finally {
      busy = false;
    }
  }

  function handleClose() {
    if (activeConnection?.state === "connected") {
      onsuccess?.(activeConnection);
    }
    onclose();
  }
</script>

<Modal
  ariaLabel={`Connect ${selectedProvider === "telegram" ? "Telegram" : "WhatsApp"}`}
  closeAriaLabel="Close connection dialog"
  title={`Connect ${selectedProvider === "telegram" ? "Telegram" : "WhatsApp"}`}
  description={`Each connection is isolated and can use a different ${selectedProvider === "telegram" ? "bot" : "WhatsApp account"}.`}
  onclose={handleClose}
>
  {#if error}
    <div role="alert" class="my-3 rounded-xl border border-rose-200 bg-rose-50 p-3 text-xs text-rose-700">
      {error}
    </div>
  {/if}

  {#if !activeConnection}
    <div class="my-5">
      <Input
        id="channelAccountLabel"
        label="Account label"
        bind:value={label}
        maxlength={80}
        autocomplete="off"
        placeholder={selectedProvider === "telegram" ? "e.g. Support bot" : "e.g. Sales WhatsApp"}
        helper="This label only appears inside WhatFunnel."
      />
    </div>
    {#if selectedProvider === "telegram"}
      <div class="mb-5">
        <Input
          id="channelBotToken"
          label="Bot token"
          type="password"
          bind:value={credential}
          autocomplete="new-password"
          placeholder="Token from @BotFather"
          helper="The token is encrypted at rest and is never shown again."
        />
      </div>
    {/if}
    <div class="flex justify-end gap-2">
      <Button variant="ghost" onclick={handleClose}>Cancel</Button>
      <Button
        variant="primary"
        onclick={startConnection}
        disabled={busy || !label.trim() || (selectedProvider === "telegram" && !credential.trim())}
        busy={busy}
      >
        {busy ? "Starting…" : selectedProvider === "telegram" ? "Connect bot" : "Show QR code"}
      </Button>
    </div>
  {:else if activeConnection.provider === "whatsapp" && activeConnection.state === "awaiting_scan"}
    <div class="my-5 space-y-4">
      <div class="mx-auto h-64 w-64 overflow-hidden rounded-xl border border-slate-200 bg-white p-2">
        <img class="h-full w-full object-contain" src={`/api-gateway/channel-connections/${activeConnection.channel_id}/qr?refresh=${qrRefreshToken}`} alt="WhatsApp pairing QR code" />
      </div>
      <p class="rounded-xl border border-blue-100 bg-blue-50 p-3 text-[11px] leading-5 text-blue-800">On your phone, open WhatsApp → Settings → Linked devices → Link a device, then scan this code.</p>
    </div>
    <div class="flex justify-end gap-2">
      <Button variant="secondary" onclick={() => void refreshActive()}>Refresh</Button>
      <Button variant="primary" onclick={handleClose}>I scanned it</Button>
    </div>
  {:else if activeConnection.state === "connected"}
    <div class="my-5 rounded-xl border border-emerald-200 bg-emerald-50 p-4 text-xs leading-5 text-emerald-800">{activeConnection.label} is connected. One-to-one messages will appear in the unified inbox.</div>
    <div class="flex justify-end">
      <Button variant="primary" onclick={handleClose}>Done</Button>
    </div>
  {:else if activeConnection.state === "error" || activeConnection.state === "disconnected"}
    <div class="my-5 rounded-xl border border-rose-200 bg-rose-50 p-4 text-xs leading-5 text-rose-800">
      {activeConnection.detail || (activeConnection.provider === "telegram" ? "Could not connect Telegram bot. Check your bot token and try again." : "WhatsApp disconnected this account. Unlink it and pair again.")}
    </div>
    {#if activeConnection.provider === "telegram"}
      <div class="mb-4">
        <Input
          id="channelReplacementBotToken"
          label="Telegram Bot Token"
          type="password"
          bind:value={credential}
          autocomplete="new-password"
          placeholder="Token from @BotFather"
          helper="Enter the bot token provided by @BotFather."
        />
      </div>
    {/if}
    <div class="flex justify-end gap-2">
      <Button variant="ghost" onclick={handleClose}>Close</Button>
      <Button
        variant="primary"
        onclick={() => retry(activeConnection!)}
        disabled={busy || (activeConnection.provider === "telegram" && !credential.trim())}
        busy={busy}
      >
        {busy ? "Connecting…" : "Reconnect"}
      </Button>
    </div>
  {:else}
    <div class="my-5 rounded-xl border border-amber-200 bg-amber-50 p-4 text-xs leading-5 text-amber-800">{activeConnection.detail || "Connecting to WhatsApp…"}</div>
    <div class="flex justify-end">
      <Button variant="primary" onclick={() => void refreshActive()}>Check status</Button>
    </div>
  {/if}
</Modal>
