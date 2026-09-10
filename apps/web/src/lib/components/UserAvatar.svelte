<script lang="ts">
	import ChannelBadge from './ChannelBadge.svelte';

	let {
		name = '',
		avatar = '',
		size = 'md',
		channel = '',
		class: className = ''
	}: {
		name?: string;
		avatar?: string;
		size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl';
		channel?: string;
		class?: string;
	} = $props();

	const initials = $derived(
		name ? name.trim().split(/\s+/).map((n) => n.charAt(0).toUpperCase()).slice(0, 2).join('') : '?'
	);

	const bgColors = [
		'bg-[#0057D0] text-white',
		'bg-[#9AE600] text-slate-900',
		'bg-[#C27AFF] text-purple-950',
		'bg-[#FB64B6] text-white'
	];

	function getDeterministicBg(str: string): string {
		let hash = 0;
		for (let i = 0; i < str.length; i++) {
			hash = (hash << 5) - hash + str.charCodeAt(i);
			hash |= 0;
		}
		return bgColors[Math.abs(hash) % bgColors.length];
	}

	const fallbackBg = $derived(getDeterministicBg(name || 'user'));

	const sizeClasses = {
		xs: 'w-5 h-5 text-[10px]',
		sm: 'w-6 h-6 text-xs',
		md: 'w-8 h-8 text-xs',
		lg: 'w-9 h-9 text-xs font-medium',
		xl: 'w-12 h-12 text-base font-medium',
		'2xl': 'w-14 h-14 text-base font-medium'
	};
</script>

<div class="relative inline-flex shrink-0">
	<div
		class="rounded-full overflow-hidden flex items-center justify-center font-medium shadow-xs {sizeClasses[size]} {avatar ? 'bg-slate-100' : fallbackBg} {className}"
		title={name}
	>
		{#if avatar}
			<img src={avatar} alt={name} class="w-full h-full object-cover" />
		{:else}
			<span>{initials}</span>
		{/if}
	</div>

	{#if channel}
		<div class="absolute -bottom-0.5 -right-0.5 ring-2 ring-white rounded-full">
			<ChannelBadge {channel} size="xs" showTooltip={false} />
		</div>
	{/if}
</div>
