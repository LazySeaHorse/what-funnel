<script lang="ts">
	let {
		channel = 'webchat',
		size = 'sm',
		class: className = '',
		showTooltip = true
	}: {
		channel?: string;
		size?: 'xs' | 'sm' | 'md' | 'lg';
		class?: string;
		showTooltip?: boolean;
	} = $props();

	const sizeClasses = {
		xs: 'w-4 h-4 rounded-md text-[10px]',
		sm: 'w-6 h-6 rounded-lg text-xs',
		md: 'w-8 h-8 rounded-xl text-sm',
		lg: 'w-10 h-10 rounded-2xl text-base'
	};

	const iconSizes = {
		xs: 'w-2.5 h-2.5',
		sm: 'w-3.5 h-3.5',
		md: 'w-4 h-4',
		lg: 'w-5 h-5'
	};

	const normalizedChannel = $derived(
		channel.toLowerCase().replace('matrix_', '')
	);
</script>

{#if normalizedChannel === 'instagram'}
	<div
		title={showTooltip ? 'Instagram' : undefined}
		class="{sizeClasses[size]} bg-gradient-to-tr from-amber-500 via-rose-500 to-purple-600 flex items-center justify-center text-white shadow-xs shrink-0 {className}"
	>
		<svg class="{iconSizes[size]}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
			<rect width="20" height="20" x="2" y="2" rx="5" ry="5"/>
			<path d="M16 11.37A4 4 0 1 1 12.63 8 4 4 0 0 1 16 11.37z"/>
			<line x1="17.5" x2="17.51" y1="6.5" y2="6.5"/>
		</svg>
	</div>
{:else if normalizedChannel === 'whatsapp'}
	<div
		title={showTooltip ? 'WhatsApp' : undefined}
		class="{sizeClasses[size]} bg-emerald-500 flex items-center justify-center text-white shadow-xs shrink-0 {className}"
	>
		<svg class="{iconSizes[size]}" viewBox="0 0 24 24" fill="currentColor">
			<path d="M12.04 2c-5.46 0-9.91 4.45-9.91 9.91 0 1.75.46 3.45 1.32 4.95L2.05 22l5.25-1.38c1.45.79 3.08 1.21 4.74 1.21 5.46 0 9.91-4.45 9.91-9.91 0-2.65-1.03-5.14-2.9-7.01A9.816 9.816 0 0 0 12.04 2z"/>
		</svg>
	</div>
{:else if normalizedChannel === 'messenger'}
	<div
		title={showTooltip ? 'Messenger' : undefined}
		class="{sizeClasses[size]} bg-gradient-to-tr from-blue-600 via-indigo-500 to-purple-600 flex items-center justify-center text-white shadow-xs shrink-0 {className}"
	>
		<svg class="{iconSizes[size]}" viewBox="0 0 24 24" fill="currentColor">
			<path d="M12 2C6.36 2 2 6.13 2 11.7c0 2.91 1.2 5.51 3.14 7.35V22l3.05-1.68c1.17.32 2.44.5 3.81.5 5.64 0 10-4.13 10-9.7S17.64 2 12 2zm1 12.3-2.54-2.71-4.96 2.71 5.45-5.79 2.6 2.71 4.9-2.71-5.45 5.79z"/>
		</svg>
	</div>
{:else if normalizedChannel === 'telegram'}
	<div
		title={showTooltip ? 'Telegram' : undefined}
		class="{sizeClasses[size]} bg-sky-500 flex items-center justify-center text-white shadow-xs shrink-0 {className}"
	>
		<svg class="{iconSizes[size]}" viewBox="0 0 24 24" fill="currentColor">
			<path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm4.64 6.8-1.7 8.01c-.13.57-.46.71-.94.44l-2.6-1.92-1.25 1.21c-.14.14-.26.26-.53.26l.19-2.64 4.81-4.34c.21-.19-.05-.29-.32-.11L8.35 13.56l-2.56-.8c-.56-.17-.57-.56.12-.83l10-3.86c.46-.17.87.11.73.73z"/>
		</svg>
	</div>
{:else}
	<div
		title={showTooltip ? 'Webchat' : undefined}
		class="{sizeClasses[size]} bg-blue-500 flex items-center justify-center text-white shadow-xs shrink-0 {className}"
	>
		<svg class="{iconSizes[size]}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
			<path stroke-linecap="round" stroke-linejoin="round" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
		</svg>
	</div>
{/if}
