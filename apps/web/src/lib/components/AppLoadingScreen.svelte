<script lang="ts">
	let {
		message = 'Loading What Funnel…',
		detail = 'Connecting to your workspace.'
	}: {
		message?: string;
		detail?: string;
	} = $props();
</script>

<div class="wf-page flex min-h-screen items-center justify-center p-6" role="status" aria-live="polite">
	<div class="flex flex-col items-center text-center">
		<svg class="h-11 w-11" viewBox="0 0 36 36" fill="none" aria-hidden="true">
			<rect width="36" height="36" rx="10" fill="#0057D0" />
			<circle cx="14" cy="14" r="3" fill="white" />
			<circle cx="22" cy="18" r="4.5" fill="white" />
			<circle cx="14" cy="23" r="2.5" fill="white" />
		</svg>
		<div class="mt-5 h-1 w-32 overflow-hidden rounded-full bg-blue-100" aria-hidden="true">
			<div class="wf-loading-sweep h-full w-1/2 rounded-full bg-blue-600"></div>
		</div>
		<p class="mt-4 text-sm font-medium text-slate-700">{message}</p>
		<p class="mt-1 text-xs text-slate-400">{detail}</p>
	</div>
</div>

<style>
	.wf-loading-sweep {
		animation: loading-sweep 1.15s ease-in-out infinite;
	}

	@keyframes loading-sweep {
		0% { transform: translateX(-110%); }
		100% { transform: translateX(210%); }
	}

	@media (prefers-reduced-motion: reduce) {
		.wf-loading-sweep { animation: none; transform: translateX(50%); }
	}
</style>
