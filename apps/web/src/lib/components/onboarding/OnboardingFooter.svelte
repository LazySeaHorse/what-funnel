<script lang="ts">
	import { ChevronRightIcon } from '@fvilers/heroicons-svelte/20/solid';
	import { Button } from '$lib/components/ui';
	let { stepNum, kbStatus, rawText, submitting, compiling, continueDisabled = false, onBack, onContinue, onTour, onInbox }:
		{ stepNum: number; kbStatus: string; rawText: string; submitting: boolean; compiling: boolean; continueDisabled?: boolean; onBack: () => void; onContinue: () => void; onTour: () => void; onInbox: () => void } = $props();
</script>

{#if !(stepNum === 6 && (kbStatus === 'processing' || kbStatus === 'publishing'))}
	<footer class="shrink-0 border-t border-slate-100 flex items-center justify-between gap-3 w-full bg-white px-5 sm:px-10 lg:px-12 py-4 sm:py-5">
		{#if stepNum === 8}
			<Button variant="secondary" size="lg" onclick={onTour}>View tour</Button>
			<Button variant="primary" size="lg" class="ml-auto flex items-center gap-2" onclick={onInbox}>
				<span>Go to Inbox</span>
				<ChevronRightIcon class="w-4 h-4 text-white" />
			</Button>
		{:else}
			<div class="hidden sm:block">
				{#if stepNum > 1}
					<Button variant="secondary" size="lg" onclick={onBack} disabled={submitting || compiling}>Back</Button>
				{/if}
			</div>
			{#if stepNum < 8}
				<Button
					variant="primary"
					size="lg"
					class="ml-auto"
					onclick={onContinue}
					disabled={submitting || compiling || continueDisabled}
					busy={submitting || compiling}
				>
					{#if stepNum === 6 && kbStatus === 'input'}
						<span>{rawText.trim() ? 'Organize with AI' : 'Skip'}</span>
					{:else if stepNum === 6 && kbStatus === 'results'}
						<span>Add to Knowledge Base</span>
					{:else if stepNum === 7}
						<span>Complete setup</span>
					{:else}
						<span>Continue</span>
					{/if}
				</Button>
			{/if}
		{/if}
	</footer>
{/if}
