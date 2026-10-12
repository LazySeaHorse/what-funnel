<script lang="ts">
	import { CheckIcon, SparklesIcon, XMarkIcon } from '@fvilers/heroicons-svelte/24/outline';
	import type { Snippet } from 'svelte';

	let {
		tone,
		onDismiss,
		children,
		...rest
	}: {
		tone: 'info' | 'success' | 'error';
		onDismiss: () => void;
		children: Snippet;
		role?: string;
		'data-testid'?: string;
	} = $props();

	const tones = {
		info: {
			box: 'bg-blue-50/90 border-blue-200 text-blue-900',
			icon: 'text-blue-600',
			dismiss: 'text-blue-500 hover:text-blue-700',
			Icon: SparklesIcon
		},
		success: {
			box: 'bg-emerald-50/90 border-emerald-200 text-emerald-900',
			icon: 'text-emerald-600',
			dismiss: 'text-emerald-500 hover:text-emerald-700',
			Icon: CheckIcon
		},
		error: {
			box: 'bg-rose-50/90 border-rose-200 text-rose-900',
			icon: 'text-rose-600',
			dismiss: 'text-rose-500 hover:text-rose-700',
			Icon: XMarkIcon
		}
	};
	const style = $derived(tones[tone]);
</script>

<div class="px-3.5 py-2.5 border rounded-xl text-xs flex items-center justify-between gap-2 shadow-2xs {style.box}" {...rest}>
	<div class="flex items-center gap-2">
		<style.Icon class="w-4 h-4 shrink-0 {style.icon}" />
		<span>{@render children()}</span>
	</div>
	<button type="button" onclick={onDismiss} class="p-0.5 rounded cursor-pointer {style.dismiss}" aria-label="Dismiss banner">
		<XMarkIcon class="w-4 h-4" />
	</button>
</div>
