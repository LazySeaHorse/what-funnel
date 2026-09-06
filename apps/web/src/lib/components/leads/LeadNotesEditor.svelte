<script lang="ts">
	import { PencilSquareIcon } from '@fvilers/heroicons-svelte/24/outline';

	let {
		notes = [],
		loading = false,
		expanded = false,
		onadd
	}: {
		notes?: any[];
		loading?: boolean;
		expanded?: boolean;
		onadd: (body: string) => void | Promise<void>;
	} = $props();
	let composing = $state(false);
	let value = $state('');

	async function submit() {
		if (!value.trim()) return;
		await onadd(value.trim());
		value = '';
		composing = false;
	}

	function formatTime(timestamp?: string | number): string {
		return timestamp ? new Date(timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '';
	}
</script>

<div class="space-y-2">
	<div class="flex items-center justify-between">
		<span class="font-medium text-slate-700">{expanded ? 'Internal notes' : 'Notes'}</span>
		{#if expanded}<button type="button" onclick={() => (composing = true)} class="text-[11px] text-blue-600 font-medium hover:underline cursor-pointer">+ Add note</button>{/if}
	</div>
	{#if loading}
		<div class="p-3 bg-slate-50 text-xs text-slate-400">Loading notes…</div>
	{:else if !expanded && notes.length > 0}
		<div class="bg-slate-50 border border-slate-100 rounded-xl p-3.5 flex items-start justify-between gap-2">
			<div class="text-slate-600 leading-relaxed flex-1">{notes[0].body || notes[0].text}</div>
			<button type="button" onclick={() => (composing = !composing)} class="text-slate-400 hover:text-slate-600 shrink-0 p-0.5 cursor-pointer" title="Edit note" aria-label="Edit note"><PencilSquareIcon class="w-3.5 h-3.5" /></button>
		</div>
	{:else if notes.length === 0}
		<p class="rounded-xl border border-slate-100 bg-slate-50 p-3.5 text-slate-400">No notes found. Add an internal note for team members.</p>
	{:else}
		<div class="space-y-2">
			{#each notes as note (note.id ?? note.created_at ?? note.body)}
				<div class="p-3 rounded-xl bg-white border border-slate-200/80 text-xs text-slate-600 leading-relaxed">
					{note.body || note.text}
					<div class="text-[10px] text-slate-400 mt-1">{formatTime(note.created_at)}</div>
				</div>
			{/each}
		</div>
	{/if}
	{#if composing}
		<div class="p-3 bg-slate-50 rounded-xl space-y-2">
			<textarea bind:value rows="2" placeholder="Add an internal note..." class="w-full p-2 text-xs bg-white border border-slate-200 rounded-lg outline-none focus:border-blue-500 resize-none"></textarea>
			<div class="flex justify-end gap-2">
				<button type="button" onclick={() => { composing = false; value = ''; }} class="text-xs text-slate-500 cursor-pointer">Cancel</button>
				<button type="button" onclick={() => void submit()} class="px-2.5 py-1 bg-blue-600 text-white rounded-lg text-xs cursor-pointer">Save</button>
			</div>
		</div>
	{/if}
</div>
