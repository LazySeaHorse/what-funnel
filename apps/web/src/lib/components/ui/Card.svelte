<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		children,
		header,
		footer,
		class: className = ''
	}: {
		children: Snippet;
		header?: Snippet;
		footer?: Snippet;
		class?: string;
	} = $props();
</script>

<div class="wf-card overflow-hidden {className}">
	{#if header}
		<div class="px-5 py-4 border-b border-slate-100">
			{@render header()}
		</div>
	{/if}
	<div class="p-5 sm:p-6">
		{@render children()}
	</div>
	{#if footer}
		<div class="px-5 py-3.5 border-t border-slate-100 bg-slate-50/50 rounded-b-2xl">
			{@render footer()}
		</div>
	{/if}
</div>
