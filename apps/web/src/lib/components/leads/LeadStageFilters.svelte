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
		{ key: 'all', label: 'All Leads', dot: 'bg-blue-600', activeClass: 'bg-blue-50 text-blue-600 border-blue-200' },
		{ key: 'new', label: 'New Lead', dot: 'bg-amber-500', activeClass: 'bg-amber-50 text-amber-700 border-amber-200' },
		{ key: 'contacted', label: 'Contacted', dot: 'bg-blue-500', activeClass: 'bg-blue-50 text-blue-700 border-blue-200' },
		{ key: 'follow_up', label: 'Follow-up', dot: 'bg-purple-500', activeClass: 'bg-purple-50 text-purple-700 border-purple-200' },
		{ key: 'interested', label: 'Interested', dot: 'bg-emerald-500', activeClass: 'bg-emerald-50 text-emerald-700 border-emerald-200' },
		{ key: 'converted', label: 'Converted', dot: 'bg-teal-500', activeClass: 'bg-teal-50 text-teal-700 border-teal-200' }
	];
</script>

<div class="flex items-center gap-1.5 overflow-x-auto shrink-0 scrollbar-none">
	{#each stages as st}
		{@const isActive = activeFilter === st.key}
		{@const count = counts[st.key as keyof typeof counts] || 0}
		<button
			onclick={() => onSelectFilter(st.key)}
			class="flex items-center gap-2 px-3 py-1.5 rounded-xl border text-xs font-medium transition cursor-pointer {isActive ? st.activeClass : 'bg-white border-slate-200/80 hover:bg-slate-50 text-slate-600'}"
		>
			<span class="w-2 h-2 rounded-full {st.dot}"></span>
			<span>{st.label}</span>
			<span class="px-1.5 py-0.2 rounded-md text-[11px] font-semibold {isActive ? 'bg-white/80' : 'bg-slate-100 text-slate-500'} tabular-nums">{count}</span>
		</button>
	{/each}

	<a
		href="/settings/pipeline"
		class="flex items-center gap-1 px-3 py-1.5 rounded-xl border border-dashed border-slate-200 hover:border-slate-300 hover:bg-slate-50 text-xs font-medium text-slate-500 transition cursor-pointer shrink-0 ml-1"
	>
		<svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
			<path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
		</svg>
		<span>Manage</span>
	</a>
</div>
