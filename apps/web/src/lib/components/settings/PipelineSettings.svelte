<script lang="ts">
	import { onMount } from 'svelte';
	import { apiRequest } from '$lib/api';
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
		<div class="space-y-2">
			{#each states as state, index (state.key)}
				<div class="flex flex-wrap items-center gap-2 rounded-xl border border-slate-200 bg-white p-3">
					<input aria-label="{state.label} color" type="color" bind:value={state.color} class="h-7 w-8 cursor-pointer rounded border-0 bg-transparent p-0" />
					<input aria-label="Stage label" bind:value={state.label} class="wf-input min-w-36 flex-1 rounded-lg px-2.5 py-1.5" />
					<span class="text-[11px] text-slate-400">{state.key}</span>
					<div class="ml-auto flex gap-1">
						<button aria-label="Move {state.label} up" onclick={() => moveState(index, -1)} disabled={index === 0} class="rounded-lg px-2 py-1 text-xs text-slate-600 hover:bg-slate-100 disabled:opacity-30">↑</button>
						<button aria-label="Move {state.label} down" onclick={() => moveState(index, 1)} disabled={index === states.length - 1} class="rounded-lg px-2 py-1 text-xs text-slate-600 hover:bg-slate-100 disabled:opacity-30">↓</button>
						<button aria-label="Remove {state.label}" onclick={() => removeState(index)} class="rounded-lg px-2 py-1 text-xs text-rose-600 hover:bg-rose-50">Remove</button>
					</div>
				</div>
			{/each}
		</div>

		<div class="grid grid-cols-1 gap-2 rounded-xl border border-dashed border-slate-300 bg-slate-50 p-3 sm:grid-cols-[1fr_1fr_auto_auto]">
			<input bind:value={newStateKey} placeholder="Stage key" class="wf-input rounded-lg px-2.5 py-2" />
			<input bind:value={newStateLabel} placeholder="Stage label" class="wf-input rounded-lg px-2.5 py-2" />
			<input aria-label="New stage color" type="color" bind:value={newStateColor} class="h-8 w-full cursor-pointer rounded border-0 bg-transparent p-0 sm:w-10" />
			<button onclick={addState} class="rounded-lg bg-white px-3 py-2 text-xs font-medium text-blue-600 ring-1 ring-slate-200 hover:bg-blue-50">Add stage</button>
		</div>

		<div class="flex justify-end">
			<button onclick={savePipeline} disabled={saving} class="wf-button-primary px-4 py-2.5">{saving ? 'Saving…' : 'Save pipeline'}</button>
		</div>
	{/if}
</div>
