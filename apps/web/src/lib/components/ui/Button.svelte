<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';

	type Variant = 'primary' | 'secondary' | 'danger' | 'ghost';
	type Size = 'xs' | 'sm' | 'md' | 'lg';

	let {
		variant = 'secondary',
		size = 'sm',
		busy = false,
		disabled = false,
		children,
		class: className = '',
		type = 'button',
		onclick,
		...restProps
	}: HTMLButtonAttributes & {
		variant?: Variant;
		size?: Size;
		busy?: boolean;
		children?: Snippet;
	} = $props();

	const variantClasses: Record<Variant, string> = {
		primary: 'bg-blue-600 hover:bg-blue-700 text-white shadow-xs focus-visible:ring-2 focus-visible:ring-blue-400 focus-visible:ring-offset-1',
		secondary: 'border border-slate-200 bg-white hover:bg-slate-50 text-slate-700 shadow-2xs focus-visible:ring-2 focus-visible:ring-slate-300 focus-visible:ring-offset-1',
		danger: 'bg-red-600 hover:bg-red-700 text-white shadow-xs focus-visible:ring-2 focus-visible:ring-red-400 focus-visible:ring-offset-1',
		ghost: 'text-slate-600 hover:bg-slate-100 hover:text-slate-900 border border-transparent focus-visible:ring-2 focus-visible:ring-slate-300'
	};

	const sizeClasses: Record<Size, string> = {
		xs: 'px-2.5 py-1 text-[11px] rounded-lg',
		sm: 'px-3.5 py-1.5 text-xs rounded-xl',
		md: 'px-4 py-2 text-xs font-medium rounded-xl',
		lg: 'px-5 py-2.5 text-sm font-medium rounded-xl'
	};
</script>

<button
	{type}
	disabled={disabled || busy}
	{onclick}
	class="inline-flex items-center justify-center gap-2 font-medium transition duration-100 ease-out active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-50 cursor-pointer outline-none {variantClasses[variant]} {sizeClasses[size]} {className}"
	{...restProps}
>
	{#if busy}
		<span class="inline-block h-3.5 w-3.5 animate-spin rounded-full border-2 border-current border-t-transparent" aria-hidden="true"></span>
	{/if}
	{@render children?.()}
</button>
