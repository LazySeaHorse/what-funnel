<script lang="ts">
	type TabItem = {
		key: string;
		label: string;
		count?: number;
	};

	let {
		tabs = [],
		activeTab = $bindable(),
		onchange,
		class: className = ''
	}: {
		tabs: TabItem[];
		activeTab: any;
		onchange?: (key: any) => void;
		class?: string;
	} = $props();

	function selectTab(key: any) {
		activeTab = key;
		onchange?.(key);
	}
</script>

<div class="flex items-center gap-1 border-b border-slate-100 text-xs font-medium text-slate-400 {className}">
	{#each tabs as tab (tab.key)}
		{@const isActive = activeTab === tab.key}
		<button
			type="button"
			aria-pressed={isActive}
			onclick={() => selectTab(tab.key)}
			class="pb-2.5 px-2.5 transition cursor-pointer border-b-2 {isActive ? 'text-blue-600 font-medium border-blue-600' : 'border-transparent text-slate-500 hover:text-slate-800'}"
		>
			{tab.label}
			{#if tab.count !== undefined}
				<span class="ml-1 text-[11px] {isActive ? 'text-blue-600' : 'text-slate-400'}">({tab.count})</span>
			{/if}
		</button>
	{/each}
</div>
