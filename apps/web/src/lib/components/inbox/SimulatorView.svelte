<script lang="ts">
	import CustomerSimulator from '$lib/CustomerSimulator.svelte';
	let { onBack }: { onBack: () => void } = $props();
</script>

<div class="flex-1 flex flex-col overflow-hidden p-4 sm:p-6 space-y-4 bg-slate-50/50">
	<!-- Studio Header -->
	<div class="flex items-center justify-between pb-3 border-b border-slate-200/80 shrink-0">
		<div class="flex items-center gap-3">
			<div class="w-10 h-10 rounded-xl bg-blue-50 text-blue-600 border border-blue-100/80 flex items-center justify-center font-medium shadow-2xs">
				<svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
					<rect width="14" height="20" x="5" y="2" rx="2" ry="2"/>
					<path d="M12 18h.01"/>
				</svg>
			</div>
			<div>
				<div class="flex items-center gap-2">
					<h1 class="text-lg sm:text-xl font-semibold text-slate-900 tracking-tight">Customer Simulation Studio</h1>
					<span class="px-2 py-0.5 rounded-md bg-slate-100 text-slate-600 text-[10px] font-medium border border-slate-200 uppercase tracking-wider">DEV MODE</span>
				</div>
				<p class="text-xs text-slate-500">Simulate incoming customer messages, test multi-channel routing, and inspect AI cascade execution telemetry</p>
			</div>
		</div>
		<button
			onclick={onBack}
			class="px-3.5 py-1.5 rounded-xl border border-slate-200/90 bg-white text-xs font-medium text-slate-700 hover:bg-slate-50 hover:text-slate-900 transition flex items-center gap-1.5 shadow-2xs cursor-pointer active:scale-[0.98]"
		>
			<svg class="w-3.5 h-3.5 text-slate-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
				<path d="m15 18-6-6 6-6"/>
			</svg>
			<span>Back to Inbox</span>
		</button>
	</div>

	<!-- Main Studio Canvas -->
	<div class="flex-1 overflow-hidden min-h-0 w-full">
		<CustomerSimulator />
	</div>
</div>
