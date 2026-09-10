<script lang="ts">
	import { navigating } from '$app/state';
	import favicon from '$lib/assets/favicon.svg';
	import { onMount } from 'svelte';
	import '../app.css';

	let { children } = $props();

	onMount(() => {
		document.body.classList.add('wf-app-ready');
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

{#if navigating.to}
	<div
		class="wf-route-progress fixed inset-x-0 top-0 z-[100] h-0.5 overflow-hidden bg-blue-100"
		role="progressbar"
		aria-label="Loading page"
	>
		<div class="h-full w-1/3 bg-blue-600"></div>
	</div>
{/if}

{@render children()}

<style>
	.wf-route-progress > div {
		animation: route-progress 1s ease-in-out infinite;
	}

	@keyframes route-progress {
		0% { transform: translateX(-100%); }
		100% { transform: translateX(400%); }
	}

	@media (prefers-reduced-motion: reduce) {
		.wf-route-progress > div { animation: none; width: 70%; }
	}
</style>
