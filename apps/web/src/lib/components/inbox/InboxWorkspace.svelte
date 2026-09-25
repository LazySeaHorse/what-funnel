<script lang="ts">
  import { tick } from "svelte";
  import { apiRequest } from "$lib/api";
  import type { InboxState } from "$lib/store.svelte";
  import type { UICapabilities } from "$lib/ui-capabilities";
  import type { LeadEditor } from "$lib/leads/lead-editor.svelte";
  import {
    formatTime,
    getContactHandle,
    getContactName,
    parseMessageContent,
  } from "$lib/inbox/presentation";
  import ChannelBadge from "$lib/components/ChannelBadge.svelte";
  import UserAvatar from "$lib/components/UserAvatar.svelte";
  import {
    CheckCircleIcon,
    CheckIcon,
    ChevronLeftIcon,
    PaperAirplaneIcon,
    PaperClipIcon,
    SparklesIcon,
    XMarkIcon,
  } from "@fvilers/heroicons-svelte/24/outline";
  import ConversationDetailsPanel from "./ConversationDetailsPanel.svelte";
  import ConversationList from "./ConversationList.svelte";

  let {
    inbox,
    leadEditor,
    capabilities,
    pipelineStates,
    searchQuery = $bindable(),
    aiEnabled,
    aiProviderConfigured,
    aiAutoReplyEnabled,
    onSimulate,
  }: {
    inbox: InboxState;
    leadEditor: LeadEditor;
    capabilities: UICapabilities;
    pipelineStates: any[];
    searchQuery: string;
    aiEnabled: boolean;
    aiProviderConfigured: boolean;
    aiAutoReplyEnabled: boolean;
    onSimulate: () => void;
  } = $props();

  let messageContainer: HTMLDivElement | null = $state(null);
  let showStatus = $state(false);
  let attachmentInput: HTMLInputElement | null = $state(null);
  let attachment = $state<File | null>(null);
  // Which message ID is currently "active" (expanded to show metadata)
  let activeMessageID = $state<string | null>(null);

  let emptyComposer = {
    text: "",
    aiReplyDraftID: null,
    replyToMessageID: null,
    sending: false,
    error: "",
  };
  let composer = $derived(
    inbox.activeConvoID
      ? (inbox.composers[inbox.activeConvoID] ?? emptyComposer)
      : emptyComposer,
  );
  let draft = $derived(
    inbox.activeConvoID
      ? (inbox.replyDrafts[inbox.activeConvoID] ?? null)
      : null,
  );
  let aiControl = $derived(
    inbox.activeConvo?.ai_control ?? {
      state: "active",
      reply_override: "inherit",
      run_state: "idle",
    },
  );
  let aiReplyEnabled = $derived(
    aiEnabled &&
      aiProviderConfigured &&
      (aiControl.reply_override === "enabled" ||
        (aiControl.reply_override === "inherit" && aiAutoReplyEnabled)) &&
      aiControl.state === "active",
  );
  let providerSupportsReplies = $derived(
    ["whatsapp", "telegram"].includes(
      inbox.activeConvo?.channel?.provider ||
        inbox.activeConvo?.channel?.type ||
        inbox.activeConvo?.channel_type ||
        "",
    ),
  );
  let displayMessages = $derived.by(() =>
    inbox.messages
      .filter((message) => message.content_type !== "reaction")
      .map((message) => ({
        ...message,
        parsedContent: parseMessageContent(message.content),
      })),
  );
  let filteredConversations = $derived(
    inbox.conversations.filter((conversation) => {
      const query = searchQuery.trim().toLowerCase();
      return (
        !query ||
        getContactName(conversation).toLowerCase().includes(query) ||
        JSON.stringify(conversation.last_message || "")
          .toLowerCase()
          .includes(query)
      );
    }),
  );
  let counts = $derived({
    all: inbox.conversations.length,
    unassigned: inbox.conversations.filter(
      (conversation) => !conversation.assigned_user_ids?.length,
    ).length,
    mine: inbox.conversations.filter((conversation) =>
      conversation.assigned_user_ids?.includes(
        inbox.currentUser?.user_id || inbox.currentUser?.id,
      ),
    ).length,
  });

  $effect(() => {
    displayMessages;
    void tick().then(() => {
      if (messageContainer)
        messageContainer.scrollTop = messageContainer.scrollHeight;
    });
  });

  // Reset active message when conversation changes
  $effect(() => {
    inbox.activeConvoID;
    activeMessageID = null;
  });

  // ── Cluster helpers ──────────────────────────────────────────────────────────
  // Two messages are "clustered" when they're from the same sender direction and
  // within 2 minutes of each other.
  function isSameCluster(a: any, b: any): boolean {
    if (!a || !b) return false;
    const aOut = a.direction === "outbound" || a.sender_type === "human" || a.sender_type === "ai";
    const bOut = b.direction === "outbound" || b.sender_type === "human" || b.sender_type === "ai";
    if (aOut !== bOut) return false;
    // Also treat ai vs human differently (different sender within same direction)
    if (a.sender_type !== b.sender_type) return false;
    const diff = Math.abs(
      new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
    );
    return diff < 2 * 60 * 1000; // 2 minutes
  }

  function getBubbleRadius(
    isOutbound: boolean,
    prevClustered: boolean,
    nextClustered: boolean,
  ): string {
    // Base: fully round
    // Outbound tucks right corners, inbound tucks left corners
    if (isOutbound) {
      if (!prevClustered && !nextClustered) return "rounded-2xl"; // solo
      if (!prevClustered && nextClustered) return "rounded-2xl rounded-br-[6px]"; // top of cluster
      if (prevClustered && nextClustered) return "rounded-2xl rounded-r-[6px]"; // middle
      return "rounded-2xl rounded-tr-[6px]"; // tail
    } else {
      if (!prevClustered && !nextClustered) return "rounded-2xl"; // solo
      if (!prevClustered && nextClustered) return "rounded-2xl rounded-bl-[6px]"; // top of cluster
      if (prevClustered && nextClustered) return "rounded-2xl rounded-l-[6px]"; // middle
      return "rounded-2xl rounded-tl-[6px]"; // tail
    }
  }

  function getSpacingClass(prevClustered: boolean): string {
    return prevClustered ? "mt-0.5" : "mt-4";
  }

  // ── Cascade stage label ──────────────────────────────────────────────────────
  function stageBadge(stageMatched?: string): string {
    if (!stageMatched) return "";
    if (stageMatched.includes("pattern")) return "L1 Fast-Path";
    if (stageMatched.includes("embed")) return "L2 Semantic";
    if (stageMatched.includes("llm") || stageMatched.includes("grounded")) return "L3 RAG";
    return "L4";
  }

  async function selectConversation(id: string) {
    await inbox.selectConversation(id);
  }
  async function changeFilter(filter: "all" | "unassigned" | "mine") {
    inbox.filter = filter;
    await inbox.loadConversations();
    if (
      !inbox.conversations.some(
        (conversation) => conversation.id === inbox.activeConvoID,
      ) &&
      inbox.conversations[0]
    )
      await selectConversation(inbox.conversations[0].id);
  }
  async function changeStateFilter(state: string) {
    inbox.stateFilter = state;
    await inbox.loadConversations();
    if (!inbox.conversations.length) inbox.clearConversationSelection();
  }
  async function sendMessage() {
    const id = inbox.activeConvoID;
    if (!id || (!composer.text.trim() && !attachment) || composer.sending)
      return;
    let media:
      | { id: string; contentType: "image" | "video" | "audio" | "document" }
      | undefined;
    if (attachment) {
      if (attachment.size > 20 * 1024 * 1024) {
        composer.error = "Attachments must be 20 MiB or smaller.";
        return;
      }
      const form = new FormData();
      form.append("file", attachment);
      try {
        const uploaded = await apiRequest(`/conversations/${id}/media`, {
          method: "POST",
          body: form,
        });
        const contentType = attachment.type.startsWith("image/")
          ? "image"
          : attachment.type.startsWith("video/")
            ? "video"
            : attachment.type.startsWith("audio/")
              ? "audio"
              : "document";
        media = { id: uploaded.id, contentType };
      } catch (reason: any) {
        composer.error = reason?.message || "Failed to upload attachment.";
        return;
      }
    }
    const sent = await inbox.sendMessage(
      id,
      composer.text.trim(),
      composer.aiReplyDraftID ?? undefined,
      media,
    );
    if (sent) attachment = null;
  }
  function handleComposerKeydown(event: KeyboardEvent) {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      void sendMessage();
    }
  }
  function setComposerText(value: string) {
    composer.text = value;
  }
  function toggleChatAI() {
    if (!inbox.activeConvo) return;
    void inbox.updateConversationAIControl(
      inbox.activeConvo.id,
      aiReplyEnabled ? "" : aiControl.state === "active" ? "" : "resume",
      aiReplyEnabled ? "disabled" : "enabled",
    );
  }
  function takeOverFromAI() {
    if (!inbox.activeConvo) return;
    void inbox.updateConversationAIControl(inbox.activeConvo.id, "pause", "");
  }
  function useDraft() {
    if (!draft) return;
    composer.text = draft.draft_text;
    composer.aiReplyDraftID = draft.id;
  }
  async function dismissDraft() {
    if (inbox.activeConvoID && draft)
      await inbox.dismissReplyDraft(inbox.activeConvoID, draft.id);
  }
  function toggleMessageActive(id: string) {
    activeMessageID = activeMessageID === id ? null : id;
  }
