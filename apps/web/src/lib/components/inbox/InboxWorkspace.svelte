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
    BoltIcon,
    BookmarkIcon,
    CheckCircleIcon,
    CheckIcon,
    ChevronLeftIcon,
    FaceSmileIcon,
    PaperAirplaneIcon,
    PlusIcon,
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
  let replyTab = $state<"reply" | "note">("reply");
  let internalNote = $state("");
  let messageContainer: HTMLDivElement | null = $state(null);
  let showStatus = $state(false);
  let attachmentInput: HTMLInputElement | null = $state(null);
  let attachment = $state<File | null>(null);
  let emptyComposer = {
    text: "",
    aiReplyDraftID: null,
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
    if (!id || (!composer.text.trim() && !attachment) || composer.sending) return;
    let media: { id: string; contentType: "image" | "video" | "audio" | "document" } | undefined;
    if (attachment) {
      if (attachment.size > 20 * 1024 * 1024) {
        composer.error = "Attachments must be 20 MiB or smaller.";
        return;
      }
      const form = new FormData();
      form.append("file", attachment);
      try {
        const uploaded = await apiRequest(`/conversations/${id}/media`, { method: "POST", body: form });
        const contentType = attachment.type.startsWith("image/") ? "image"
          : attachment.type.startsWith("video/") ? "video"
          : attachment.type.startsWith("audio/") ? "audio" : "document";
        media = { id: uploaded.id, contentType };
      } catch (reason: any) {
        composer.error = reason?.message || "Failed to upload attachment.";
        return;
      }
    }
    const sent = await inbox.sendMessage(id, composer.text.trim(), composer.aiReplyDraftID ?? undefined, media);
    if (sent) attachment = null;
  }
  async function postNote() {
    const leadID = inbox.activeConvo?.lead?.id;
    const body = internalNote.trim();
    if (!leadID || !body) return;
    await apiRequest(`/leads/${leadID}/notes`, {
      method: "POST",
      body: { body },
    });
    internalNote = "";
    replyTab = "reply";
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
  function useDraft() {
    if (!draft) return;
    composer.text = draft.draft_text;
    composer.aiReplyDraftID = draft.id;
  }
  async function dismissDraft() {
    if (inbox.activeConvoID && draft)
      await inbox.dismissReplyDraft(inbox.activeConvoID, draft.id);
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
    {#if !inbox.activeConvo}<div
        class="flex-1 flex flex-col items-center justify-center p-8 text-center"
      >
        <h3 class="text-sm font-medium text-slate-800">
          Select a conversation
        </h3>
        <p class="text-xs text-slate-400">
          Select a conversation from the list to view messages and send replies.
        </p>
      </div>
    {:else}
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
        <div class="flex items-center gap-2">
          <button
            type="button"
            role="switch"
            aria-label="AI replies for this chat"
            aria-checked={aiReplyEnabled}
            onclick={toggleChatAI}
            disabled={!aiProviderConfigured}
            class="h-8 px-2.5 rounded-lg border text-[11px] {aiReplyEnabled
              ? 'text-emerald-700 bg-emerald-50 border-emerald-200'
              : 'text-slate-500 bg-white border-slate-200'}"
            >{aiControl.run_state === "replying"
              ? "AI replying"
              : aiReplyEnabled
                ? "AI replies on"
                : "AI replies off"}</button
          >
          <div class="relative">
            <button
              onclick={() => (showStatus = !showStatus)}
              title="Conversation Status"
              class="p-2"><CheckCircleIcon class="w-4 h-4" /></button
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
      <div
        bind:this={messageContainer}
        class="flex-1 p-4 sm:p-6 overflow-y-auto space-y-4"
      >
        <div class="flex justify-center">
          <span class="text-[11px] text-slate-400">Messages</span>
        </div>
        {#if !displayMessages.length}<div
            class="text-center py-12 text-slate-400 text-xs"
          >
            No messages recorded in this conversation. Send a reply below.
          </div>{:else}{#each displayMessages as message (message.id)}{@const customer =
              message.sender_type === "contact" ||
              message.sender_type === "customer" ||
              message.direction === "inbound"}{@const text =
              message.parsedContent.text ||
              message.parsedContent.caption ||
              (message.parsedContent.media_id ? "" : JSON.stringify(message.parsedContent))}
            <div
              class="message-row {customer
                ? ''
                : 'outbound'} flex flex-col {customer
                ? 'items-start'
                : 'items-end ml-auto'} max-w-md"
            >
              <div
                class="msg-text p-3.5 rounded-2xl text-xs whitespace-pre-wrap {customer
                  ? 'bg-white border border-slate-200/70 text-slate-800'
                  : 'bg-blue-600 text-white'}"
              >
                {#if message.parsedContent.media_id}
                  {@const mediaURL = `/api-gateway/media/${message.parsedContent.media_id}`}
                  {#if message.content_type === "image"}
                    <img src={mediaURL} alt={text || "Shared image"} class="mb-2 max-h-72 rounded-xl object-contain" loading="lazy" />
                  {:else if message.content_type === "video"}
                    <video src={mediaURL} controls preload="metadata" class="mb-2 max-h-72 rounded-xl"><track kind="captions" /></video>
                  {:else if message.content_type === "audio"}
                    <audio src={mediaURL} controls preload="metadata"><track kind="captions" /></audio>
                  {:else}
                    <a href={mediaURL} target="_blank" rel="noreferrer" class="mb-2 block underline">Open attachment</a>
                  {/if}
                {/if}
                {#if text}{text}{/if}
              </div>
              {#if message.reactions?.length}
                <div class="mt-1 flex flex-wrap gap-1" aria-label="Message reactions">
                  {#each message.reactions as reaction (`${reaction.sender_external_id}:${reaction.emoji}`)}
                    <span class="rounded-full border border-slate-200 bg-white px-2 py-0.5 text-xs shadow-2xs" title={reaction.sender_external_id}>{reaction.emoji}</span>
                  {/each}
                </div>
              {/if}
              <span class="text-[10px] text-slate-400 mt-1"
                >{formatTime(message.created_at)}{#if !customer}<CheckIcon
                    class="w-3.5 h-3.5 text-blue-500 inline-block"
                  /><span class="ml-1">{message.delivery_status || "sent"}</span>{/if}</span
              >
            </div>{/each}{/if}
      </div>
      <div class="p-3 sm:p-4 bg-white border-t border-slate-100 shrink-0">
        {#if capabilities.useReplyDrafts && draft && composer.aiReplyDraftID !== draft.id && replyTab === "reply"}<div
            class="mb-3 p-3 rounded-xl bg-blue-50/60 border border-blue-100"
          >
            <div class="flex justify-between">
              <span class="text-xs font-medium text-blue-700"
                ><SparklesIcon class="w-3.5 h-3.5 inline" /> AI reply suggestion</span
              ><button onclick={dismissDraft} aria-label="Dismiss suggestion"
                ><XMarkIcon class="w-3.5 h-3.5" /></button
              >
            </div>
            <div class="text-xs text-slate-700">{draft.draft_text}</div>
            <div class="flex justify-end">
              <button
                onclick={useDraft}
                class="px-3 py-1 bg-blue-600 text-white text-xs rounded-lg"
                >Use this</button
              >
            </div>
          </div>{/if}
        <div
          class="bg-white rounded-2xl border border-slate-200/90 overflow-hidden"
        >
          {#if capabilities.leadTracking}<div
              class="flex items-center gap-6 px-4 pt-2.5 border-b border-slate-100 text-xs"
            >
              <button
                onclick={() => (replyTab = "reply")}
                class="pb-2 {replyTab === 'reply'
                  ? 'text-blue-600 border-b-2 border-blue-600'
                  : ''}">Reply</button
              ><button
                onclick={() => (replyTab = "note")}
                class="pb-2 {replyTab === 'note'
                  ? 'text-blue-600 border-b-2 border-blue-600'
                  : ''}">Internal Note</button
              >
            </div>{/if}
          {#if composer.error || (inbox.activeConvoID && inbox.mutationErrors[inbox.activeConvoID])}<p
              role="alert"
              class="wf-alert-error"
            >
              {composer.error || inbox.mutationErrors[inbox.activeConvoID!]}
            </p>{/if}
          {#if replyTab === "reply"}{#if aiReplyEnabled}<div
                class="mx-3 my-2.5 rounded-xl bg-slate-100 px-4 py-3 text-xs"
              >
                <div>
                  {aiControl.run_state === "replying"
                    ? "AI is replying..."
                    : "AI is handling this chat."}
                </div>
                <button
                  onclick={() =>
                    inbox.activeConvo &&
                    inbox.updateConversationAIControl(
                      inbox.activeConvo.id,
                      "pause",
                      "",
                    )}
                  class="mt-2 underline"
                  >{aiControl.run_state === "replying"
                    ? "Pause AI"
                    : "Take over"}</button
                >
              </div>{:else}<div class="px-4 py-2.5">
                <input
                  type="text"
                  value={composer.text}
                  oninput={(event) =>
                    setComposerText(event.currentTarget.value)}
                  onkeydown={handleComposerKeydown}
                  placeholder="Enter a message..."
                  class="compose-input w-full text-xs sm:text-sm bg-transparent focus:outline-none"
                />
                {#if attachment}
                  <div class="mt-2 flex items-center justify-between rounded-lg bg-slate-100 px-2.5 py-2 text-[11px] text-slate-600">
                    <span class="truncate">{attachment.name} · {(attachment.size / 1024 / 1024).toFixed(1)} MiB</span>
                    <button aria-label="Remove attachment" onclick={() => { attachment = null; if (attachmentInput) attachmentInput.value = ""; }}><XMarkIcon class="h-3.5 w-3.5" /></button>
                  </div>
                {/if}
              </div>
              <div class="flex justify-between px-3 py-2 border-t border-slate-100">
                <div class="flex text-slate-400">
                  <input bind:this={attachmentInput} type="file" class="hidden" accept="image/*,video/*,audio/*,.pdf,.doc,.docx,.txt" onchange={(event) => { attachment = event.currentTarget.files?.[0] || null; }} />
                  <button title="Add attachment" onclick={() => attachmentInput?.click()}
                    ><PlusIcon class="w-4 h-4" /></button
                  ><button title="Emoji picker"
                    ><FaceSmileIcon class="w-4 h-4" /></button
                  ><button title="Saved replies"
                    ><BookmarkIcon class="w-4 h-4" /></button
                  ><button title="Quick automation"
                    ><BoltIcon class="w-4 h-4" /></button
                  >
                </div>
                <button
                  onclick={sendMessage}
                  disabled={(!composer.text.trim() && !attachment) || composer.sending}
                  class="send-btn w-8 h-8 bg-blue-600 text-white rounded-full flex items-center justify-center disabled:opacity-40"
                  aria-label="Send message"
                  ><PaperAirplaneIcon class="w-4 h-4" /></button
                >
              </div>{/if}{:else}<div class="p-3">
              <textarea
                bind:value={internalNote}
                placeholder="Enter an internal note for team members..."
                class="w-full h-20 p-2.5 text-xs bg-orange-50/40 rounded-xl border border-orange-200 focus:outline-none focus:border-orange-300 transition"
              ></textarea>
              <div class="flex justify-end mt-2">
                <button
                  onclick={postNote}
                  class="px-3 py-1.5 bg-orange-600 text-white text-xs rounded-lg"
                  >Post Internal Note</button
                >
              </div>
            </div>{/if}
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
