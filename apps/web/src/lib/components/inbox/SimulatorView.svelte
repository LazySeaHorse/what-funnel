<script lang="ts">
	import CustomerSimulator from '$lib/CustomerSimulator.svelte';
	import { DevicePhoneMobileIcon, ArrowLeftIcon } from '@fvilers/heroicons-svelte/24/outline';
	let { onBack }: { onBack: () => void } = $props();
</script>

<div class="flex-1 flex flex-col overflow-hidden bg-slate-50/50">
	<!-- Studio Header -->
	<div class="px-6 pt-5 pb-3 flex items-center justify-between border-b border-slate-200/80 shrink-0 bg-white">
		<div class="flex items-center gap-3">
			<div class="w-10 h-10 rounded-xl bg-blue-50 text-blue-600 border border-blue-100/80 flex items-center justify-center font-medium shadow-2xs">
				<DevicePhoneMobileIcon class="w-5 h-5" />
			</div>
			<div class="flex items-center gap-2.5">
				<h1 class="text-2xl font-medium text-slate-900 tracking-tight">Customer Simulation Studio</h1>
				<span class="px-2 py-0.5 rounded-md bg-slate-100 text-slate-600 text-[10px] font-medium border border-slate-200 uppercase tracking-wider">DEV MODE</span>
			</div>
		</div>
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
