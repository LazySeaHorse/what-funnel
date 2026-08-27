<script lang="ts">
	import Icon from '$lib/Icon.svelte';
	let { stepNum, kbStatus, rawText, submitting, compiling, continueDisabled = false, onBack, onContinue, onTour, onInbox }:
		{ stepNum: number; kbStatus: string; rawText: string; submitting: boolean; compiling: boolean; continueDisabled?: boolean; onBack: () => void; onContinue: () => void; onTour: () => void; onInbox: () => void } = $props();
</script>

{#if !(stepNum === 5 && kbStatus === 'processing')}
	<div class="fixed sm:sticky bottom-0 left-0 right-0 sm:left-auto sm:right-auto px-5 sm:px-0 pt-3 pb-5 sm:pt-8 sm:pb-0 mt-auto border-t border-slate-100 flex items-center justify-between gap-3 w-full bg-white/95 backdrop-blur-xs z-30">
		{#if stepNum === 7}
			<button type="button" class="px-5 py-2.5 rounded-xl border border-slate-200 hover:border-slate-300 text-slate-700 text-sm font-medium hover:bg-slate-50 transition cursor-pointer" onclick={onTour}>Take a quick tour</button>
			<button type="button" class="ml-auto px-6 py-2.5 bg-blue-600 hover:bg-blue-700 active:bg-blue-800 text-white font-medium rounded-xl text-sm shadow-xs hover:shadow-sm transition-all duration-150 cursor-pointer flex items-center justify-center gap-2" onclick={onInbox}><span>Go to Inbox</span><Icon name="chevron-right" size={14} color="#FFFFFF" /></button>
		{:else}
			<div class="hidden sm:block">
				{#if stepNum > 1}
					<button type="button" class="px-5 py-2.5 rounded-xl border border-slate-200 hover:border-slate-300 text-slate-700 text-sm font-medium hover:bg-slate-50 transition cursor-pointer disabled:opacity-50" onclick={onBack} disabled={submitting || compiling}>Back</button>
				{/if}
			</div>
			{#if stepNum < 7}
				<button type="button" class="ml-auto w-auto px-6 py-2.5 bg-blue-600 hover:bg-blue-700 active:bg-blue-800 text-white font-medium rounded-xl text-sm shadow-xs hover:shadow-sm transition-all duration-150 cursor-pointer disabled:opacity-60 disabled:cursor-not-allowed flex items-center justify-center gap-2" onclick={onContinue} disabled={submitting || compiling || continueDisabled}>
					{#if submitting || compiling}<span>Processing...</span>{:else if stepNum === 5 && kbStatus === 'input'}<span>{rawText.trim() ? 'Organize with AI' : 'Skip'}</span>{:else if stepNum === 6}<span>Complete setup</span>{:else}<span>Continue</span>{/if}
				</button>
			{/if}
		{/if}
	</div>
{/if}
