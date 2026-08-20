<script lang="ts">
	import { page } from '$app/state';
	import heroImage from '$lib/assets/404-hero.webp';

	// Fallback to 404 if status is not explicitly set
	const status = $derived(page?.status ?? 404);
	const isNotFound = $derived(status === 404);
</script>

<svelte:head>
	<title>{isNotFound ? '404 — Page Not Found' : `${status} — Error`} · What Funnel</title>
</svelte:head>

<div class="min-h-screen w-full bg-white flex items-center justify-center p-6 sm:p-10 lg:p-16 selection:bg-blue-100 selection:text-blue-900">
	<div class="w-full max-w-[1360px] mx-auto">
		<div class="grid grid-cols-1 lg:grid-cols-12 gap-8 sm:gap-12 lg:gap-8 items-center">
			
			<!-- Left Column: Headline, Message, Actions -->
			<div class="order-2 lg:order-1 lg:col-span-6 xl:col-span-5 flex flex-col justify-center text-left">
				<h1 class="text-3xl sm:text-4xl md:text-5xl lg:text-[52px] font-medium text-slate-900 tracking-tight leading-[1.12]">
					Oops! This page<br />
					<span class="text-[#0057D0]">wandered off somewhere</span>
				</h1>

				<p class="mt-4 sm:mt-5 text-sm sm:text-base lg:text-[17px] text-slate-500 font-normal leading-relaxed max-w-md">
					We can't find the page you're looking for.<br class="hidden sm:inline" />
					It might have been moved, deleted, or never existed.
				</p>

				<!-- Action Buttons -->
				<div class="mt-8 sm:mt-10 flex flex-col sm:flex-row items-stretch sm:items-center gap-3.5 sm:gap-4">
					<!-- Primary button: Go back home -->
					<a
						href="/"
						class="inline-flex items-center justify-center gap-2.5 rounded-xl sm:rounded-2xl bg-[#0057D0] hover:bg-[#0046A8] px-6 py-3.5 text-sm sm:text-base font-medium text-white shadow-xs transition-all duration-200 active:scale-[0.98] cursor-pointer"
					>
						<svg class="h-4 w-4 sm:h-5 sm:w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
							<line x1="19" y1="12" x2="5" y2="12" />
							<polyline points="12 19 5 12 12 5" />
						</svg>
						<span>Go back home</span>
					</a>

					<!-- Secondary button: Go to inbox -->
					<a
						href="/inbox"
						class="inline-flex items-center justify-center gap-2.5 rounded-xl sm:rounded-2xl border border-slate-200 hover:border-slate-300 bg-white hover:bg-slate-50 px-6 py-3.5 text-sm sm:text-base font-medium text-slate-700 hover:text-slate-900 shadow-2xs transition-all duration-200 active:scale-[0.98] cursor-pointer"
					>
						<span>Go to inbox</span>
						<svg class="h-4 w-4 sm:h-5 sm:w-5 text-slate-400 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
							<path d="M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 0 1-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8Z" />
							<circle cx="8.5" cy="11.5" r="1" fill="currentColor" stroke="none" />
							<circle cx="15.5" cy="11.5" r="1" fill="currentColor" stroke="none" />
							<path d="M10 14a2.5 2.5 0 0 0 4 0" fill="none" stroke="currentColor" stroke-width="1.5" />
						</svg>
					</a>
				</div>
			</div>

			<!-- Right Column: 3D 404 Hero Illustration with Left/Right Edge Fade -->
			<div class="order-1 lg:order-2 lg:col-span-6 xl:col-span-7 flex items-center justify-center pointer-events-none">
				<img
					src={heroImage}
					alt="404 Page Not Found — What Funnel"
					class="w-full max-w-[440px] sm:max-w-[540px] lg:max-w-[620px] xl:max-w-[680px] object-contain drop-shadow-xs select-none [mask-image:linear-gradient(to_right,transparent_0%,black_8%,black_92%,transparent_100%)] [-webkit-mask-image:linear-gradient(to_right,transparent_0%,black_8%,black_92%,transparent_100%)]"
					loading="eager"
				/>
			</div>

		</div>
	</div>
</div>
