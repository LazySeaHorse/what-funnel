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
		const colors = ['#F59E0B', '#3B82F6', '#8B5CF6', '#EC4899', '#06B6D4', '#10B981'];
		const nextKey = `stage_${Date.now()}_${states.length + 1}`;
		states = [...states, { key: nextKey, label: 'New Stage', color: colors[states.length % colors.length] }];
	}

	function sanitizeStates(rawStates: any[]) {
		const usedKeys = new Set<string>();
		return rawStates.map((s, idx) => {
			const slug = (s.label || '').trim().toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '');
			const baseKey = slug || s.key || `stage_${idx + 1}`;
			let key = baseKey;
			let counter = 1;
			while (usedKeys.has(key)) {
				counter++;
				key = `${baseKey}_${counter}`;
			}
			usedKeys.add(key);
			return {
				key,
				label: (s.label || '').trim() || 'Stage',
				color: s.color || '#3B82F6'
			};
		});
	}

	async function savePipeline() {
		if (!pipeline) return;
		saving = true;
		error = '';
		success = '';
		try {
			const preparedStates = sanitizeStates(states);
			await apiRequest(`/workspace/pipelines/${pipeline.id}`, {
				method: 'PUT',
				body: { name: pipeline.name, states: preparedStates }
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
			{#each states as state, index (state.key || index)}
				<div class="flex items-center gap-3 rounded-xl border border-slate-200/90 bg-white p-2.5 sm:p-3 shadow-2xs transition hover:border-slate-300">
					<!-- Styled Color Swatch Picker -->
					<div class="relative w-7 h-7 rounded-lg overflow-hidden border border-slate-200/80 shadow-2xs shrink-0 flex items-center justify-center bg-slate-50 cursor-pointer" title="Change color">
						<span class="w-4 h-4 rounded-full border border-black/10 shadow-2xs" style="background-color: {state.color};"></span>
						<input aria-label="{state.label} color" type="color" bind:value={state.color} class="absolute inset-0 opacity-0 w-full h-full cursor-pointer" />
					</div>

					<!-- Stage Name Input -->
					<input
						aria-label="Stage label"
						bind:value={state.label}
						class="wf-input min-w-32 flex-1 rounded-lg px-3 py-1.5 text-sm text-slate-900 border border-slate-200 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-100 font-normal transition"
						placeholder="Stage name"
					/>

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

		<!-- Add another stage button matching Onboarding -->
		<button
			type="button"
			class="flex items-center gap-2 px-3.5 py-2 text-xs font-medium text-blue-600 hover:text-blue-700 hover:bg-blue-50 rounded-xl transition cursor-pointer border border-blue-200 border-dashed"
			onclick={addState}
		>
			<Icon name="plus" size={14} color="currentColor" />
			<span>Add another stage</span>
		</button>

		<div class="flex justify-end pt-2">
			<button onclick={savePipeline} disabled={saving} class="wf-button-primary px-4 py-2.5">{saving ? 'Saving…' : 'Save pipeline'}</button>
		</div>
	{/if}
</div>
