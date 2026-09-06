<script lang="ts">
	import { ChevronDownIcon } from '@fvilers/heroicons-svelte/24/outline';
	import LeadStateBadge from '../LeadStateBadge.svelte';

	let {
		stateKey,
		stateLabel = '',
		states = [],
		onchange
	}: {
		stateKey: string;
		stateLabel?: string;
		states: any[];
		onchange: (stateKey: string) => void | Promise<void>;
	} = $props();
	let open = $state(false);
</script>

<div class="space-y-1.5 relative">
	<span class="font-medium text-slate-700">Lead stage</span>
	<button type="button" onclick={() => (open = !open)} aria-label="Change lead stage" class="w-full flex items-center justify-between p-2.5 bg-amber-50/50 rounded-xl border border-amber-200/80 cursor-pointer hover:bg-amber-50 transition text-left">
		<LeadStateBadge {stateKey} label={stateLabel || stateKey} size="sm" class="border-0 bg-transparent p-0" />
		<ChevronDownIcon class="w-3.5 h-3.5 text-amber-500" />
	</button>
	{#if open}
		<div class="absolute top-full left-0 right-0 mt-1 bg-white rounded-xl border border-slate-200 shadow-md py-1 z-50">
			{#each states as state (state.key)}
				<button type="button" onclick={() => { open = false; void onchange(state.key); }} aria-label={`Set lead stage to ${state.label}`} class="w-full text-left px-3 py-1.5 text-xs font-medium hover:bg-slate-50 text-slate-700 flex items-center gap-2 cursor-pointer">
					<LeadStateBadge stateKey={state.key} label={state.label} size="xs" class="border-0 bg-transparent p-0" />
				</button>
			{/each}
		</div>
	{/if}
</div>
