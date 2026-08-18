<script lang="ts">
	let {
		name = 'bot',
		size = 18,
		color = 'currentColor',
		strokeWidth = 2,
		class: className = ''
	}: {
		name?: string;
		size?: number;
		color?: string;
		strokeWidth?: number;
		class?: string;
	} = $props();
</script>

<svg
	width={size}
	height={size}
	viewBox="0 0 24 24"
	fill="none"
	stroke={color}
	stroke-width={strokeWidth}
	stroke-linecap="round"
	stroke-linejoin="round"
	class={`notion-icon ${className}`}
	aria-hidden="true"
>
	{#if name === 'bot' || name === 'robot'}
		<!-- Bot / AI -->
		<rect x="3" y="11" width="18" height="10" rx="3" />
		<circle cx="9" cy="16" r="1" fill={color} />
		<circle cx="15" cy="16" r="1" fill={color} />
		<path d="M12 7v4" />
		<line x1="8" y1="3" x2="16" y2="3" />
		<path d="M12 3v4" />

	{:else if name === 'layout' || name === 'kanban' || name === 'workspace'}
		<!-- Workspace / Kanban -->
		<rect x="3" y="3" width="18" height="18" rx="2" />
		<path d="M9 3v18" />
		<path d="M15 3v18" />
		<path d="M3 9h18" />

	{:else if name === 'salon' || name === 'scissors'}
		<!-- Scissors / Beauty -->
		<circle cx="6" cy="6" r="3" />
		<circle cx="6" cy="18" r="3" />
		<line x1="20" y1="4" x2="8.12" y2="15.88" />
		<line x1="14.47" y1="14.48" x2="20" y2="20" />
		<line x1="8.12" y1="8.12" x2="12" y2="12" />

	{:else if name === 'camera' || name === 'photography'}
		<!-- Camera -->
		<path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z" />
		<circle cx="12" cy="13" r="4" />

	{:else if name === 'book' || name === 'tutoring'}
		<!-- Book / Education -->
		<path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20" />
		<path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z" />

	{:else if name === 'wrench' || name === 'home_services'}
		<!-- Wrench / Tools -->
		<path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z" />

	{:else if name === 'briefcase' || name === 'business' || name === 'other'}
		<!-- Briefcase / General business -->
		<rect x="2" y="7" width="20" height="14" rx="2" ry="2" />
		<path d="M16 21V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16" />

	{:else if name === 'whatsapp' || name === 'chat' || name === 'message'}
		<!-- WhatsApp / Chat Bubble -->
		<path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z" />

	{:else if name === 'check' || name === 'success'}
		<!-- Checkmark -->
		<polyline points="20 6 9 17 4 12" />

	{:else if name === 'zap' || name === 'bolt' || name === 'flash'}
		<!-- Lightning / Zap -->
		<polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />

	{:else if name === 'x' || name === 'close'}
		<!-- X / Close -->
		<line x1="18" y1="6" x2="6" y2="18" />
		<line x1="6" y1="6" x2="18" y2="18" />

	{:else if name === 'search'}
		<!-- Search -->
		<circle cx="11" cy="11" r="8" />
		<line x1="21" y1="21" x2="16.65" y2="16.65" />

	{:else if name === 'user' || name === 'person'}
		<!-- User -->
		<path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" />
		<circle cx="12" cy="7" r="4" />

	{:else if name === 'settings' || name === 'cog'}
		<!-- Settings -->
		<circle cx="12" cy="12" r="3" />
		<path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />

	{:else if name === 'pipeline' || name === 'funnel'}
		<!-- Pipeline / Funnel -->
		<polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3" />

	{:else if name === 'kb' || name === 'brain'}
		<!-- KB / Brain / File text -->
		<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
		<polyline points="14 2 14 8 20 8" />
		<line x1="16" y1="13" x2="8" y2="13" />
		<line x1="16" y1="17" x2="8" y2="17" />
		<polyline points="10 9 9 9 8 9" />

	{:else if name === 'channels' || name === 'link'}
		<!-- Link / Channels -->
		<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
		<path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />

	{:else if name === 'users' || name === 'team'}
		<!-- Users / Team -->
		<path d="M17 21v-2a4 4 0 0 0-3-3.87" />
		<path d="M9 21v-2a4 4 0 0 0-4-4.87" />
		<path d="M16 3.13a4 4 0 0 1 0 7.75" />
		<circle cx="9" cy="7" r="4" />

	{:else if name === 'plus'}
		<!-- Plus -->
		<line x1="12" y1="5" x2="12" y2="19" />
		<line x1="5" y1="12" x2="19" y2="12" />

	{:else if name === 'arrow-right'}
		<!-- Arrow right -->
		<line x1="5" y1="12" x2="19" y2="12" />
		<polyline points="12 5 19 12 12 19" />

	{:else if name === 'arrow-left'}
		<!-- Arrow left -->
		<line x1="19" y1="12" x2="5" y2="12" />
		<polyline points="12 19 5 12 12 5" />

	{:else if name === 'sparkles' || name === 'star' || name === 'party'}
		<!-- Sparkles / Party -->
		<path d="m12 3-1.912 5.813a2 2 0 0 1-1.275 1.275L3 12l5.813 1.912a2 2 0 0 1 1.275 1.275L12 21l1.912-5.813a2 2 0 0 1 1.275-1.275L21 12l-5.813-1.912a2 2 0 0 1-1.275-1.275L12 3Z" />

	{:else if name === 'trash'}
		<!-- Trash -->
		<polyline points="3 6 5 6 21 6" />
		<path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />

	{:else if name === 'edit'}
		<!-- Edit -->
		<path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
		<path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />

	{:else if name === 'notes' || name === 'note'}
		<!-- Note -->
		<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
		<polyline points="14 2 14 8 20 8" />

	{:else if name === 'history' || name === 'clock'}
		<!-- History / Clock -->
		<circle cx="12" cy="12" r="10" />
		<polyline points="12 6 12 12 16 14" />

	{:else if name === 'tag'}
		<!-- Tag -->
		<path d="M20.59 13.41l-7.17 7.17a2 2 0 0 1-2.83 0L2 12V2h10l8.59 8.59a2 2 0 0 1 0 2.82z" />
		<line x1="7" y1="7" x2="7.01" y2="7" />

	{:else if name === 'send'}
		<!-- Send -->
		<line x1="22" y1="2" x2="11" y2="13" />
		<polygon points="22 2 15 22 11 13 2 9 22 2" />

	{:else if name === 'logout'}
		<!-- Logout -->
		<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
		<polyline points="16 17 21 12 16 7" />
		<line x1="21" y1="12" x2="9" y2="12" />

	{:else if name === 'instagram'}
		<!-- Instagram -->
		<rect x="2" y="2" width="20" height="20" rx="5" ry="5" />
		<path d="M16 11.37A4 4 0 1 1 12.63 8 4 4 0 0 1 16 11.37z" />
		<line x1="17.5" y1="6.5" x2="17.51" y2="6.5" />

	{:else if name === 'messenger'}
		<!-- Facebook Messenger -->
		<path d="M12 2C6.477 2 2 6.145 2 11.258c0 2.91 1.455 5.518 3.735 7.195V22l3.39-1.86c.915.253 1.883.39 2.875.39 5.523 0 10-4.145 10-9.272C22 6.145 17.523 2 12 2z" />
		<path d="m7.5 13.5 3.5-3.8 2.5 2.5 4.5-4-3.5 3.8-2.5-2.5-4.5 4z" />

	{:else if name === 'telegram'}
		<!-- Telegram -->
		<path d="m21.5 3.5-19 7.5 7 2.5 2 6.5 3.5-3.5 5 4 3-18.5z" />
		<path d="m9.5 13.5 7.5-6.5-6 7.5" />

	{:else if name === 'globe' || name === 'webchat'}
		<!-- Globe / WebChat -->
		<circle cx="12" cy="12" r="10" />
		<line x1="2" y1="12" x2="22" y2="12" />
		<path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />

	{:else if name === 'drag' || name === 'grip' || name === 'handle'}
		<!-- 6 dots drag handle -->
		<circle cx="9" cy="6" r="1.5" fill={color} />
		<circle cx="15" cy="6" r="1.5" fill={color} />
		<circle cx="9" cy="12" r="1.5" fill={color} />
		<circle cx="15" cy="12" r="1.5" fill={color} />
		<circle cx="9" cy="18" r="1.5" fill={color} />
		<circle cx="15" cy="18" r="1.5" fill={color} />

	{:else if name === 'dialogues' || name === 'logo-dots'}
		<!-- 4 rounded dots logo -->
		<circle cx="7" cy="8" r="3" fill={color} />
		<circle cx="17" cy="8" r="3" fill={color} />
		<circle cx="7" cy="16" r="3" fill={color} />
		<circle cx="17" cy="16" r="3" fill={color} />

	{:else if name === 'store' || name === 'building'}
		<!-- Store / Building -->
		<path d="m2 7 1-4h18l1 4" />
		<path d="M4 7v14a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1V7" />
		<path d="M9 22V12h6v10" />

	{:else if name === 'shield'}
		<!-- Shield -->
		<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />

	{:else}
		<!-- Generic dot / fallback mark -->
		<circle cx="12" cy="12" r="6" />
	{/if}
</svg>

<style>
	.notion-icon {
		display: inline-block;
		vertical-align: middle;
		flex-shrink: 0;
	}
</style>
