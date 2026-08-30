<script lang="ts">
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
	import heroImage from '$lib/assets/sign-in-hero.webp';
	import BrandLogo from '$lib/components/BrandLogo.svelte';
	import {
		EnvelopeIcon,
		LockClosedIcon,
		EyeIcon,
		EyeSlashIcon,
		ExclamationCircleIcon
	} from '@fvilers/heroicons-svelte/24/outline';

	let identifier = $state('');
	let password = $state('');
	let showPassword = $state(false);
	let error = $state('');
	let loading = $state(false);

	async function handleLogin(e: Event) {
		e.preventDefault();
		loading = true;
		error = '';
		try {
			await apiRequest('/auth/login', {
				method: 'POST',
				body: { identifier, password }
			});
			goto('/inbox');
		} catch (err: any) {
			error = err.message || 'The username or password is incorrect. Try again.';
		} finally {
			loading = false;
		}
	}

</script>

<svelte:head>
	<title>Sign in — What Funnel</title>
</svelte:head>

<div class="wf-page min-h-screen flex flex-col justify-between p-4 pt-6 pb-0 sm:p-8 lg:p-12 selection:bg-blue-100 selection:text-blue-900 overflow-x-hidden">
	<div class="w-full max-w-[1360px] mx-auto relative flex-1 flex flex-col justify-between lg:justify-center">
		
		<div class="grid grid-cols-1 lg:grid-cols-12 gap-8 lg:gap-12 items-center flex-1">
			
			<!-- Left Column: Brand, Headline, 3D Hero Image (Desktop only) -->
			<div class="hidden lg:flex lg:col-span-7 flex-col justify-between h-full relative">
				
				<!-- Top Content -->
				<div class="pt-6 sm:pt-10 lg:pt-12">
					<!-- Top Bar: Brand Logo & Decorative Dots -->
					<div class="flex items-center justify-between">
						<BrandLogo size="lg" />

						<!-- Decorative 4x3 Dot Matrix -->
						<div class="hidden sm:grid grid-cols-4 gap-2 w-fit opacity-40 pr-4">
							<div class="w-1.5 h-1.5 rounded-full bg-blue-600"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-blue-600"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-blue-600"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-blue-600"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-blue-600"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-blue-600"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-blue-600"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-blue-600"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-blue-600"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-blue-600"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-blue-600"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-blue-600"></div>
						</div>
					</div>

					<!-- Hero Headline and Subhead -->
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

				<!-- 3D Hero Illustration -->
				<div class="relative w-full flex items-center justify-center mt-2 lg:mt-0 pointer-events-none">
					<img
						src={heroImage}
						alt="What Funnel dashboard illustration"
						class="w-full max-h-[520px] lg:max-h-[580px] object-contain"
						loading="eager"
					/>
					
					<!-- Decorative 2x2 Green Dots -->
					<div class="absolute bottom-4 left-2 grid grid-cols-2 gap-1.5 opacity-60">
						<div class="w-1.5 h-1.5 rounded-full bg-emerald-500"></div>
						<div class="w-1.5 h-1.5 rounded-full bg-emerald-500"></div>
						<div class="w-1.5 h-1.5 rounded-full bg-emerald-500"></div>
						<div class="w-1.5 h-1.5 rounded-full bg-emerald-500"></div>
					</div>
				</div>

			</div>

			<!-- Right Column: Sign In Card (Mobile & Desktop) -->
			<div class="lg:col-span-5 flex flex-col justify-between lg:justify-center items-center lg:items-end w-full flex-1">
				<div class="w-full flex flex-col items-center">
					<!-- Mobile Brand Header -->
					<div class="lg:hidden flex flex-col items-center text-center mb-4 pt-1">
						<BrandLogo size="md" />
					</div>

					<div class="wf-card w-full max-w-[440px] p-6 sm:p-9 shadow-sm sm:shadow-xs">
						
						<!-- Form Header (Centered on mobile) -->
						<div class="text-center lg:text-left">
							<h2 class="text-2xl font-medium text-slate-900 tracking-tight">Sign in</h2>
							<p class="text-slate-500 text-sm mt-1 font-normal">Sign in to your workspace.</p>
						</div>

						<!-- Error alert -->
						{#if error}
							<div class="wf-alert-error mt-5 flex items-start gap-2.5 p-3.5 text-xs leading-relaxed sm:text-sm">
								<ExclamationCircleIcon class="w-4 h-4 text-rose-500 shrink-0 mt-0.5" />
								<span>{error}</span>
							</div>
						{/if}

						<!-- Sign In Form -->
						<form onsubmit={handleLogin} class="mt-6 space-y-4">
							<!-- Identifier Input -->
							<div>
								<label for="identifier-input" class="block text-xs font-medium text-slate-700 mb-1.5">Email or username</label>
								<div class="relative">
									<div class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-400">
										<EnvelopeIcon class="w-4 h-4" />
									</div>
									<input
										type="text"
										id="identifier-input"
										bind:value={identifier}
										placeholder="you@company.com or acme-username"
										required
										disabled={loading}
										class="wf-input py-2.5 pl-10 pr-4 text-sm placeholder:text-slate-400"
									/>
								</div>
							</div>

							<!-- Password Input -->
							<div>
								<label for="password-input" class="block text-xs font-medium text-slate-700 mb-1.5">Password</label>
								<div class="relative">
									<div class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-400">
										<LockClosedIcon class="w-4 h-4" />
									</div>
									<input
										type={showPassword ? 'text' : 'password'}
										id="password-input"
										bind:value={password}
										placeholder="Enter your password"
										required
										disabled={loading}
										class="wf-input py-2.5 pl-10 pr-11 text-sm placeholder:text-slate-400"
									/>
									<button
										type="button"
										onclick={() => (showPassword = !showPassword)}
										class="absolute inset-y-0 right-0 pr-3.5 flex items-center text-slate-400 hover:text-slate-600 focus:outline-none cursor-pointer"
										aria-label={showPassword ? 'Hide password' : 'Show password'}
									>
										{#if showPassword}
											<EyeSlashIcon class="w-4 h-4" />
										{:else}
											<EyeIcon class="w-4 h-4" />
										{/if}
									</button>
								</div>
							</div>

							<!-- Submit Button -->
							<button
								type="submit"
								disabled={loading}
								class="wf-button-primary mt-2 w-full py-3 text-sm hover:shadow-sm"
							>
								{#if loading}
									<svg class="animate-spin h-4 w-4 text-white" viewBox="0 0 24 24" fill="none">
										<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
										<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
									</svg>
									<span>Signing in...</span>
								{:else}
									<span>Sign in</span>
								{/if}
							</button>
						</form>

						<!-- Bottom Create Account Link -->
						<div class="mt-8 text-center text-xs sm:text-sm text-slate-500">
							Do not have an account? <a href="/signup" class="text-blue-600 font-medium hover:text-blue-700 hover:underline transition-colors">Create account</a>
						</div>

					</div>
				</div>

			</div>

		</div>
	</div>

	<!-- Mobile Bottom Hero Illustration (Moved to the very bottom of the screen) -->
	<div class="lg:hidden w-[calc(100%+2rem)] -mx-4 mt-auto pointer-events-none select-none flex items-end justify-center overflow-hidden leading-none z-0">
		<img
			src={heroImage}
			alt="What Funnel dashboard illustration"
			class="w-full h-auto max-h-[260px] sm:max-h-[320px] object-cover object-bottom block"
			loading="eager"
		/>
	</div>

</div>
