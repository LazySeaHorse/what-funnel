<script lang="ts">
	import Icon from '$lib/Icon.svelte';
	type Stage = { key: string; label: string; color: string };
	let { step, totalSteps, stages = $bindable() }: { step: number; totalSteps: number; stages: Stage[] } = $props();
	function add() { const colors = ['#F59E0B', '#3B82F6', '#8B5CF6', '#EC4899', '#06B6D4', '#10B981']; stages = [...stages, { key: `stage_${Date.now()}`, label: 'New Stage', color: colors[stages.length % colors.length] }]; }
	function remove(index: number) { if (stages.length > 1) stages = stages.filter((_, itemIndex) => itemIndex !== index); }
</script>

						<div class="text-center lg:text-left mb-6">
							<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {step} of {totalSteps}</div>
							<h2 class="text-2xl sm:text-3xl font-medium text-slate-900 tracking-tight mb-1">Set up your lead pipeline</h2>
							<p class="text-sm text-slate-500 font-normal">Create the stages your leads will go through.</p>
						</div>

						<div class="space-y-3 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
							<div class="space-y-2.5 w-full">
								{#each stages as stage, i}
									<div class="flex items-center gap-2.5 p-2 sm:p-2.5 bg-slate-50/80 border border-slate-200 rounded-xl w-full">
										<div class="grid grid-cols-2 gap-0.5 text-slate-300 shrink-0 ml-1.5">
											<div class="w-1 h-1 rounded-full bg-slate-400"></div>
											<div class="w-1 h-1 rounded-full bg-slate-400"></div>
											<div class="w-1 h-1 rounded-full bg-slate-400"></div>
											<div class="w-1 h-1 rounded-full bg-slate-400"></div>
											<div class="w-1 h-1 rounded-full bg-slate-400"></div>
											<div class="w-1 h-1 rounded-full bg-slate-400"></div>
										</div>
										<div class="w-2.5 h-2.5 rounded-full shrink-0" style="background-color: {stage.color};"></div>
										<input
											type="text"
											class="flex-1 px-3 py-1.5 bg-white border border-slate-200 rounded-lg text-sm text-slate-900 focus:border-blue-600 focus:ring-1 focus:ring-blue-100 outline-none font-normal"
											bind:value={stage.label}
											placeholder="Stage name"
										/>
										<button
											type="button"
											class="p-1.5 text-slate-400 hover:text-rose-500 rounded-lg hover:bg-rose-50 transition cursor-pointer disabled:opacity-30 disabled:cursor-not-allowed"
											onclick={() => remove(i)}
											title="Remove stage"
											disabled={stages.length <= 1}
										>
											<Icon name="trash" size={16} color="currentColor" />
										</button>
									</div>
								{/each}
							</div>

							<button type="button" class="mt-2 flex items-center gap-2 px-3.5 py-2 text-xs font-medium text-blue-600 hover:text-blue-700 hover:bg-blue-50 rounded-xl transition cursor-pointer border border-blue-200 border-dashed" onclick={add}>
								<Icon name="plus" size={14} color="currentColor" />
								<span>Add another stage</span>
							</button>
						</div>
