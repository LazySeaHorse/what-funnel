<script lang="ts">
	import { CheckIcon } from '@fvilers/heroicons-svelte/24/outline';

	let { currentPlan, storageUsedGB, storageTotalGB, storagePercent, onManagePlan, onDelete }:
		{ currentPlan: string; storageUsedGB: number; storageTotalGB: number; storagePercent: number; onManagePlan: () => void; onDelete: () => void } = $props();
	const features = ['Automatic AI replies', 'Unlimited channels', '10 team members', 'Automations'];
</script>

<div class="col-span-12 lg:col-span-4 space-y-5">
	<div class="bg-white rounded-2xl border border-slate-200/80 p-5 shadow-2xs space-y-4">
		<span class="text-xs font-medium text-slate-700">Workspace plan</span>
		<div class="flex items-center justify-between">
			<span class="text-sm font-medium text-slate-900">{currentPlan}</span>
			<button onclick={onManagePlan} class="text-xs font-medium text-blue-600 bg-white hover:bg-blue-50/80 border border-slate-200 hover:border-blue-200 rounded-lg px-3 py-1 transition cursor-pointer">Manage</button>
		</div>
		<div class="space-y-2 text-xs text-slate-600 pt-1">
			{#each features as feature}
				<div class="flex items-center gap-2">
					<CheckIcon class="w-4 h-4 text-emerald-500 shrink-0" />
					<span>{feature}</span>
				</div>
			{/each}
		</div>
	</div>
	<div class="bg-white rounded-2xl border border-slate-200/80 p-5 shadow-2xs space-y-3">
		<span class="text-xs font-medium text-slate-700 block">Storage</span>
		<div class="text-xs text-slate-600">{storageUsedGB} GB of {storageTotalGB} GB used</div>
		<div class="space-y-1">
			<div class="h-2 w-full bg-slate-100 rounded-full overflow-hidden">
				<div class="h-full bg-blue-500 rounded-full" style={`width: ${storagePercent}%`}></div>
			</div>
			<div class="text-[11px] text-slate-400 font-medium text-right">{storagePercent}%</div>
		</div>
	</div>
	<div class="bg-red-50 border border-red-100 rounded-2xl p-5 shadow-2xs space-y-3">
		<span class="text-xs font-medium text-red-600 block">Danger zone</span>
		<div class="flex items-center justify-between gap-3">
			<div>
				<div class="text-xs font-medium text-slate-900">Delete workspace</div>
				<div class="text-[11px] text-slate-500">You cannot undo this action.</div>
			</div>
			<button onclick={onDelete} class="text-xs font-medium text-red-600 bg-white border border-red-200 rounded-xl px-3.5 py-1.5 shadow-2xs hover:bg-red-50 shrink-0">Delete</button>
		</div>
	</div>
</div>
