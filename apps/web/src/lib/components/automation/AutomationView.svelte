<script lang="ts">
	let {
		autoReplyEnabled,
		providerConfigured,
		providerStatusLoaded
	}: {
		autoReplyEnabled: boolean;
		providerConfigured: boolean;
		providerStatusLoaded: boolean;
	} = $props();
</script>

<div class="flex-1 flex flex-col overflow-hidden bg-white">
	<div class="px-6 pt-5 pb-3 flex items-center justify-between shrink-0 bg-white">
		<h1 class="text-2xl font-medium text-slate-900 tracking-tight">Automations</h1>
	</div>

	<div class="flex-1 overflow-y-auto px-6 py-4 space-y-6">
		<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
		<div class="p-4 rounded-2xl border border-slate-200/80 bg-slate-50/40 space-y-3">
			<div class="flex items-center justify-between">
				<div class="flex items-center gap-2">
					<span class="w-8 h-8 rounded-xl bg-blue-50 text-blue-600 flex items-center justify-center font-medium">⚡</span>
					<div>
						<h3 class="text-xs font-medium text-slate-900">Automatic AI replies</h3>
						<p class="text-[11px] text-slate-400">Sends automatic replies to incoming customer messages</p>
					</div>
				</div>
				<span class="px-2 py-0.5 rounded-full text-[10px] font-medium {autoReplyEnabled && providerConfigured ? 'bg-emerald-50 text-emerald-600' : 'bg-slate-100 text-slate-500'}">
					{!providerStatusLoaded ? 'CHECKING' : autoReplyEnabled && providerConfigured ? 'ACTIVE' : !providerConfigured ? 'NOT CONFIGURED' : 'OFF'}
				</span>
			</div>
			<p class="text-xs text-slate-600">
				{autoReplyEnabled && providerConfigured
					? 'Uses your knowledge base to answer customer questions automatically.'
					: !providerConfigured
						? 'Connect an AI provider in Settings before automatic replies can run.'
						: 'Automatic replies are disabled in workspace settings.'}
			</p>
		</div>
	</div>
</div>
</div>
