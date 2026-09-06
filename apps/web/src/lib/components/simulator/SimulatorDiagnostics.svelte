<script lang="ts">
  import {
    BoltIcon,
    CodeBracketIcon,
  } from "@fvilers/heroicons-svelte/24/outline";
  import type { SimulatorController } from "./SimulatorController.svelte";

  let { controller }: { controller: SimulatorController } = $props();
  let showPayloadJSON = $state(false);
  const stages = [
    {
      key: "pattern",
      code: "L1",
      name: "Fast-Path Trigger Match",
      detail: "rapidfuzz regex / exact phrases",
      activeClass: "bg-emerald-50/80 border-emerald-300 text-emerald-900",
      badgeClass: "bg-emerald-600 text-white",
      status: "MATCHED",
    },
    {
      key: "embedding",
      code: "L2",
      name: "Semantic Vector Match",
      detail: "pgvector cosine distance",
      activeClass: "bg-emerald-50/80 border-emerald-300 text-emerald-900",
      badgeClass: "bg-emerald-600 text-white",
      status: "MATCHED",
    },
    {
      key: "llm_grounded",
      code: "L3",
      name: "Knowledge Base RAG",
      detail: "kb_concepts + grounded LLM",
      activeClass: "bg-blue-50/80 border-blue-300 text-blue-900",
      badgeClass: "bg-blue-600 text-white",
      status: "MATCHED",
    },
    {
      key: "human",
      code: "L4",
      name: "Human Queue Escalation",
      detail: "Confidence below threshold",
      activeClass: "bg-amber-50/80 border-amber-300 text-amber-900",
      badgeClass: "bg-amber-600 text-white",
      status: "ESCALATED",
    },
  ];
  function isActive(key: string) {
    return key === "human"
      ? controller.currentTelemetry.stageMatched === "none" &&
          controller.currentTelemetry.action === "flagged_human"
      : controller.currentTelemetry.stageMatched === key;
  }
</script>

<div class="lg:col-span-4 flex flex-col gap-4 overflow-y-auto pl-1">
  <div
    class="p-4 bg-white rounded-2xl border border-slate-200/80 shadow-2xs space-y-4 shrink-0"
  >
    <div
      class="flex items-center justify-between pb-2 border-b border-slate-100"
    >
      <div class="flex items-center gap-2">
        <BoltIcon class="w-4 h-4 text-blue-600" /><span
          class="text-xs font-semibold text-slate-900 tracking-tight"
          >AI Cascade Diagnostics</span
        >
      </div>
      <span class="text-[10px] font-mono text-slate-400"
        >{controller.currentTelemetry.latencyMs
          ? `${controller.currentTelemetry.latencyMs}ms`
          : "Ready"}</span
      >
    </div>
    <div class="space-y-2">
      <span
        class="text-[10px] font-medium uppercase tracking-wider text-slate-400"
        >Cascade Execution Stage</span
      >
      <div class="space-y-1.5">
        {#each stages as stage (stage.key)}
          {@const active = isActive(stage.key)}
          <div
            class="p-2.5 rounded-xl border transition flex items-center justify-between {active
              ? stage.activeClass
              : 'bg-slate-50/60 border-slate-200/60 text-slate-500 opacity-60'}"
          >
            <div class="flex items-center gap-2">
              <span
                class="w-5 h-5 rounded-md flex items-center justify-center text-[10px] font-mono font-medium {active
                  ? stage.badgeClass
                  : 'bg-slate-200 text-slate-600'}">{stage.code}</span
              >
              <div>
                <div class="font-medium text-xs">{stage.name}</div>
                <div class="text-[9px] opacity-80">{stage.detail}</div>
              </div>
            </div>
            {#if active}<span
                class="px-2 py-0.5 rounded-full bg-white/60 text-[9px] font-medium"
                >{stage.status}</span
              >{/if}
          </div>
        {/each}
      </div>
    </div>
    <div class="grid grid-cols-2 gap-2 pt-1">
      <div
        class="p-2.5 bg-slate-50 rounded-xl border border-slate-200/70 space-y-1"
      >
        <span
          class="text-[9px] font-medium uppercase tracking-wider text-slate-400"
          >Confidence</span
        >
        <div
          class="text-sm font-semibold font-mono tabular-nums text-slate-900"
        >
          {controller.currentTelemetry.confidence != null
            ? `${(controller.currentTelemetry.confidence * 100).toFixed(1)}%`
            : "—"}
        </div>
      </div>
      <div
        class="p-2.5 bg-slate-50 rounded-xl border border-slate-200/70 space-y-1"
      >
        <span
          class="text-[9px] font-medium uppercase tracking-wider text-slate-400"
          >Action</span
        >
        <div class="text-xs font-semibold text-slate-900 capitalize truncate">
          {controller.currentTelemetry.action !== "none"
            ? controller.currentTelemetry.action.replace("_", " ")
            : "Idle"}
        </div>
      </div>
    </div>
    {#if controller.currentTelemetry.draftText}
      <div
        class="p-3 bg-blue-50/50 rounded-xl border border-blue-100 space-y-1"
      >
        <div class="flex items-center justify-between">
          <span
            class="text-[10px] font-medium uppercase tracking-wider text-blue-700"
            >AI Reply Draft</span
          ><span class="text-[9px] font-mono text-blue-500 uppercase"
            >{controller.currentTelemetry.draftStatus || "pending"}</span
          >
        </div>
        <p class="text-[11px] text-slate-700 leading-snug">
          {controller.currentTelemetry.draftText}
        </p>
      </div>
    {/if}
  </div>
  <div
    class="p-4 bg-white rounded-2xl border border-slate-200/80 shadow-2xs space-y-2.5 flex-1"
  >
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-2">
        <CodeBracketIcon class="w-4 h-4 text-slate-600" /><span
          class="text-xs font-semibold text-slate-900 tracking-tight"
          >Webhook Payload Inspector</span
        >
      </div>
      <button
        onclick={() => (showPayloadJSON = !showPayloadJSON)}
        class="text-[10px] font-mono text-blue-600 hover:text-blue-700 transition cursor-pointer"
        >{showPayloadJSON ? "Collapse" : "Expand"}</button
      >
    </div>
    <div class="text-[10px] text-slate-500 space-y-1 font-mono">
      <div>
        Endpoint: <span class="text-slate-800"
          >/webhooks/{controller.selectedPlatform}</span
        >
      </div>
      <div>
        Channel ID: <span class="text-slate-800"
          >{controller.selectedChannelID || "none"}</span
        >
      </div>
    </div>
    {#if controller.currentTelemetry.lastPayload}<pre
        class="p-3 bg-slate-900 text-slate-200 rounded-xl font-mono text-[10px] leading-relaxed overflow-x-auto {showPayloadJSON
          ? 'max-h-60'
          : 'max-h-24'}">{JSON.stringify(
          controller.currentTelemetry.lastPayload,
          null,
          2,
        )}</pre>{:else}<div
        class="p-4 bg-slate-50 rounded-xl text-center text-slate-400 text-[11px]"
      >
        Dispatch a message to inspect the raw JSON payload
      </div>{/if}
  </div>
</div>
