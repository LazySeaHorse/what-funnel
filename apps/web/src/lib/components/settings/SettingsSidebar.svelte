<script lang="ts">
	import Icon from '$lib/Icon.svelte';

	type SettingsSection = 'general' | 'business_profile' | 'ai_provider' | 'users_permissions' | 'channels' | 'pipeline';
	let { activeSection = $bindable<SettingsSection>(), productMode, canManageTeam = false }:
		{ activeSection: SettingsSection; productMode: string; canManageTeam?: boolean } = $props();
	const items: Array<{ key: Exclude<SettingsSection, 'pipeline'>; label: string; icon: string }> = [
		{ key: 'general', label: 'General', icon: 'M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4' },
		{ key: 'business_profile', label: 'Business profile', icon: 'M21 13.255A23.931 23.931 0 0112 15c-3.183 0-6.22-.62-9-1.745M16 6V4a2 2 0 00-2-2h-4a2 2 0 00-2 2v2m4 6h.01M5 20h14a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z' },
		{ key: 'ai_provider', label: 'AI provider', icon: '' },
		{ key: 'users_permissions', label: 'Users & permissions', icon: 'M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z' },
		{ key: 'channels', label: 'Channels', icon: 'M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1' }
	];
</script>

<div class="col-span-12 md:col-span-3 lg:col-span-2">
	<div class="space-y-1" role="tablist" aria-label="Workspace settings sections">
		{#each items as item}
			{#if item.key !== 'users_permissions' || canManageTeam}
			<button onclick={() => activeSection = item.key} role="tab" aria-selected={activeSection === item.key} aria-controls={`settings-panel-${item.key}`} class="w-full flex items-center gap-2.5 px-3 py-2 rounded-xl text-xs font-medium transition-all text-left {activeSection === item.key ? 'bg-blue-50/80 text-blue-600 shadow-2xs' : 'text-slate-600 hover:text-slate-900 hover:bg-slate-50'}">
				{#if item.key === 'ai_provider'}
					<Icon name="bot" size={16} class={activeSection === item.key ? 'text-blue-600' : 'text-slate-400'} />
				{:else}
					<svg class="w-4 h-4 {activeSection === item.key ? 'text-blue-600' : 'text-slate-400'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d={item.icon} /></svg>
				{/if}
				<span>{item.label}</span>
			</button>
			{/if}
		{/each}
		{#if productMode === 'full_workspace'}
			<button onclick={() => activeSection = 'pipeline'} role="tab" aria-selected={activeSection === 'pipeline'} aria-controls="settings-panel-pipeline" class="w-full flex items-center gap-2.5 px-3 py-2 rounded-xl text-xs font-medium transition-all text-left {activeSection === 'pipeline' ? 'bg-blue-50/80 text-blue-600 shadow-2xs' : 'text-slate-600 hover:text-slate-900 hover:bg-slate-50'}">
				<Icon name="pipeline" size={16} class={activeSection === 'pipeline' ? 'text-blue-600' : 'text-slate-400'} />
				<span>Lead pipeline</span>
			</button>
		{/if}
	</div>
</div>
