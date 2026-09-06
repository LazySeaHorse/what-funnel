<script lang="ts">
  import { onMount } from "svelte";
  import { apiRequest } from "$lib/api";
  import type { InboxState } from "$lib/store.svelte";
  import type { WorkspaceState } from "$lib/workspace.svelte";

  let { inbox, workspace }: { inbox: InboxState; workspace: WorkspaceState } =
    $props();

  type ReplyMode = "" | "auto_send" | "draft_only";

  let accountSlug = $state("");
  let replyMode = $state<ReplyMode>("");
  let workspaceDefault = $state<"auto_send" | "draft_only">("draft_only");
  let effectiveReplyMode = $state<"auto_send" | "draft_only">("draft_only");
  let overrideAllowed = $state(false);
  let loading = $state(true);
  let saving = $state(false);
  let errorMsg = $state("");
  let successMsg = $state("");

  let username = $derived(inbox.currentUser?.username || "agent");
  let loginIdentifier = $derived(
    accountSlug ? `${accountSlug}-${username}` : username,
  );

  onMount(async () => {
    try {
      await workspace.loadCore(inbox.currentUser);
      const requests: Promise<any>[] = [apiRequest("/workspace/account/slug")];
      if (workspace.capabilities.useReplyDrafts) {
        requests.push(apiRequest("/workspace/users/me/reply-mode"));
      }
      const [slugResponse, replyResponse] = await Promise.all(requests);
      accountSlug = slugResponse?.slug || "";
      if (replyResponse) {
        replyMode = replyResponse.reply_mode || "";
        workspaceDefault =
          replyResponse.workspace_default === "auto_send"
            ? "auto_send"
            : "draft_only";
        effectiveReplyMode =
          replyResponse.effective_reply_mode === "auto_send"
            ? "auto_send"
            : "draft_only";
        overrideAllowed = replyResponse.override_allowed === true;
      }
    } catch (err: any) {
      errorMsg = err?.message || "Failed to load your preferences.";
    } finally {
      loading = false;
    }
  });

  async function saveReplyMode() {
    saving = true;
    errorMsg = "";
    successMsg = "";
    try {
      await apiRequest("/workspace/users/me/reply-mode", {
        method: "PATCH",
        body: { reply_mode: replyMode || null },
      });
      effectiveReplyMode = replyMode || workspaceDefault;
      successMsg = "Reply preference saved.";
    } catch (err: any) {
      errorMsg = err?.message || "Failed to save your reply preference.";
    } finally {
      saving = false;
    }
  }

  function modeLabel(mode: "auto_send" | "draft_only") {
    return mode === "auto_send" ? "Auto-send" : "Draft only";
  }
</script>

<section
  class="mx-auto w-full max-w-3xl px-4 py-6 sm:px-6 lg:py-8"
  aria-labelledby="preferences-title"
