<script lang="ts">
	import Icon from '$lib/Icon.svelte';

	let {
		step,
		totalSteps,
		productMode,
		businessName,
		channelsText,
		pipelineStageCount,
		teamMemberCount,
		slug,
		aiMode,
		knowledgeSummary,
		onEdit
	}: {
		step: number;
		totalSteps: number;
		productMode: string;
		businessName: string;
		channelsText: string;
		pipelineStageCount: number;
		teamMemberCount: number;
		slug: string;
		aiMode: string;
		knowledgeSummary: string;
		onEdit: (step: number) => void;
	} = $props();
</script>

				<div class="text-center lg:text-left mb-6">
					<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {step} of {totalSteps}</div>
					<h2 class="text-2xl font-medium text-slate-900 tracking-tight mb-1">Review and finish</h2>
					<p class="text-sm text-slate-500 font-normal">Here's a summary of your workspace setup.</p>
				</div>

				<div class="space-y-3 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
					<!-- Business -->
					<div class="flex items-center justify-between p-4 bg-white border border-slate-200 rounded-xl w-full">
						<div class="flex items-center gap-3">
							<div class="w-8 h-8 rounded-lg bg-blue-50 text-blue-600 flex items-center justify-center shrink-0">
								<Icon name="store" size={16} color="currentColor" />
							</div>
							<div>
								<div class="text-[11px] font-medium text-slate-400 uppercase">Business</div>
								<div class="text-sm font-medium text-slate-900">{businessName || 'Your workspace'}</div>
							</div>
						</div>
						<button type="button" class="text-xs font-medium text-blue-600 hover:underline" onclick={() => onEdit(1)}>Edit</button>
					</div>

					<!-- Channels -->
					<div class="flex items-center justify-between p-4 bg-white border border-slate-200 rounded-xl w-full">
						<div class="flex items-center gap-3">
							<div class="w-8 h-8 rounded-lg bg-blue-50 text-blue-600 flex items-center justify-center shrink-0">
								<Icon name="chat" size={16} color="currentColor" />
							</div>
							<div>
								<div class="text-[11px] font-medium text-slate-400 uppercase">Channels</div>
								<div class="text-sm font-medium text-slate-900">{channelsText}</div>
							</div>
						</div>
						<button type="button" class="text-xs font-medium text-blue-600 hover:underline" onclick={() => onEdit(2)}>Edit</button>
					</div>

					{#if productMode === 'full_workspace'}
					<!-- Lead Pipeline -->
					<div class="flex items-center justify-between p-4 bg-white border border-slate-200 rounded-xl w-full">
						<div class="flex items-center gap-3">
							<div class="w-8 h-8 rounded-lg bg-blue-50 text-blue-600 flex items-center justify-center shrink-0">
								<Icon name="pipeline" size={16} color="currentColor" />
							</div>
							<div>
								<div class="text-[11px] font-medium text-slate-400 uppercase">Lead pipeline</div>
								<div class="text-sm font-medium text-slate-900">{pipelineStageCount} stages configured</div>
							</div>
						</div>
						<button type="button" class="text-xs font-medium text-blue-600 hover:underline" onclick={() => onEdit(3)}>Edit</button>
					</div>

					<!-- Team Members -->
					<div class="flex items-center justify-between p-4 bg-white border border-slate-200 rounded-xl w-full">
						<div class="flex items-center gap-3">
							<div class="w-8 h-8 rounded-lg bg-blue-50 text-blue-600 flex items-center justify-center shrink-0">
								<Icon name="users" size={16} color="currentColor" />
							</div>
							<div>
								<div class="text-[11px] font-medium text-slate-400 uppercase">Team</div>
								<div class="text-sm font-medium text-slate-900">{teamMemberCount > 0 ? `${teamMemberCount} team member(s) added (prefix: ${slug || 'default'})` : `Prefix: ${slug || 'default'} (no extra members)`}</div>
							</div>
						</div>
						<button type="button" class="text-xs font-medium text-blue-600 hover:underline" onclick={() => onEdit(4)}>Edit</button>
					</div>
					{/if}

					<!-- AI Assistant -->
					<div class="flex items-center justify-between p-4 bg-white border border-slate-200 rounded-xl w-full">
						<div class="flex items-center gap-3">
							<div class="w-8 h-8 rounded-lg bg-blue-50 text-blue-600 flex items-center justify-center shrink-0">
								<Icon name="bot" size={16} color="currentColor" />
							</div>
							<div>
								<div class="text-[11px] font-medium text-slate-400 uppercase">AI Assistant</div>
								<div class="text-sm font-medium text-slate-900">{aiMode}</div>
							</div>
						</div>
						<button type="button" class="text-xs font-medium text-blue-600 hover:underline" onclick={() => onEdit(5)}>Edit</button>
					</div>

					<!-- Knowledge Base -->
					<div class="flex items-center justify-between p-4 bg-white border border-slate-200 rounded-xl w-full">
						<div class="flex items-center gap-3">
							<div class="w-8 h-8 rounded-lg bg-blue-50 text-blue-600 flex items-center justify-center shrink-0">
								<Icon name="book" size={16} color="currentColor" />
							</div>
							<div>
								<div class="text-[11px] font-medium text-slate-400 uppercase">Knowledge Base</div>
								<div class="text-sm font-medium text-slate-900">{knowledgeSummary}</div>
							</div>
						</div>
						<button type="button" class="text-xs font-medium text-blue-600 hover:underline" onclick={() => onEdit(6)}>Edit</button>
					</div>
				</div>
