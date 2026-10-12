<script lang="ts">
  import { onMount } from "svelte";
  import { apiRequest } from "$lib/api";
  import { Input, Button } from "$lib/components/ui";
  import {
    DEFAULT_AI_PROVIDER_CONFIG,
    applyAIProviderStatus,
    saveAIProviderConfig,
    testAIProvider,
    validateAIProviderConfig,
    type AIProviderTestCheck,
  } from "$lib/ai-provider";
  import type { WorkspaceState } from "$lib/workspace.svelte";
  import AIProviderCheckList from "./AIProviderCheckList.svelte";

  let { workspace }: { workspace?: WorkspaceState } = $props();

  let configured = $state(false);
  let apiKey = $state("");
  let baseURL = $state(DEFAULT_AI_PROVIDER_CONFIG.base_url);
  let analysisModel = $state(DEFAULT_AI_PROVIDER_CONFIG.analysis_model);
  let replyModel = $state(DEFAULT_AI_PROVIDER_CONFIG.reply_model);
  let embeddingModel = $state(DEFAULT_AI_PROVIDER_CONFIG.embedding_model);
  let showKey = $state(false);
  let loading = $state(true);
  let saving = $state(false);
  let testing = $state(false);
  let verifiedConfigFingerprint = $state("");
  let testChecks = $state<AIProviderTestCheck[]>([]);
  let message = $state<{ kind: "success" | "error"; text: string } | null>(
    null,
  );

  function currentConfig() {
    return {
      api_key: apiKey,
      base_url: baseURL,
      analysis_model: analysisModel,
      reply_model: replyModel,
      embedding_model: embeddingModel,
    };
  }

  onMount(async () => {
    try {
      const status = await apiRequest("/workspace/account/ai-config/status");
      const applied = applyAIProviderStatus(status, {
        base_url: baseURL,
        analysis_model: analysisModel,
        reply_model: replyModel,
        embedding_model: embeddingModel,
      });
      configured = applied.configured;
      baseURL = applied.values.base_url;
      analysisModel = applied.values.analysis_model;
      replyModel = applied.values.reply_model;
      embeddingModel = applied.values.embedding_model;
    } catch (error: any) {
      message = {
        kind: "error",
        text: error?.message || "Failed to load AI provider configuration.",
      };
    } finally {
      loading = false;
    }
  });

  async function testConnection() {
    const invalid = validateAIProviderConfig(
      currentConfig(),
      configured,
      "API key is required to test connection.",
    );
    if (invalid) {
      message = { kind: "error", text: invalid };
      return;
    }

    testing = true;
    message = null;
    testChecks = [];
    const outcome = await testAIProvider(currentConfig());
    testChecks = outcome.checks;
    if (outcome.ok) {
      verifiedConfigFingerprint = outcome.fingerprint;
      message = {
        kind: "success",
        text:
          outcome.message || "AI provider connection verified successfully!",
      };
    } else {
      verifiedConfigFingerprint = "";
      message = {
        kind: "error",
        text: (outcome.error as Error)?.message || "AI provider test failed.",
      };
    }
    testing = false;
  }

  async function save() {
    const invalid = validateAIProviderConfig(
      currentConfig(),
      configured,
      "API key is required.",
    );
    if (invalid) {
      message = { kind: "error", text: invalid };
      return;
    }

    saving = true;
    message = null;
    try {
      await saveAIProviderConfig(currentConfig(), verifiedConfigFingerprint);
      apiKey = "";
      showKey = false;
      configured = true;
      await workspace?.refreshAccount();
      message = { kind: "success", text: "AI provider configuration saved." };
    } catch (error: any) {
      message = {
        kind: "error",
        text: error?.message || "Failed to save AI provider configuration.",
      };
    } finally {
      saving = false;
    }
  }
</script>

<form
  class="space-y-6"
  onsubmit={(event) => {
    event.preventDefault();
    void save();
  }}
  aria-busy={loading}
>
  <div>
    <div class="flex flex-wrap items-center gap-2">
      <h2 class="text-base font-medium text-slate-900">AI provider</h2>
      <span
        class="rounded-md px-2 py-0.5 text-[10px] font-medium {configured
          ? 'bg-emerald-50 text-emerald-700'
          : 'bg-orange-50 text-orange-700'}"
        >{loading
          ? "Loading"
          : configured
            ? "Configured"
            : "Not configured"}</span
      >
    </div>
    <p class="mt-1 text-xs leading-relaxed text-slate-500">
      Connect an OpenAI-compatible provider with your own API key. The system
      encrypts credentials before storage.
    </p>
  </div>

  {#if message}
    <div
      role={message.kind === "error" ? "alert" : "status"}
      class="rounded-xl border p-4 text-xs {message.kind === 'error'
        ? 'border-rose-200 bg-rose-50 text-rose-700'
        : 'border-emerald-200 bg-emerald-50 text-emerald-700'}"
    >
      {message.text}
    </div>
  {/if}
  {#if configured}
    <div
      class="rounded-xl border border-emerald-200 bg-emerald-50/60 p-4 text-xs leading-relaxed text-emerald-800"
    >
      An AI provider is connected. The system encrypts and saves your API key.
      You can update the base URL and model names without entering a new key.
    </div>
  {/if}
  <AIProviderCheckList checks={testChecks} />

  <fieldset
    disabled={loading || saving || testing}
    class="space-y-4 text-xs disabled:opacity-60"
  >
    <Input
      id="aiSettingsApiKey"
      type={showKey ? "text" : "password"}
      label={configured ? "New API key" : "API key"}
      autocomplete="new-password"
      bind:value={apiKey}
      class="pr-16"
      placeholder={configured
        ? "Enter a replacement key"
        : "Enter your provider API key"}
      helper="The system does not show the API key after saving."
    >
      {#snippet trailing()}
        <button
          type="button"
          onclick={() => (showKey = !showKey)}
          class="px-3 text-[11px] font-medium text-slate-500 hover:text-slate-800 cursor-pointer"
          aria-label={showKey ? "Hide API key" : "Show API key"}
          >{showKey ? "Hide" : "Show"}</button
        >
      {/snippet}
    </Input>
    <Input
      id="aiSettingsBaseURL"
      type="url"
      label="OpenAI-compatible base URL"
      bind:value={baseURL}
      required
    />
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
      <Input
        id="aiSettingsAnalysisModel"
        label="Analysis model"
        bind:value={analysisModel}
        helper="Knowledge ingestion, spam checks, and summaries."
        required
      />
      <Input
        id="aiSettingsReplyModel"
        label="Customer reply model"
        bind:value={replyModel}
        helper="Customer-facing generated replies only."
        required
      />
      <Input
        id="aiSettingsEmbeddingModel"
        label="Embedding model"
        bind:value={embeddingModel}
        required
      />
    </div>
  </fieldset>
  <div class="flex items-center justify-between border-t border-slate-100 pt-5">
    <Button
      type="button"
      variant="secondary"
      onclick={testConnection}
      disabled={loading || saving || testing || (!configured && !apiKey.trim())}
      busy={testing}
    >
      Test connection
    </Button>
    <Button
      type="submit"
      variant="primary"
      disabled={loading || saving || testing || (!configured && !apiKey.trim())}
      busy={saving}
      class="px-5 py-2.5"
    >
      {configured ? "Save changes" : "Save provider"}
    </Button>
  </div>
</form>