>
  <div class="mb-6">
    <h1
      id="preferences-title"
      class="text-2xl font-medium tracking-tight text-slate-900"
    >
      Preferences
    </h1>
    <p class="mt-1 text-xs text-slate-500">
      Your account details and personal conversation preferences.
    </p>
  </div>

  {#if errorMsg}
    <div
      class="mb-4 rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-xs text-rose-700"
      role="alert"
    >
      {errorMsg}
    </div>
  {/if}
  {#if successMsg}
    <div
      class="mb-4 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-xs text-emerald-700"
      role="status"
    >
      {successMsg}
    </div>
  {/if}

  {#if loading}
    <div class="space-y-3" aria-label="Loading preferences">
      <div class="h-32 animate-pulse rounded-2xl bg-slate-100"></div>
      <div class="h-44 animate-pulse rounded-2xl bg-slate-100"></div>
    </div>
  {:else}
    <div class="space-y-5">
      <section
        class="rounded-2xl border border-slate-200 bg-white p-5"
        aria-labelledby="profile-heading"
      >
        <div class="flex items-start justify-between gap-4">
          <div>
            <h2 id="profile-heading" class="text-sm font-medium text-slate-900">
              Profile
            </h2>
            <p class="mt-1 text-xs text-slate-500">
              Your identity in {workspace.account?.name || "this workspace"}.
            </p>
          </div>
          <span
            class="rounded-lg bg-slate-100 px-2.5 py-1 text-[11px] font-medium text-slate-600"
            >Agent</span
          >
        </div>
        <dl class="mt-5 grid gap-4 sm:grid-cols-2">
          <div>
            <dt class="text-[11px] font-medium text-slate-400">Username</dt>
            <dd class="mt-1 text-sm font-medium text-slate-800">{username}</dd>
          </div>
          <div>
            <dt class="text-[11px] font-medium text-slate-400">
              Login identifier
            </dt>
            <dd class="mt-1 font-mono text-sm text-slate-800">
              {loginIdentifier}
            </dd>
          </div>
        </dl>
      </section>

      {#if workspace.capabilities.useReplyDrafts}
        <section
          class="rounded-2xl border border-slate-200 bg-white p-5"
          aria-labelledby="reply-mode-heading"
        >
          <h2
            id="reply-mode-heading"
            class="text-sm font-medium text-slate-900"
          >
            AI reply mode
          </h2>
          <p class="mt-1 text-xs text-slate-500">
            Choose how AI assists you when you work on assigned conversations.
          </p>

          {#if overrideAllowed}
            <div class="mt-4 space-y-2">
              <label
                class="flex cursor-pointer items-start gap-3 rounded-xl border p-3 {replyMode ===
                ''
                  ? 'border-blue-300 bg-blue-50/50'
                  : 'border-slate-200'}"
              >
                <input
                  type="radio"
                  name="agent-reply-mode"
                  value=""
                  bind:group={replyMode}
                  class="mt-0.5 accent-blue-600"
                />
                <span
                  ><span class="block text-xs font-medium text-slate-800"
                    >Workspace default</span
                  ><span class="text-[11px] text-slate-500"
                    >Currently {modeLabel(workspaceDefault)}</span
                  ></span
                >
              </label>
              <label
                class="flex cursor-pointer items-start gap-3 rounded-xl border p-3 {replyMode ===
                'draft_only'
                  ? 'border-blue-300 bg-blue-50/50'
                  : 'border-slate-200'}"
              >
                <input
                  type="radio"
                  name="agent-reply-mode"
                  value="draft_only"
                  bind:group={replyMode}
                  class="mt-0.5 accent-blue-600"
                />
                <span
                  ><span class="block text-xs font-medium text-slate-800"
                    >Draft only</span
                  ><span class="text-[11px] text-slate-500"
                    >Review AI suggestions before sending.</span
                  ></span
                >
              </label>
              <label
                class="flex cursor-pointer items-start gap-3 rounded-xl border p-3 {replyMode ===
                'auto_send'
                  ? 'border-blue-300 bg-blue-50/50'
                  : 'border-slate-200'}"
              >
                <input
                  type="radio"
                  name="agent-reply-mode"
                  value="auto_send"
                  bind:group={replyMode}
                  class="mt-0.5 accent-blue-600"
                />
                <span
                  ><span class="block text-xs font-medium text-slate-800"
                    >Auto-send</span
                  ><span class="text-[11px] text-slate-500"
                    >Allow eligible AI answers to send automatically.</span
                  ></span
                >
              </label>
            </div>
            <div
              class="mt-4 flex items-center justify-between gap-4 border-t border-slate-100 pt-4"
            >
              <p class="text-[11px] text-slate-500">
                Effective mode: <span class="font-medium text-slate-700"
                  >{modeLabel(effectiveReplyMode)}</span
                >
              </p>
              <button
                onclick={saveReplyMode}
                disabled={saving}
                class="wf-button-primary px-4 py-2"
                >{saving ? "Saving..." : "Save preference"}</button
              >
            </div>
          {:else}
            <p
              class="mt-4 rounded-xl bg-slate-50 px-3 py-3 text-xs text-slate-600"
            >
              Your manager controls reply mode for this workspace. The current
              mode is {modeLabel(effectiveReplyMode)}.
            </p>
          {/if}
        </section>
      {/if}
    </div>
  {/if}
</section>
