<script lang="ts">
	import {
		BuildingOffice2Icon,
		BriefcaseIcon,
		CpuChipIcon,
		UserGroupIcon,
		LinkIcon,
		FunnelIcon
	} from '@fvilers/heroicons-svelte/24/outline';

	type SettingsSection = 'general' | 'business_profile' | 'ai_provider' | 'users_permissions' | 'channels' | 'pipeline';
	let { activeSection = $bindable<SettingsSection>(), productMode, canManageTeam = false }:
		{ activeSection: SettingsSection; productMode: string; canManageTeam?: boolean } = $props();

	const items = [
		{ key: 'general' as const, label: 'General', icon: BuildingOffice2Icon },
		{ key: 'business_profile' as const, label: 'Business profile', icon: BriefcaseIcon },
		{ key: 'ai_provider' as const, label: 'AI provider', icon: CpuChipIcon },
		{ key: 'users_permissions' as const, label: 'Users & permissions', icon: UserGroupIcon },
		{ key: 'channels' as const, label: 'Channels', icon: LinkIcon }
	];
</script>

<div class="col-span-12 md:col-span-3 lg:col-span-2">
	<div class="space-y-1" role="tablist" aria-label="Workspace settings sections">
		{#each items as item}
			{#if item.key !== 'users_permissions' || canManageTeam}
				{@const IconComponent = item.icon}
				<button
					onclick={() => activeSection = item.key}
					role="tab"
					aria-selected={activeSection === item.key}
					aria-controls={`settings-panel-${item.key}`}
					class="w-full flex items-center gap-2.5 px-3 py-2 rounded-xl text-xs font-medium transition-all text-left {activeSection === item.key ? 'bg-blue-50/80 text-blue-600 shadow-2xs' : 'text-slate-600 hover:text-slate-900 hover:bg-slate-50'}"
				>
					<IconComponent class="w-4 h-4 {activeSection === item.key ? 'text-blue-600' : 'text-slate-400'}" />
					<span>{item.label}</span>
				</button>
			{/if}
		{/each}
		{#if productMode === 'full_workspace'}
			<button
				onclick={() => activeSection = 'pipeline'}
				role="tab"
				aria-selected={activeSection === 'pipeline'}
				aria-controls="settings-panel-pipeline"
				class="w-full flex items-center gap-2.5 px-3 py-2 rounded-xl text-xs font-medium transition-all text-left {activeSection === 'pipeline' ? 'bg-blue-50/80 text-blue-600 shadow-2xs' : 'text-slate-600 hover:text-slate-900 hover:bg-slate-50'}"
			>
				<FunnelIcon class="w-4 h-4 {activeSection === 'pipeline' ? 'text-blue-600' : 'text-slate-400'}" />
				<span>Lead pipeline</span>
			</button>
		{/if}
	</div>
</div>
