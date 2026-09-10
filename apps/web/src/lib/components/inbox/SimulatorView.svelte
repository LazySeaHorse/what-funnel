<script lang="ts">
	import CustomerSimulator from '$lib/CustomerSimulator.svelte';
	import { ArrowLeftIcon } from '@fvilers/heroicons-svelte/24/outline';
	let { onBack }: { onBack: () => void } = $props();
</script>

<div class="flex-1 flex flex-col overflow-hidden bg-slate-50/50">
	<!-- Studio Header -->
	<div class="px-6 pt-5 pb-3 flex items-center justify-between border-b border-slate-200/80 shrink-0 bg-white">
		<h1 class="text-2xl font-medium text-slate-900 tracking-tight">Simulate</h1>
		<button
			onclick={onBack}
			class="px-3.5 py-1.5 rounded-xl border border-slate-200/90 bg-white text-xs font-medium text-slate-700 hover:bg-slate-50 hover:text-slate-900 transition flex items-center gap-1.5 shadow-2xs cursor-pointer active:scale-[0.98]"
		>
			<ArrowLeftIcon class="w-3.5 h-3.5 text-slate-500" />
			<span>Back to Inbox</span>
		</button>
	</div>

	<!-- Main Studio Canvas -->
	<div class="flex-1 overflow-hidden min-h-0 w-full p-4 sm:p-6">
		<CustomerSimulator />
	</div>
</div>
