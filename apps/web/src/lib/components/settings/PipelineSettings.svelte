<script lang="ts">
	import { onMount } from 'svelte';
	import { apiRequest } from '$lib/api';
	import Icon from '$lib/Icon.svelte';
	import type { WorkspaceState } from '$lib/workspace.svelte';

	let { workspace }: { workspace?: WorkspaceState } = $props();

	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let success = $state('');
	let pipeline = $state<any>(null);
	let states = $state<any[]>([]);
	let newStateKey = $state('');
	let newStateLabel = $state('');
	let newStateColor = $state('#0B6E99');

	onMount(loadPipeline);

	async function loadPipeline() {
		loading = !(workspace?.coreReady ?? false);
		error = '';
		try {
			if (workspace) {
				await workspace.loadCore();
				pipeline = workspace.pipeline;
			} else {
				const pipelines = await apiRequest('/workspace/pipelines');
				pipeline = pipelines?.[0] ?? null;
			}
			states = pipeline?.states ? [...pipeline.states] : [];
		} catch (err: any) {
			error = err.message || 'Failed to load the lead pipeline.';
		} finally {
			loading = false;
		}
	}

	function moveState(index: number, direction: -1 | 1) {
		const next = index + direction;
		if (next < 0 || next >= states.length) return;
		[states[index], states[next]] = [states[next], states[index]];
		states = [...states];
	}

	function removeState(index: number) {
		states = states.filter((_, stateIndex) => stateIndex !== index);
	}

	function addState() {
		const key = newStateKey.trim().toLowerCase().replace(/\s+/g, '_');
		if (!key) {
			error = 'A stage key is required.';
			return;
		}
		if (states.some((state) => state.key === key)) {
			error = 'Stage keys must be unique.';
			return;
		}
		states = [...states, { key, label: newStateLabel.trim() || newStateKey.trim(), color: newStateColor }];
		newStateKey = '';
		newStateLabel = '';
		newStateColor = '#0B6E99';
		error = '';
	}

	async function savePipeline() {
		if (!pipeline) return;
		saving = true;
		error = '';
		success = '';
		try {
			await apiRequest(`/workspace/pipelines/${pipeline.id}`, {
				method: 'PUT',
				body: { name: pipeline.name, states }
			});
			success = 'Pipeline saved.';
			if (workspace) {
				await workspace.refreshPipeline();
				pipeline = workspace.pipeline;
				states = pipeline?.states ? [...pipeline.states] : [];
			} else {
				await loadPipeline();
			}
		} catch (err: any) {
			error = err.message || 'Failed to save the pipeline.';
		} finally {
			saving = false;
		}
	}
</script>