</script>

<div class="flex-1 flex overflow-hidden min-h-0 h-full">
  <ConversationList
    {inbox}
    {capabilities}
    {pipelineStates}
    conversations={filteredConversations}
    bind:searchQuery
    {counts}
    onChangeFilter={(value) => void changeFilter(value)}
    onChangeStateFilter={(value) => void changeStateFilter(value)}
    onSelect={(id) => void selectConversation(id)}
  />
  <div
    class="{inbox.activeConvo
      ? 'flex'
      : 'hidden lg:flex'} flex-1 flex-col bg-slate-50 border-r border-slate-100 min-h-0 overflow-hidden w-full"
  >
    {#if !inbox.activeConvo}
      <!-- Empty state -->
      {#if inbox.pendingConvoID}
        <!-- Skeleton while conversation loads -->
        <div class="flex-1 p-6 space-y-4 overflow-hidden">
          <div class="flex items-start gap-2 max-w-xs animate-pulse">
            <div class="w-8 h-8 rounded-full bg-slate-200 shrink-0"></div>
            <div class="space-y-1.5 flex-1">
              <div class="h-3 bg-slate-200 rounded-full w-3/4"></div>
              <div class="h-3 bg-slate-200 rounded-full w-1/2"></div>
            </div>
          </div>
          <div class="flex items-start gap-2 max-w-xs ml-auto flex-row-reverse animate-pulse">
            <div class="space-y-1.5 flex-1">
              <div class="h-3 bg-slate-200 rounded-full w-3/4 ml-auto"></div>
              <div class="h-3 bg-slate-200 rounded-full w-1/2 ml-auto"></div>
            </div>
          </div>
          <div class="flex items-start gap-2 max-w-sm animate-pulse">
            <div class="w-8 h-8 rounded-full bg-slate-200 shrink-0"></div>
            <div class="space-y-1.5 flex-1">
              <div class="h-3 bg-slate-200 rounded-full w-5/6"></div>
              <div class="h-3 bg-slate-200 rounded-full w-2/3"></div>
              <div class="h-3 bg-slate-200 rounded-full w-1/3"></div>
            </div>
          </div>
        </div>
      {:else}
        <div
          class="flex-1 flex flex-col items-center justify-center p-8 text-center"
        >
          <h3 class="text-sm font-medium text-slate-800">
            Select a conversation
          </h3>
          <p class="text-xs text-slate-400">
            Select a conversation from the list to view messages and send
            replies.
          </p>
        </div>
      {/if}
    {:else}
      <!-- Chat header -->
      <div
        class="h-16 px-4 sm:px-6 bg-white border-b border-slate-100 flex items-center justify-between shrink-0"
      >
        <div class="flex items-center gap-3">
          <button
            onclick={() => inbox.clearConversationSelection()}
            class="lg:hidden"
            aria-label="Back to conversations"
            ><ChevronLeftIcon class="w-5 h-5" /></button
          ><UserAvatar
            name={getContactName(inbox.activeConvo)}
            avatar={inbox.activeConvo.contact?.avatar_url}
            size="md"
            channel={inbox.activeConvo.channel?.type ||
              inbox.activeConvo.channel_type}
          />
          <div>
            <div class="flex items-center gap-2">
              <h2 class="text-sm font-medium text-slate-800">
                {getContactName(inbox.activeConvo)}
              </h2>
              {#if inbox.activeConvo.channel}<ChannelBadge
                  channel={inbox.activeConvo.channel?.type ||
                    inbox.activeConvo.channel_type}
                  size="xs"
                />{/if}
            </div>
            <p class="text-[11px] text-slate-400">
              {getContactHandle(inbox.activeConvo)}
            </p>
          </div>
        </div>
        <div class="flex items-center gap-3">
          <!-- AI replies toggle -->
          <label
            class="flex items-center gap-2 cursor-pointer select-none {!aiProviderConfigured ? 'opacity-40 pointer-events-none' : ''}"
            title={!aiProviderConfigured ? 'Configure an AI provider in Settings first' : 'Toggle AI auto-replies for this conversation'}
          >
            <span class="text-[11px] font-medium text-slate-600">AI replies</span>
            <button
              type="button"
              role="switch"
              aria-label="AI replies for this chat"
              aria-checked={aiReplyEnabled}
              onclick={toggleChatAI}
              disabled={!aiProviderConfigured}
              class="relative w-9 h-5 rounded-full transition-colors duration-200 focus-visible:outline focus-visible:outline-2 focus-visible:outline-blue-400 {aiReplyEnabled ? 'bg-blue-600' : 'bg-slate-200'}"
            >
              <span class="sr-only">{aiReplyEnabled ? 'AI replies on' : 'AI replies off'}</span>
              <span
                class="absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white shadow-xs transition-transform duration-200 {aiReplyEnabled ? 'translate-x-4' : 'translate-x-0'}"
              ></span>
            </button>
          </label>

          <!-- Conversation status -->
          <div class="relative">
            <button
              onclick={() => (showStatus = !showStatus)}
              title="Conversation Status"
              class="p-2 rounded-lg hover:bg-slate-100 transition-colors"
              ><CheckCircleIcon class="w-4 h-4 text-slate-500" /></button
            >{#if showStatus}<div
                class="absolute right-0 top-full w-44 bg-white rounded-xl border border-slate-200 shadow-lg z-50 py-1"
              >
                {#if inbox.activeConvo.status === "open"}<button
                    onclick={() => {
                      inbox.closeConversation();
                      showStatus = false;
                    }}
                    class="w-full px-3 py-2 text-left text-xs text-slate-600 hover:bg-slate-50 hover:text-slate-800"
                    >Close conversation</button
                  >{:else}<div class="px-3 py-2 text-xs text-slate-400">
                    Conversation is closed
                  </div>{/if}
              </div>{/if}
          </div>
        </div>
      </div>

      <!-- Message list -->
      <div
        bind:this={messageContainer}
        class="flex-1 px-4 sm:px-6 py-4 overflow-y-auto"
      >
        <div class="flex justify-center mb-4">
          <span class="text-[11px] text-slate-400">Messages</span>
        </div>
        {#if !displayMessages.length}
          <div class="text-center py-12 text-slate-400 text-xs">
            No messages recorded in this conversation. Send a reply below.
          </div>
        {:else}
          {#each displayMessages as message, idx (message.id)}
            {@const customer =
              message.sender_type === "contact" ||
              message.sender_type === "customer" ||
              message.direction === "inbound"}
            {@const isAI = message.sender_type === "ai"}
            {@const isOutbound = !customer}
            {@const text =
              message.parsedContent.text ||
              message.parsedContent.caption ||
              (message.parsedContent.media_id
                ? ""
                : JSON.stringify(message.parsedContent))}
            {@const prevMsg = idx > 0 ? displayMessages[idx - 1] : null}
            {@const nextMsg =
              idx < displayMessages.length - 1
                ? displayMessages[idx + 1]
                : null}
            {@const prevClustered = isSameCluster(prevMsg, message)}
            {@const nextClustered = isSameCluster(message, nextMsg)}
            {@const isLast = idx === displayMessages.length - 1}
            {@const showMeta = isLast || activeMessageID === message.id}
            {@const isOptimistic = message._optimistic === true}
            {@const radiusClass = getBubbleRadius(isOutbound, prevClustered, nextClustered)}
            {@const spacingClass = getSpacingClass(prevClustered)}
            <div
              class="message-row {isOutbound ? 'outbound' : ''} flex flex-col {isOutbound ? 'items-end ml-auto' : 'items-start'} max-w-md {spacingClass}"
              role="button"
              tabindex="0"
              onclick={() => toggleMessageActive(message.id)}
              onkeydown={(e) => e.key === "Enter" && toggleMessageActive(message.id)}
            >
              <div
                class="msg-text px-3.5 py-2.5 text-xs whitespace-pre-wrap {radiusClass} {isAI
                  ? 'bg-violet-600 text-white'
                  : isOutbound
                    ? 'bg-blue-600 text-white'
                    : 'bg-white border border-slate-200/70 text-slate-800'} {isOptimistic ? 'opacity-70' : ''}"
              >
                {#if message.parsedContent.media_id}
                  {@const mediaURL = `/api-gateway/media/${message.parsedContent.media_id}`}
                  {#if message.content_type === "image"}
                    <img
                      src={mediaURL}
                      alt={text || "Shared image"}
                      class="mb-2 max-h-72 rounded-xl object-contain"
                      loading="lazy"
                    />
                  {:else if message.content_type === "video"}
                    <video
                      src={mediaURL}
                      controls
                      preload="metadata"
                      class="mb-2 max-h-72 rounded-xl"
                      ><track kind="captions" /></video
                    >
                  {:else if message.content_type === "audio"}
                    <audio src={mediaURL} controls preload="metadata"
                      ><track kind="captions" /></audio
                    >
                  {:else}
                    <a
                      href={mediaURL}
                      target="_blank"
                      rel="noreferrer"
                      class="mb-2 block underline">Open attachment</a
                    >
                  {/if}
                {/if}
                {#if text}{text}{/if}
              </div>

              {#if message.reactions?.length}
                <div
                  class="mt-1 flex flex-wrap gap-1"
                  aria-label="Message reactions"
                >
                  {#each message.reactions as reaction (`${reaction.sender_external_id}:${reaction.emoji}`)}
                    <span
                      class="rounded-full border border-slate-200 bg-white px-2 py-0.5 text-xs shadow-2xs"
                      title={reaction.sender_external_id}>{reaction.emoji}</span
                    >
                  {/each}
                </div>
              {/if}

              <!-- Metadata row: only show for last or tapped message -->
              {#if showMeta}
                <div class="flex items-center gap-1.5 mt-1">
                  {#if isAI}
                    <span class="text-[10px] text-violet-400 font-medium"
                      >AI</span
                    >
                  {/if}
                  <span class="text-[10px] text-slate-400"
                    >{formatTime(message.created_at)}</span
                  >
                  {#if isOutbound}
                    <CheckIcon class="w-3 h-3 text-blue-400" />
                    <span class="text-[10px] text-slate-400"
                      >{isOptimistic ? "sending" : (message.delivery_status || "sent")}</span
                    >
                  {/if}
                  {#if providerSupportsReplies && !isOutbound}
                    <button
                      type="button"
                      title="Reply to message"
                      onclick={(e) => {
                        e.stopPropagation();
                        composer.replyToMessageID = message.id;
                      }}
                      class="text-[10px] text-slate-400 hover:text-blue-600 transition-colors"
                      >Reply</button
                    >
                  {/if}
                </div>
              {/if}
            </div>
          {/each}
        {/if}
      </div>

      <!-- Composer -->
      <div class="px-3 sm:px-4 py-3 bg-white border-t border-slate-100 shrink-0">
        <!-- AI draft suggestion banner -->
        {#if capabilities.useReplyDrafts && draft && composer.aiReplyDraftID !== draft.id}
          <div class="mb-3 p-3 rounded-xl bg-violet-50/70 border border-violet-100">
            <div class="flex justify-between items-start">
              <div class="flex items-center gap-1.5">
                <SparklesIcon class="w-3.5 h-3.5 text-violet-600 shrink-0" />
                <span class="text-xs font-medium text-violet-700">AI reply suggestion</span>
                {#if draft.stage_matched}
                  <span class="text-[10px] text-violet-400 font-mono">{stageBadge(draft.stage_matched)}</span>
                {/if}
              </div>
              <button onclick={dismissDraft} aria-label="Dismiss suggestion" class="text-violet-400 hover:text-violet-600 transition-colors">
                <XMarkIcon class="w-3.5 h-3.5" />
              </button>
            </div>
            <div class="text-xs text-slate-700 mt-1.5">{draft.draft_text}</div>
            <div class="flex justify-end mt-2">
              <button
                onclick={useDraft}
                class="px-3 py-1 bg-violet-600 text-white text-xs rounded-lg hover:bg-violet-700 transition-colors"
                >Use this</button
              >
            </div>
          </div>
        {/if}

        <!-- Error banner -->
        {#if composer.error || (inbox.activeConvoID && inbox.mutationErrors[inbox.activeConvoID])}
          <p role="alert" class="wf-alert-error mb-2">
            {composer.error || inbox.mutationErrors[inbox.activeConvoID!]}
          </p>
        {/if}

        <!-- Composer box -->
        <div class="bg-white rounded-2xl border border-slate-200/90 overflow-hidden">
          {#if aiReplyEnabled}
            <!-- AI is controlling — show take-over pill -->
            <div class="px-4 py-3 flex items-center justify-between">
              <button
                type="button"
                onclick={takeOverFromAI}
                class="flex items-center gap-2 px-3 py-2 rounded-xl bg-violet-50 border border-violet-200/80 hover:bg-violet-100 transition-colors group cursor-pointer"
                title="Pause AI"
                aria-label="Pause AI"
              >
                <SparklesIcon class="w-3.5 h-3.5 text-violet-500 shrink-0" />
                <span class="text-xs ai-shimmer-text font-medium">
                  {aiControl.run_state === "replying"
                    ? "AI is replying... Tap to take over"
                    : "AI is handling this chat. Tap to take over"}
                </span>
                <span class="sr-only">Pause AI</span>
              </button>
            </div>
          {:else}
            <!-- Standard composer -->
            {#if composer.replyToMessageID}
              {@const replyTarget = displayMessages.find(
                (m) => m.id === composer.replyToMessageID,
              )}
              <div
                class="mx-3 mt-2.5 flex items-center justify-between rounded-lg border-l-2 border-blue-500 bg-blue-50 px-2.5 py-2 text-[11px] text-slate-600"
              >
                <span class="truncate"
                  >Replying to {replyTarget?.parsedContent.text ||
                    replyTarget?.parsedContent.caption ||
                    "message"}</span
                >
                <button
                  type="button"
                  aria-label="Cancel reply"
                  onclick={() => {
                    composer.replyToMessageID = null;
                  }}
                  ><XMarkIcon class="h-3.5 w-3.5" /></button
                >
              </div>
            {/if}

            {#if attachment}
              <div
                class="mx-3 mt-2.5 flex items-center justify-between rounded-lg bg-slate-100 px-2.5 py-2 text-[11px] text-slate-600"
              >
                <span class="truncate"
                  >{attachment.name} · {(
                    attachment.size /
                    1024 /
                    1024
                  ).toFixed(1)} MiB</span
                >
                <button
                  aria-label="Remove attachment"
                  onclick={() => {
                    attachment = null;
                    if (attachmentInput) attachmentInput.value = "";
                  }}><XMarkIcon class="h-3.5 w-3.5" /></button
                >
              </div>
            {/if}

            <textarea
              value={composer.text}
              onkeydown={handleComposerKeydown}
              placeholder="Enter a message..."
              rows="1"
              class="compose-input w-full px-4 pt-2.5 pb-0.5 text-xs sm:text-sm bg-transparent focus:outline-none resize-none leading-relaxed"
              style="max-height: 160px; overflow-y: auto;"
              oninput={(e) => {
                const el = e.currentTarget;
                el.style.height = 'auto';
                el.style.height = Math.min(el.scrollHeight, 160) + 'px';
                setComposerText(el.value);
              }}
            ></textarea>

            <div
              class="flex items-center justify-between px-3 py-2 border-t border-slate-100"
            >
              <div class="flex items-center">
                <input
                  bind:this={attachmentInput}
                  type="file"
                  class="hidden"
                  accept="image/*,video/*,audio/*,.pdf,.doc,.docx,.txt"
                  onchange={(event) => {
                    attachment = event.currentTarget.files?.[0] || null;
                  }}
                />
                <button
                  type="button"
                  title="Add attachment"
                  aria-label="Add attachment"
                  onclick={() => attachmentInput?.click()}
                  class="attachment-btn w-8 h-8 rounded-full border border-slate-300 hover:border-slate-400 hover:bg-slate-50 text-slate-500 hover:text-slate-700 flex items-center justify-center transition cursor-pointer"
                >
                  <PaperClipIcon class="w-4 h-4" />
                </button>
              </div>
              <button
                onclick={sendMessage}
                disabled={(!composer.text.trim() && !attachment) ||
                  composer.sending}
                class="send-btn w-8 h-8 bg-blue-600 text-white rounded-full flex items-center justify-center disabled:opacity-40 hover:bg-blue-700 active:scale-95 transition-all"
                aria-label="Send message"
                ><PaperAirplaneIcon class="w-4 h-4" /></button
              >
            </div>
          {/if}
        </div>
      </div>
    {/if}
  </div>
  {#if capabilities.showConversationSidePanel}<ConversationDetailsPanel
      {inbox}
      editor={leadEditor}
      {capabilities}
      {pipelineStates}
      {onSimulate}
    />{/if}
</div>
