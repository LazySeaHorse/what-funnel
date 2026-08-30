<script lang="ts">
	import { TrashIcon, PlusIcon } from '@fvilers/heroicons-svelte/24/outline';
	type Stage = { key: string; label: string; color: string };
	let { step, totalSteps, stages = $bindable() }: { step: number; totalSteps: number; stages: Stage[] } = $props();
	function add() {
		const colors = ['#F59E0B', '#3B82F6', '#8B5CF6', '#EC4899', '#06B6D4', '#10B981'];
		const nextKey = `stage_${Date.now()}_${stages.length + 1}`;
		stages = [...stages, { key: nextKey, label: 'New Stage', color: colors[stages.length % colors.length] }];
	}
	function remove(index: number) {
		if (stages.length > 1) stages = stages.filter((_, itemIndex) => itemIndex !== index);
	}
</script>

<div class="text-center lg:text-left mb-6">
	<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {step} of {totalSteps}</div>
	<h2 class="text-2xl sm:text-3xl font-medium text-slate-900 tracking-tight mb-1">Set up lead pipeline</h2>
	<p class="text-sm text-slate-500 font-normal">Create the pipeline stages for your leads.</p>
</div>

<div class="space-y-3 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
	<div class="space-y-2.5 w-full">
		{#each stages as stage, i}
			<div class="flex items-center gap-3 p-2.5 sm:p-3 bg-white border border-slate-200/90 rounded-xl shadow-2xs hover:border-slate-300 transition w-full">
				<!-- Styled Color Swatch Picker -->
				<div class="relative w-7 h-7 rounded-lg overflow-hidden border border-slate-200/80 shadow-2xs shrink-0 flex items-center justify-center bg-slate-50 cursor-pointer" title="Change color">
					<span class="w-4 h-4 rounded-full border border-black/10 shadow-2xs" style="background-color: {stage.color};"></span>
					<input aria-label="{stage.label} color" type="color" bind:value={stage.color} class="absolute inset-0 opacity-0 w-full h-full cursor-pointer" />
				</div>

				<input
					type="text"
					aria-label="Stage label"
					class="wf-input min-w-32 flex-1 rounded-lg px-3 py-1.5 text-sm text-slate-900 border border-slate-200 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-100 font-normal transition"
					bind:value={stage.label}
					placeholder="Stage name"
				/>

				<button
					type="button"
					class="p-1.5 text-slate-400 hover:text-rose-600 rounded-lg hover:bg-rose-50 transition cursor-pointer disabled:opacity-30 disabled:cursor-not-allowed"
					onclick={() => remove(i)}
					title="Remove stage"
					aria-label="Remove {stage.label}"
					disabled={stages.length <= 1}
				>
					<TrashIcon class="w-4 h-4" />
				</button>
			</div>
		{/each}
	</div>

	<button type="button" class="mt-2 flex items-center gap-2 px-3.5 py-2 text-xs font-medium text-blue-600 hover:text-blue-700 hover:bg-blue-50 rounded-xl transition cursor-pointer border border-blue-200 border-dashed" onclick={add}>
		<PlusIcon class="w-4 h-4" />
		<span>Add stage</span>
	</button>
</div>
