<script lang="ts">
	import { fade, scale } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { XMarkIcon } from '@fvilers/heroicons-svelte/24/outline';
	import type { Snippet } from 'svelte';

	let {
		open = true,
		title = '',
		description = '',
		maxWidth = 'max-w-md',
		showClose = true,
		ariaLabel,
		ariaLabelledby,
		closeAriaLabel = 'Close dialog',
		onclose = () => {},
		header,
		children,
		footer
	}: {
		open?: boolean;
		title?: string;
		description?: string;
		maxWidth?: string;
		showClose?: boolean;
		ariaLabel?: string;
		ariaLabelledby?: string;
		closeAriaLabel?: string;
		onclose?: () => void;
		header?: Snippet;
		children: Snippet;
		footer?: Snippet;
	} = $props();

	let dialogLabel = $derived(ariaLabel || (!ariaLabelledby && title ? title : undefined));

	function handleKeydown(event: KeyboardEvent) {
		if (open && event.key === 'Escape') {
			onclose();
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
	<div
		transition:fade={{ duration: 150 }}
		class="wf-modal-backdrop"
		role="presentation"
		onclick={(e) => e.target === e.currentTarget && onclose()}
	>
		<div
			transition:scale={{ start: 0.96, duration: 180, easing: cubicOut }}
			class="wf-modal {maxWidth} space-y-4"
			role="dialog"
			aria-modal="true"
			aria-label={dialogLabel}
			aria-labelledby={ariaLabelledby}
		>
			{#if header}
				{@render header()}
			{:else if title || showClose}
				<div class="flex items-start justify-between gap-3">
					{#if title}
						<div>
							<h3 id={ariaLabelledby} class="text-sm font-medium text-slate-900">{title}</h3>
							{#if description}
								<p class="text-xs text-slate-500 mt-0.5">{description}</p>
							{/if}
						</div>
					{:else}
						<div></div>
					{/if}
					{#if showClose}
						<button
							type="button"
							onclick={onclose}
							aria-label={closeAriaLabel}
							class="w-7 h-7 rounded-lg hover:bg-slate-100 flex items-center justify-center text-slate-400 hover:text-slate-600 transition cursor-pointer shrink-0"
						>
							<XMarkIcon class="w-4 h-4" />
						</button>
					{/if}
				</div>
			{/if}

			<div class="space-y-4">
				{@render children()}
			</div>

			{#if footer}
				<div class="flex items-center justify-end gap-3 pt-2">
					{@render footer()}
				</div>
			{/if}
		</div>
	</div>
{/if}
