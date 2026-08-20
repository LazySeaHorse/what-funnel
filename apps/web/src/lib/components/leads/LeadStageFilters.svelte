<script lang="ts">
	let {
		activeFilter = 'all',
		counts = {
			all: 0,
			new: 0,
			contacted: 0,
			follow_up: 0,
			interested: 0,
			converted: 0
		},
		onSelectFilter = () => {}
	}: {
		activeFilter: string;
		counts: {
			all: number;
			new: number;
			contacted: number;
			follow_up: number;
			interested: number;
			converted: number;
		};
		onSelectFilter: (key: string) => void;
	} = $props();

	const stages = [
		{ key: 'all', label: 'All Leads', dot: '' },
		{ key: 'new', label: 'New Lead', dot: 'bg-amber-400' },
		{ key: 'contacted', label: 'Contacted', dot: 'bg-blue-500' },
		{ key: 'follow_up', label: 'Follow-up', dot: 'bg-purple-500' },
		{ key: 'interested', label: 'Interested', dot: 'bg-emerald-500' },
		{ key: 'converted', label: 'Converted', dot: 'bg-emerald-500' }
	];
</script>

<div class="flex items-center gap-3 overflow-x-auto shrink-0 scrollbar-none py-1 w-full">
	{#each stages as st}
		{@const isActive = activeFilter === st.key}
		{@const count = counts[st.key as keyof typeof counts] ?? 0}
		<button
			type="button"
			onclick={() => onSelectFilter(st.key)}
			class="relative flex flex-col justify-between min-w-[108px] h-[58px] px-3.5 py-2.5 rounded-xl text-left transition cursor-pointer select-none shrink-0 {isActive ? 'bg-[#F4F8FE] border border-blue-200/90 shadow-xs' : 'bg-white border border-slate-200/80 hover:border-slate-300 hover:bg-slate-50/50 shadow-[0_1px_2px_rgba(0,0,0,0.02)]'}"
		>
			<div class="flex items-center gap-1.5 text-xs font-medium {isActive ? 'text-blue-600' : 'text-slate-700'}">
				{#if st.dot}
					<span class="w-1.5 h-1.5 rounded-full {st.dot} shrink-0"></span>
				{/if}
				<span class="truncate">{st.label}</span>
			</div>
			<div class="text-xs font-semibold tabular-nums {isActive ? 'text-blue-600' : 'text-slate-800'}">
				{count}
			</div>

			{#if isActive}
				<div class="absolute bottom-0 left-2 right-2 h-[2.5px] bg-blue-600 rounded-full"></div>
			{/if}
		</button>
	{/each}

	<a
		href="/settings/pipeline"
		class="flex items-center justify-center gap-1.5 min-w-[100px] h-[58px] px-3.5 py-2.5 rounded-xl border border-slate-200/80 bg-white hover:bg-slate-50 text-xs font-medium text-slate-600 transition cursor-pointer shrink-0 shadow-[0_1px_2px_rgba(0,0,0,0.02)]"
	>
		<svg class="w-3.5 h-3.5 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
			<path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
		</svg>
		<span>Add view</span>
	</a>
</div>
