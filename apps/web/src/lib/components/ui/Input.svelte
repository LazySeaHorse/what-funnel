<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLInputAttributes } from 'svelte/elements';

	let {
		value = $bindable(''),
		label = '',
		id,
		error = '',
		helper = '',
		leading,
		trailing,
		class: className = '',
		type = 'text',
		disabled = false,
		required = false,
		placeholder = '',
		autocomplete,
		oninput,
		onkeydown,
		...restProps
	}: Omit<HTMLInputAttributes, 'value'> & {
		value?: string | number;
		label?: string;
		error?: string;
		helper?: string;
		leading?: Snippet;
		trailing?: Snippet;
	} = $props();
</script>

<div class="space-y-1.5 w-full">
	{#if label}
		<label for={id} class="block text-xs font-medium text-slate-700">
			{label}
		</label>
	{/if}
	<div class="relative w-full">
		{#if leading}
			<div class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-400">
				{@render leading()}
			</div>
		{/if}
		<input
			{id}
			{type}
			bind:value
			{disabled}
			{required}
			{placeholder}
			{autocomplete}
			{oninput}
			{onkeydown}
			class="w-full rounded-xl border bg-white px-3.5 py-2.5 text-xs font-medium text-slate-800 shadow-2xs outline-none transition focus:ring-2 disabled:bg-slate-50 disabled:text-slate-400 disabled:cursor-not-allowed {leading ? 'pl-10' : ''} {trailing ? 'pr-10' : ''} {error ? 'border-rose-300 focus:border-rose-500 focus:ring-rose-100 text-rose-900' : 'border-slate-200 focus:border-blue-500 focus:ring-blue-100'} {className}"
			{...restProps}
		/>
		{#if trailing}
			<div class="absolute inset-y-0 right-0 pr-3.5 flex items-center text-slate-400">
				{@render trailing()}
			</div>
		{/if}
	</div>
	{#if error}
		<p class="text-[11px] font-medium text-rose-600">{error}</p>
	{:else if helper}
		<p class="text-[11px] text-slate-400">{helper}</p>
	{/if}
</div>
