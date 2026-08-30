<script lang="ts">
	import heroImage from '$lib/assets/404-hero.webp';
	import { ArrowLeftIcon, InboxIcon } from '@fvilers/heroicons-svelte/24/outline';
</script>

<svelte:head>
	<title>404 — Page Not Found · What Funnel</title>
</svelte:head>

<div class="min-h-screen w-full bg-white flex items-center justify-center p-6 sm:p-10 lg:p-16 selection:bg-blue-100 selection:text-blue-900">
	<div class="w-full max-w-[1360px] mx-auto">
		<div class="grid grid-cols-1 lg:grid-cols-12 gap-8 sm:gap-12 lg:gap-8 items-center">
			
			<!-- Left Column: Headline, Message, Actions -->
			<div class="order-2 lg:order-1 lg:col-span-6 xl:col-span-5 flex flex-col justify-center text-left">
				<h1 class="text-3xl sm:text-4xl md:text-5xl lg:text-[52px] font-medium text-slate-900 tracking-tight leading-[1.12]">
					404 — Page not found<br />
					<span class="text-[#0057D0]">The page does not exist</span>
				</h1>

				<p class="mt-4 sm:mt-5 text-sm sm:text-base lg:text-[17px] text-slate-500 font-normal leading-relaxed max-w-md">
					The page you requested does not exist or moved to a new address.
				</p>

				<!-- Action Buttons -->
				<div class="mt-8 sm:mt-10 flex flex-col sm:flex-row items-stretch sm:items-center gap-3.5 sm:gap-4">
					<!-- Primary button: Go to home page -->
					<a
						href="/"
						class="inline-flex items-center justify-center gap-2.5 rounded-xl sm:rounded-2xl bg-[#0057D0] hover:bg-[#0046A8] px-6 py-3.5 text-sm sm:text-base font-medium text-white shadow-xs transition-all duration-200 active:scale-[0.98] cursor-pointer"
					>
						<ArrowLeftIcon class="h-4 w-4 sm:h-5 sm:w-5" />
						<span>Go to home page</span>
					</a>

					<!-- Secondary button: Go to inbox -->
					<a
						href="/inbox"
						class="inline-flex items-center justify-center gap-2.5 rounded-xl sm:rounded-2xl border border-slate-200 hover:border-slate-300 bg-white hover:bg-slate-50 px-6 py-3.5 text-sm sm:text-base font-medium text-slate-700 hover:text-slate-900 shadow-2xs transition-all duration-200 active:scale-[0.98] cursor-pointer"
					>
						<span>Go to inbox</span>
						<InboxIcon class="h-4 w-4 sm:h-5 sm:w-5 text-slate-400 shrink-0" />
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
