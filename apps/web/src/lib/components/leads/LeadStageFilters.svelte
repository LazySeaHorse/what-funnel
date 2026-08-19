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
		{ key: 'all', label: 'All Leads', dot: 'bg-blue-600', activeClass: 'bg-blue-50/60 border-blue-500 text-blue-700 shadow-xs' },
		{ key: 'new', label: 'New Lead', dot: 'bg-amber-500', activeClass: 'bg-amber-50/60 border-amber-500 text-amber-700 shadow-xs' },
		{ key: 'contacted', label: 'Contacted', dot: 'bg-blue-500', activeClass: 'bg-blue-50/60 border-blue-500 text-blue-700 shadow-xs' },
		{ key: 'follow_up', label: 'Follow-up', dot: 'bg-purple-500', activeClass: 'bg-purple-50/60 border-purple-500 text-purple-700 shadow-xs' },
		{ key: 'interested', label: 'Interested', dot: 'bg-emerald-500', activeClass: 'bg-emerald-50/60 border-emerald-500 text-emerald-700 shadow-xs' },
		{ key: 'converted', label: 'Converted', dot: 'bg-teal-500', activeClass: 'bg-teal-50/60 border-teal-500 text-teal-700 shadow-xs' }
	];
</script>

<div class="flex items-center gap-2 overflow-x-auto pb-1 shrink-0">
	{#each stages as st}
		{@const isActive = activeFilter === st.key}
		{@const count = counts[st.key as keyof typeof counts] || 0}
		<button
			onclick={() => onSelectFilter(st.key)}
			class="flex flex-col justify-between px-4 py-2.5 min-w-[100px] h-[64px] rounded-xl border text-left transition-all duration-150 cursor-pointer active:scale-[0.98] {isActive ? st.activeClass : 'bg-white border-slate-200/80 hover:border-slate-300 text-slate-700 hover:bg-slate-50/50'}"
		>
			<div class="flex items-center gap-1.5 text-xs font-medium">
				<span class="w-2 h-2 rounded-full {st.dot}"></span>
				<span>{st.label}</span>
			</div>
			<span class="text-base font-semibold leading-tight text-slate-900 tabular-nums">{count}</span>
		</button>
	{/each}

	<a
		href="/settings/pipeline"
		class="flex items-center justify-center gap-1.5 px-4 h-[64px] rounded-xl border border-dashed border-slate-200 hover:border-slate-300 hover:bg-white text-xs font-medium text-slate-500 transition cursor-pointer shrink-0"
	>
		<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
			<path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
		</svg>
		<span>Manage pipeline</span>
	</a>
</div>
