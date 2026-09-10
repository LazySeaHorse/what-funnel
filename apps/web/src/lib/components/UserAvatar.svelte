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

	const avatarPalettes = [
		{ bg: '#0057D0', text: '#FFFFFF' }, // Brand blue
		{ bg: '#9AE600', text: '#1A2E05' }, // New green
		{ bg: '#C27AFF', text: '#2E1065' }, // Purple
		{ bg: '#FB64B6', text: '#FFFFFF' }  // Pink
	];

	function getDeterministicPalette(str: string) {
		let hash = 2166136261;
		for (let i = 0; i < str.length; i++) {
			hash = ((hash ^ str.charCodeAt(i)) * 16777619) >>> 0;
		}
		const index = Math.abs(((hash >>> 8) ^ (hash >>> 16) ^ hash) % avatarPalettes.length);
		return avatarPalettes[index];
	}

	const fallbackPalette = $derived(getDeterministicPalette(name || 'user'));

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
		class="rounded-full overflow-hidden flex items-center justify-center font-medium shadow-xs {sizeClasses[size]} {avatar ? 'bg-slate-100' : ''} {className}"
		style={avatar ? undefined : `background-color: ${fallbackPalette.bg}; color: ${fallbackPalette.text};`}
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
