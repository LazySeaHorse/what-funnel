<script lang="ts">
  import { tick } from "svelte";
  import ChannelBadge from "$lib/components/ChannelBadge.svelte";
  import {
    ChatBubbleLeftRightIcon,
    SparklesIcon,
    PaperAirplaneIcon,
    CheckIcon,
    ExclamationCircleIcon,
  } from "@fvilers/heroicons-svelte/24/outline";
  import type { SimulatorController } from "./SimulatorController.svelte";

  let { controller }: { controller: SimulatorController } = $props();
  let messageText = $state("");
  let chatScrollContainer: HTMLDivElement | null = $state(null);

  $effect(() => {
    controller.convoMessages;
    void tick().then(() => {
      if (chatScrollContainer)
        chatScrollContainer.scrollTop = chatScrollContainer.scrollHeight;
    });
  });

  async function sendMessage() {
    const text = messageText.trim();
    if (!text || controller.isSending) return;
    messageText = "";
    await controller.sendMessage(text);
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      void sendMessage();
    }
  }

  function parseMessageText(content: any): string {
    if (!content) return "";
    if (typeof content === "object")
      return content.text || content.caption || JSON.stringify(content);
    if (typeof content !== "string") return String(content);
    try {
      const parsed = JSON.parse(content);
      return parsed.text || parsed.caption || content;
    } catch {
      try {
        const decoded = atob(content);
        const parsed = JSON.parse(decoded);
        return parsed.text || parsed.caption || decoded;
      } catch {
        return content;
      }
    }
  }

  function formatTime(value?: string) {
    return value
      ? new Date(value).toLocaleTimeString([], {
          hour: "2-digit",
          minute: "2-digit",
        })
      : "";
  }
</script>

<div
  class="lg:col-span-4 flex flex-col h-full bg-white rounded-2xl border border-slate-200/80 shadow-xs overflow-hidden"
>
  <div
    class="px-4 py-3 bg-white border-b border-slate-100 flex items-center justify-between shrink-0"
  >
    <div class="flex items-center gap-2.5 min-w-0">
      <ChannelBadge
        channel={controller.selectedPlatform}
        size="sm"
        showTooltip={false}
      />
      <div class="min-w-0">
        <span class="text-xs font-semibold text-slate-900 tracking-tight"
          >Customer Phone View</span
        >
        <div class="text-[10px] text-slate-500 truncate">
          {controller.selectedContact?.name || "Customer"} ·
          <span class="capitalize">{controller.selectedPlatform}</span>
        </div>
      </div>
    </div>
    <div
      class="flex items-center gap-1.5 text-[10px] text-emerald-600 font-medium bg-emerald-50 px-2 py-0.5 rounded-full border border-emerald-100"
    >
      <span class="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse"
      ></span><span>Live Realtime Thread</span>
    </div>
  </div>

  <div
    bind:this={chatScrollContainer}
    class="flex-1 p-4 overflow-y-auto space-y-3 bg-slate-50/40"
  >
    {#if controller.isConvoLoading}
      <div
        class="flex flex-col items-center justify-center h-full text-slate-400 text-xs gap-2"
      >
        <div
          class="w-5 h-5 border-2 border-slate-300 border-t-blue-600 rounded-full animate-spin"
        ></div>
        <span>Loading simulated thread...</span>
      </div>
    {:else if controller.convoMessages.length === 0}
      <div
        class="flex flex-col items-center justify-center h-full text-slate-400 text-xs text-center p-6 space-y-2"
      >
        <div
          class="w-8 h-8 rounded-full bg-slate-100 flex items-center justify-center text-slate-400"
        >
          <ChatBubbleLeftRightIcon class="w-4 h-4" />
        </div>
        <div class="font-medium text-slate-600">
          No messages in this thread yet
        </div>
        <p class="text-[11px] leading-relaxed">
          Pick a scenario prompt or type below to simulate an incoming customer
          query.
        </p>
      </div>
    {:else}
      {#each controller.convoMessages as message (message.id)}
        {@const isCustomer =
          message.sender_type === "contact" ||
          message.sender_type === "customer" ||
          message.direction === "inbound"}
        {@const text = parseMessageText(message.content)}
        {#if isCustomer}
          <div class="flex flex-col items-end ml-auto max-w-[85%]">
            <div
              class="sim-bubble px-3.5 py-2.5 rounded-2xl rounded-tr-sm bg-blue-600 text-white text-xs leading-relaxed shadow-2xs"
            >
              {text}
            </div>
            <span class="text-[10px] text-slate-400 mt-1 mr-1 font-mono"
              >{formatTime(message.created_at)}</span
            >
          </div>
        {:else}
          <div class="flex flex-col items-start max-w-[85%]">
            <div
              class="sim-bubble px-3.5 py-2.5 rounded-2xl rounded-tl-sm bg-white text-slate-800 border border-slate-200/90 text-xs leading-relaxed shadow-2xs"
            >
              {#if message.sender_type === "ai"}
                <div
                  class="flex items-center gap-1.5 text-[10px] font-medium text-blue-700 mb-1 pb-1 border-b border-slate-100"
                >
                  <SparklesIcon class="w-3 h-3 text-blue-600" /><span
                    >AI Auto-reply</span
                  >
                  {#if controller.currentTelemetry.stageMatched !== "none"}<span
                      class="px-1.5 py-0.2 rounded bg-blue-50 text-[9px] font-mono border border-blue-100"
                      >{controller.currentTelemetry.stageMatched}</span
                    >{/if}
                </div>
              {/if}
              {text}
            </div>
            <span class="text-[10px] text-slate-400 mt-1 ml-1 font-mono"
              >{formatTime(message.created_at)}</span
            >
          </div>
        {/if}
      {/each}
    {/if}
  </div>

  <div class="p-3.5 bg-white border-t border-slate-100 space-y-2 shrink-0">
    <div class="relative flex items-center">
      <input
        type="text"
        bind:value={messageText}
        onkeydown={handleKeydown}
        placeholder="Send message as customer..."
        class="w-full pl-3.5 pr-10 py-2.5 bg-slate-50 text-xs text-slate-900 placeholder-slate-400 rounded-xl border border-slate-200/90 focus:outline-none focus:bg-white focus:border-blue-500 transition"
      />
      <button
        onclick={sendMessage}
        disabled={controller.isSending || !messageText.trim()}
        class="absolute right-1.5 p-1.5 bg-blue-600 hover:bg-blue-700 text-white rounded-lg disabled:opacity-40 transition flex items-center justify-center cursor-pointer shadow-2xs active:scale-[0.95]"
        title="Send as customer"
        ><PaperAirplaneIcon class="w-3.5 h-3.5" /></button
      >
    </div>
    {#if controller.lastStatus === "success"}
      <div
        class="text-[11px] text-emerald-600 font-medium flex items-center gap-1.5"
      >
        <CheckIcon class="w-3.5 h-3.5" /><span
          >Inbound message sent to What Funnel</span
        >
      </div>
    {:else if controller.lastStatus === "error"}
      <div
        class="text-[11px] text-rose-600 font-medium flex items-center gap-1.5"
      >
        <ExclamationCircleIcon class="w-3.5 h-3.5" /><span
          >{controller.lastError}</span
        >
      </div>
    {/if}
  </div>
</div>
