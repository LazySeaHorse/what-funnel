<script lang="ts">
	let {
		tags = [],
		onadd,
		onremove
	}: {
		tags?: string[];
		onadd: (tag: string) => void | Promise<void>;
		onremove: (tag: string) => void | Promise<void>;
	} = $props();
	let editing = $state(false);
	let value = $state('');

	async function submit() {
		if (!value.trim()) return;
		await onadd(value.trim());
		value = '';
		editing = false;
	}
	const tagStyles = [
		{ chip: 'bg-blue-50 text-blue-700 border-blue-200', removeBtn: 'text-blue-700/60 hover:text-blue-700' },
		{ chip: 'bg-emerald-50 text-emerald-800 border-emerald-200', removeBtn: 'text-emerald-800/60 hover:text-emerald-800' },
		{ chip: 'bg-purple-50 text-purple-700 border-purple-200', removeBtn: 'text-purple-700/60 hover:text-purple-700' },
		{ chip: 'bg-pink-50 text-pink-700 border-pink-200', removeBtn: 'text-pink-700/60 hover:text-pink-700' }
	];

	function getTagStyle(str: string) {
		let hash = 0;
		for (let i = 0; i < str.length; i++) {
			hash = (hash << 5) - hash + str.charCodeAt(i);
			hash |= 0;
		}
		return tagStyles[Math.abs(hash) % tagStyles.length];
	}
</script>

<div class="space-y-1.5">
	<span class="font-medium text-slate-700">Tags</span>
	<div class="flex flex-wrap items-center gap-1.5">
		{#if tags.length === 0}
			<span class="text-xs text-slate-400">No tags</span>
		{/if}
		{#each tags as tag (tag)}
			{@const style = getTagStyle(tag)}
			<span class="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg {style.chip} text-xs font-medium border">
				{tag}
				<button type="button" onclick={() => void onremove(tag)} aria-label="Remove tag {tag}" class="{style.removeBtn} cursor-pointer">×</button>
			</span>
		{/each}
		{#if editing}
			<input aria-label="Tag name" bind:value onkeydown={(event) => event.key === 'Enter' && void submit()} placeholder="Tag..." class="w-20 px-2 py-1 text-xs border border-blue-200 rounded-lg focus:outline-none" />
			<button type="button" aria-label="Save tag" onclick={() => void submit()} class="text-xs font-medium text-blue-600 px-1 cursor-pointer">✓</button>
		{:else}
			<button type="button" onclick={() => (editing = true)} title="Add tag" aria-label="Add tag" class="w-7 h-7 rounded-lg border border-dashed border-slate-300 text-slate-400 hover:text-slate-600 hover:border-slate-400 flex items-center justify-center text-xs transition cursor-pointer">+</button>
		{/if}
	</div>
</div>
