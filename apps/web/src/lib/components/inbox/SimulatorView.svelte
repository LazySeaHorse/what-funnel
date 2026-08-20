<script lang="ts">
	import CustomerSimulator from '$lib/CustomerSimulator.svelte';
	let { onBack }: { onBack: () => void } = $props();
</script>

<div class="flex-1 flex flex-col overflow-hidden p-6 space-y-4">
	<div class="flex items-center justify-between pb-2 border-b border-slate-100 shrink-0"><div class="flex items-center gap-3"><div class="w-10 h-10 rounded-2xl bg-purple-50 text-purple-600 flex items-center justify-center font-medium text-lg">📱</div><div><div class="flex items-center gap-2"><h1 class="text-xl font-medium text-slate-900 tracking-tight">Customer Simulation Studio</h1><span class="px-2 py-0.5 rounded-full bg-purple-50 text-purple-700 text-[10px] font-medium border border-purple-200">DEV MODE</span></div><p class="text-xs text-slate-500">Simulate incoming customer messages and test AI auto-replies across multiple channels</p></div></div><button onclick={onBack} class="px-3.5 py-1.5 rounded-xl border border-slate-200 text-xs font-medium text-slate-700 hover:bg-slate-50 transition flex items-center gap-1.5">← Back to Inbox</button></div>
	<div class="flex-1 overflow-y-auto min-h-0 w-full"><CustomerSimulator /></div>
</div>
