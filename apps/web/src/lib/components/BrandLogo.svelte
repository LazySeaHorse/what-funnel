<script lang="ts">
	let {
	size = 'md',
	class: className = ''
	}: {
		size?: 'sm' | 'md' | 'lg';
		class?: string;
	} = $props();

	const sizes = {
		sm: { icon: 'h-8 w-8', label: 'text-lg' },
		md: { icon: 'h-9 w-9', label: 'text-xl' },
		lg: { icon: 'h-9 w-9', label: 'text-2xl' }
	};
</script>

<div class="flex items-center gap-3 {className}">
	<svg class="shrink-0 {sizes[size].icon}" viewBox="0 0 36 36" fill="none" aria-hidden="true">
		<rect width="36" height="36" rx="10" fill="#0057D0" />
		<circle cx="14" cy="14" r="3" fill="white" />
		<circle cx="22" cy="18" r="4.5" fill="white" />
		<circle cx="14" cy="23" r="2.5" fill="white" />
	</svg>
	<span class="font-medium tracking-tight text-slate-900 {sizes[size].label}">what funnel</span>
</div>
