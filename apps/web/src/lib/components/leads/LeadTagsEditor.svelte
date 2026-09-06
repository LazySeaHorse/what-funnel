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
</script>

<div class="space-y-1.5">
	<span class="font-medium text-slate-700">Tags</span>
	<div class="flex flex-wrap items-center gap-1.5">
		{#if tags.length === 0}
			<span class="text-xs text-slate-400">No tags</span>
		{/if}
		{#each tags as tag (tag)}
			<span class="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg bg-violet-50 text-violet-600 text-xs font-medium border border-violet-200">
				{tag}
				<button type="button" onclick={() => void onremove(tag)} aria-label="Remove tag {tag}" class="text-violet-600/60 hover:text-violet-600 cursor-pointer">×</button>
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
