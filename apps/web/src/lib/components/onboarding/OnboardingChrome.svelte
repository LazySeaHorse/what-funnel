<script lang="ts">
	import Icon from '$lib/Icon.svelte';
	import BrandLogo from '$lib/components/BrandLogo.svelte';

	let {
		stepNum,
		stepItems,
		onStep
	}: {
		stepNum: number;
		stepItems: Array<{ num: number; label: string }>;
		onStep: (step: number) => void;
	} = $props();
</script>

<div class="hidden lg:flex w-full lg:w-80 xl:w-96 bg-slate-50 border-b lg:border-b-0 lg:border-r border-slate-200/80 flex-col justify-between shrink-0 p-6 lg:p-10 relative overflow-hidden min-h-[480px] lg:min-h-0">
	<div class="relative z-10">
		<BrandLogo class="mb-8" />
		{#if stepNum === 7}
			<h1 class="text-2xl sm:text-3xl font-medium text-slate-900 leading-snug tracking-tight mb-3">Workspace is<br /><span class="text-blue-600">ready to go</span></h1>
			<p class="text-sm text-slate-500 leading-relaxed mb-6 font-normal">Your setup is complete and channels are connected.</p>
		{:else}
			<h1 class="text-2xl sm:text-3xl font-medium text-slate-900 leading-snug tracking-tight mb-3">Let’s set up<br /><span class="text-blue-600">your workspace</span></h1>
			<p class="text-sm text-slate-500 leading-relaxed mb-6 font-normal">We’ll help you get everything ready step by step.</p>
		{/if}
		<div class="hidden sm:grid grid-cols-4 gap-2 w-fit opacity-40 mb-6">
			{#each Array(12) as _}
				<div class="w-1.5 h-1.5 rounded-full bg-blue-600"></div>
			{/each}
		</div>
	</div>
	<div class="absolute inset-x-0 bottom-0 w-full overflow-hidden pointer-events-none flex flex-col justify-end">
		<div class="absolute inset-x-0 top-0 h-32 bg-gradient-to-b from-[#F9FAFC] via-[#F9FAFC]/60 to-transparent z-10 pointer-events-none"></div>
		<img src={stepNum === 7 ? '/images/onboarding-happy.webp' : '/images/onboarding-sidebar.webp'} alt={stepNum === 7 ? 'Setup Complete Mascot' : 'Workspace Illustration'} class="w-full h-auto max-h-80 object-cover object-bottom [mask-image:linear-gradient(to_bottom,transparent_0%,black_35%)]" />
	</div>
</div>

<div class="hidden lg:flex w-full lg:w-56 p-6 lg:py-10 lg:px-6 border-b lg:border-b-0 lg:border-r border-slate-100 bg-white shrink-0 flex-col justify-start">
	<div class="flex lg:flex-col gap-3 sm:gap-6 overflow-x-auto lg:overflow-x-visible">
		{#each stepItems as item}
			{@const isActive = item.num === stepNum}
			{@const isDone = item.num < stepNum || stepNum === 7}
			<button type="button" class="flex items-center gap-3 bg-transparent border-0 p-0 text-left cursor-pointer transition shrink-0 {isActive ? 'opacity-100' : isDone ? 'opacity-85' : 'opacity-50'}" onclick={() => { if (item.num <= stepNum) onStep(item.num); }}>
				<div class="w-6 h-6 rounded-full flex items-center justify-center text-xs font-medium shrink-0 transition-all {isActive ? 'bg-blue-600 text-white shadow-xs' : isDone ? 'bg-blue-600 text-white' : 'border border-slate-300 text-slate-400 bg-white'}">
					{#if isDone}<Icon name="check" size={12} color="#FFFFFF" strokeWidth={3} />{:else}<span>{item.num}</span>{/if}
				</div>
				<span class="text-xs sm:text-sm font-medium {isActive ? 'text-slate-900' : isDone ? 'text-slate-700' : 'text-slate-400'} whitespace-nowrap">{item.label}</span>
			</button>
		{/each}
	</div>
</div>