<div class="space-y-5">
	<div>
		<h2 class="text-base font-medium text-slate-900">Lead pipeline</h2>
		<p class="mt-0.5 text-xs text-slate-500">Set the stages your team uses to qualify leads.</p>
	</div>

	{#if error}<p class="wf-alert-error">{error}</p>{/if}
	{#if success}<p class="wf-alert-success">{success}</p>{/if}

	{#if loading}
		<p class="py-8 text-xs text-slate-500">Loading pipeline…</p>
	{:else if !pipeline}
		<p class="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-500">No lead pipeline is configured.</p>
	{:else}
		<div class="space-y-2.5">
			{#each states as state, index (state.key)}
				<div class="flex items-center gap-3 rounded-xl border border-slate-200/90 bg-white p-3 shadow-2xs transition hover:border-slate-300">
					<!-- Styled Color Swatch Picker -->
					<div class="relative w-7 h-7 rounded-lg overflow-hidden border border-slate-200/80 shadow-2xs shrink-0 flex items-center justify-center bg-slate-50">
						<span class="w-4 h-4 rounded-full border border-black/10 shadow-2xs" style="background-color: {state.color};"></span>
						<input aria-label="{state.label} color" type="color" bind:value={state.color} class="absolute inset-0 opacity-0 w-full h-full cursor-pointer" />
					</div>

					<!-- Stage Name Input -->
					<input
						aria-label="Stage label"
						bind:value={state.label}
						class="wf-input min-w-32 flex-1 rounded-lg px-3 py-1.5 text-sm text-slate-900 border border-slate-200 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-100 font-normal transition"
					/>

					<!-- Stage Key Badge -->
					<span class="hidden sm:inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-mono font-medium text-slate-400 bg-slate-100/80 border border-slate-200/60 shrink-0" title="Key: {state.key}">
						{state.key}
					</span>

					<!-- Actions (Move up/down, Remove) -->
					<div class="ml-auto flex items-center gap-1 shrink-0">
						<button
							aria-label="Move {state.label} up"
							onclick={() => moveState(index, -1)}
							disabled={index === 0}
							class="p-1.5 rounded-lg text-slate-400 hover:text-slate-700 hover:bg-slate-100 disabled:opacity-25 disabled:hover:bg-transparent disabled:cursor-not-allowed transition"
							title="Move up"
						>
							<Icon name="arrow-up" size={14} color="currentColor" />
						</button>
						<button
							aria-label="Move {state.label} down"
							onclick={() => moveState(index, 1)}
							disabled={index === states.length - 1}
							class="p-1.5 rounded-lg text-slate-400 hover:text-slate-700 hover:bg-slate-100 disabled:opacity-25 disabled:hover:bg-transparent disabled:cursor-not-allowed transition"
							title="Move down"
						>
							<Icon name="arrow-down" size={14} color="currentColor" />
						</button>
						<button
							aria-label="Remove {state.label}"
							onclick={() => removeState(index)}
							class="p-1.5 rounded-lg text-slate-400 hover:text-rose-600 hover:bg-rose-50 transition cursor-pointer"
							title="Remove stage"
						>
							<Icon name="trash" size={14} color="currentColor" />
						</button>
					</div>
				</div>
			{/each}
		</div>

		<div class="rounded-xl border border-dashed border-slate-200 bg-slate-50/70 p-3.5 sm:p-4 space-y-3">
			<div class="text-xs font-medium text-slate-600 flex items-center gap-1.5">
				<Icon name="plus" size={13} color="currentColor" class="text-slate-400" />
				<span>Add new stage</span>
			</div>
			<div class="grid grid-cols-1 sm:grid-cols-[1fr_1fr_auto_auto] gap-2.5 items-center">
				<input
					bind:value={newStateKey}
					placeholder="Stage key"
					class="wf-input rounded-lg px-3 py-2 text-xs bg-white border border-slate-200 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-100"
				/>
				<input
					bind:value={newStateLabel}
					placeholder="Stage label"
					class="wf-input rounded-lg px-3 py-2 text-xs bg-white border border-slate-200 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-100"
				/>
				<div class="relative w-9 h-9 rounded-lg overflow-hidden border border-slate-200 bg-white shadow-2xs shrink-0 flex items-center justify-center">
					<span class="w-5 h-5 rounded-full border border-black/10 shadow-2xs" style="background-color: {newStateColor};"></span>
					<input aria-label="New stage color" type="color" bind:value={newStateColor} class="absolute inset-0 opacity-0 w-full h-full cursor-pointer" />
				</div>
				<button
					onclick={addState}
					class="inline-flex items-center justify-center gap-1.5 rounded-lg bg-white px-3.5 py-2 text-xs font-medium text-blue-600 border border-blue-200 hover:bg-blue-50 hover:border-blue-300 transition shadow-2xs cursor-pointer"
				>
					<Icon name="plus" size={13} color="currentColor" />
					<span>Add stage</span>
				</button>
			</div>
		</div>

		<div class="flex justify-end">
			<button onclick={savePipeline} disabled={saving} class="wf-button-primary px-4 py-2.5">{saving ? 'Saving…' : 'Save pipeline'}</button>
		</div>
	{/if}
</div>
