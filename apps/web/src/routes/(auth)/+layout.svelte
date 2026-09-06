<script lang="ts">
	import type { Snippet } from 'svelte';
	import heroImage from '$lib/assets/sign-in-hero.webp';
	import BrandLogo from '$lib/components/BrandLogo.svelte';

	let { children }: { children: Snippet } = $props();
</script>

<div class="wf-page min-h-screen flex flex-col justify-between p-4 pt-6 pb-0 sm:p-8 lg:p-12 selection:bg-blue-100 selection:text-blue-900 overflow-x-hidden">
	<div class="w-full max-w-[1360px] mx-auto relative flex-1 flex flex-col justify-between lg:justify-center">
		<div class="grid grid-cols-1 lg:grid-cols-12 gap-8 lg:gap-12 items-center flex-1">
			<div class="hidden lg:flex lg:col-span-7 flex-col justify-between h-full relative">
				<div class="pt-6 sm:pt-10 lg:pt-12">
					<div class="flex items-center justify-between">
						<BrandLogo size="lg" />
						<div class="hidden sm:grid grid-cols-4 gap-2 w-fit opacity-40 pr-4" aria-hidden="true">
							{#each { length: 12 } as _}<div class="w-1.5 h-1.5 rounded-full bg-blue-600"></div>{/each}
						</div>
					</div>
					<div class="mt-6 sm:mt-8">
						<h1 class="text-3xl sm:text-4xl lg:text-5xl font-medium text-slate-900 tracking-tight leading-[1.15]">
							All conversations.<br />
							<span class="text-blue-600">All leads.</span> One workspace.
						</h1>
						<p class="text-slate-500 text-sm sm:text-base font-normal leading-relaxed max-w-xl mt-3.5">
							Connect messaging channels, automate replies with AI, and track leads.
						</p>
					</div>
				</div>
				<div class="relative w-full flex items-center justify-center mt-2 lg:mt-0 pointer-events-none">
					<img src={heroImage} alt="What Funnel dashboard illustration" class="w-full max-h-[520px] lg:max-h-[580px] object-contain" loading="eager" />
					<div class="absolute bottom-4 left-2 grid grid-cols-2 gap-1.5 opacity-60" aria-hidden="true">
						{#each { length: 4 } as _}<div class="w-1.5 h-1.5 rounded-full bg-emerald-500"></div>{/each}
					</div>
				</div>
			</div>

			<div class="lg:col-span-5 flex flex-col justify-center items-center lg:items-end w-full flex-1">
				<div class="lg:hidden flex flex-col items-center text-center mb-4 pt-1">
					<BrandLogo size="md" />
				</div>
				{@render children()}
			</div>
		</div>
	</div>

	<div class="lg:hidden w-[calc(100%+2rem)] -mx-4 mt-auto pointer-events-none select-none flex items-end justify-center overflow-hidden leading-none z-0">
		<img src={heroImage} alt="What Funnel dashboard illustration" class="w-full h-auto max-h-[260px] sm:max-h-[320px] object-cover object-bottom block" loading="eager" />
	</div>
</div>
