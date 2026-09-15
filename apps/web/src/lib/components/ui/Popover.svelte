<script lang="ts">
	import { fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import type { Snippet } from 'svelte';

	let {
		open = $bindable(false),
		align = 'left',
		width = 'w-48',
		trigger,
		children,
		class: className = ''
	}: {
		open?: boolean;
		align?: 'left' | 'right';
		width?: string;
		trigger: Snippet;
		children: Snippet;
		class?: string;
	} = $props();

	let containerEl = $state<HTMLElement | null>(null);

	function handleClickOutside(event: MouseEvent) {
		if (open && containerEl && !containerEl.contains(event.target as Node)) {
			open = false;
		}
	}

	function handleKeydown(event: KeyboardEvent) {
		if (open && event.key === 'Escape') {
			open = false;
		}
	}
</script>

<svelte:window onclick={handleClickOutside} onkeydown={handleKeydown} />

<div bind:this={containerEl} class="relative inline-block {className}">
	{@render trigger()}

	{#if open}
		<div
			transition:fly={{ y: -4, duration: 120, easing: cubicOut }}
			class="absolute top-full {align === 'right' ? 'right-0' : 'left-0'} mt-1.5 {width} bg-white rounded-xl border border-slate-200 shadow-lg py-1.5 z-50 text-xs"
			role="menu"
			tabindex="-1"
		>
			{@render children()}
		</div>
	{/if}
</div>
