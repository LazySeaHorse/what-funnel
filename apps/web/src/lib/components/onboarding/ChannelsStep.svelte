<script lang="ts">
	import Icon from '$lib/Icon.svelte';
	let { step, totalSteps, channels, onConnect }: { step: number; totalSteps: number; channels: any[]; onConnect: (channel: any) => void } = $props();
</script>

						<div class="text-center lg:text-left mb-6">
							<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {step} of {totalSteps}</div>
							<h2 class="text-2xl sm:text-3xl font-medium text-slate-900 tracking-tight mb-1">Connect your channels</h2>
							<p class="text-sm text-slate-500 font-normal">Bring all your conversations into one place.</p>
						</div>

						<div class="space-y-3 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
							{#each channels as ch}
								<div class="flex items-center justify-between p-3.5 sm:p-4 bg-white border border-slate-200 rounded-xl hover:border-slate-300 transition">
									<div class="flex items-center gap-3">
										<div class="w-9 h-9 rounded-lg bg-slate-50 border border-slate-100 flex items-center justify-center shrink-0">
											<Icon name={ch.icon} size={20} color={ch.color} />
										</div>
										<span class="text-sm font-medium text-slate-800">{ch.name}</span>
									</div>

									<div>
										{#if ch.connected}
											<button type="button" class="flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg bg-emerald-50 border border-emerald-200 text-emerald-700 text-xs font-medium cursor-pointer" onclick={() => onConnect(ch)}>
												<Icon name="check" size={14} color="#10B981" />
												<span>Connected</span>
											</button>
										{:else}
											<button type="button" class="px-3.5 py-1.5 rounded-lg border border-slate-200 hover:border-slate-300 bg-white hover:bg-slate-50 text-slate-700 text-xs font-medium transition cursor-pointer shadow-xs" onclick={() => onConnect(ch)}>
												Connect
											</button>
										{/if}
									</div>
								</div>
							{/each}

							<!-- Web Chat coming soon item -->
							<div class="flex items-center justify-between p-3.5 sm:p-4 bg-slate-50/50 border border-slate-200/60 rounded-xl opacity-75">
								<div class="flex items-center gap-3">
									<div class="w-9 h-9 rounded-lg bg-white border border-slate-200 flex items-center justify-center shrink-0 text-slate-400">
										<Icon name="chat" size={18} color="currentColor" />
									</div>
									<span class="text-sm font-medium text-slate-600">Web Chat</span>
								</div>
								<span class="px-2.5 py-1 text-[11px] font-medium text-slate-400 bg-slate-100 rounded-lg">Coming soon</span>
							</div>

							<p class="text-xs text-slate-400 text-center lg:text-left pt-2">You can connect more later.</p>
						</div>
