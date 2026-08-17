<script lang="ts">
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
	import heroImage from '$lib/assets/sign-in-hero.webp';

	let email = $state('');
	let password = $state('');
	let rememberMe = $state(false);
	let showPassword = $state(false);
	let error = $state('');
	let loading = $state(false);
	let toastMessage = $state('');

	async function handleLogin(e: Event) {
		e.preventDefault();
		loading = true;
		error = '';
		try {
			await apiRequest('/auth/login', {
				method: 'POST',
				body: { email, password }
			});
			goto('/inbox');
		} catch (err: any) {
			error = err.message || 'Invalid email or password. Please try again.';
		} finally {
			loading = false;
		}
	}

	function handleSocialLogin(provider: string) {
		toastMessage = `${provider} authentication is coming soon. Please sign in with your email and password.`;
		setTimeout(() => {
			toastMessage = '';
		}, 4000);
	}
</script>

<svelte:head>
	<title>Sign In — What Funnel</title>
</svelte:head>

<div class="min-h-screen w-full bg-[#F8F9FD] flex items-center justify-center p-4 sm:p-8 lg:p-12 font-sans antialiased text-slate-800 selection:bg-blue-100 selection:text-blue-900">
	<div class="w-full max-w-[1360px] mx-auto relative">
		
		{#if toastMessage}
			<div class="fixed top-6 right-6 z-50 bg-slate-900 text-white text-xs sm:text-sm px-4 py-2.5 rounded-xl shadow-lg flex items-center gap-2 transition-all animate-fade-in">
				<svg class="w-4 h-4 text-blue-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
				</svg>
				<span>{toastMessage}</span>
				<button type="button" onclick={() => toastMessage = ''} class="ml-2 text-slate-400 hover:text-white" aria-label="Close notification">×</button>
			</div>
		{/if}

		<div class="grid grid-cols-1 lg:grid-cols-12 gap-8 lg:gap-12 items-center">
			
			<!-- Left Column: Brand, Headline, 3D Hero Image -->
			<div class="lg:col-span-7 flex flex-col justify-between h-full relative">
				
				<!-- Top Content (Logo, Brand Name, Dots, Headline, Subhead) -->
				<div class="pt-10 sm:pt-14 lg:pt-16">
					<!-- Top Bar: Brand Logo & Decorative Dots -->
					<div class="flex items-center justify-between">
						<div class="flex items-center gap-3">
							<!-- What Funnel Platform Logo (matching dashboard) -->
							<svg class="w-9 h-9 shrink-0" viewBox="0 0 36 36" fill="none">
								<rect width="36" height="36" rx="10" fill="#4F80FF" />
								<circle cx="14" cy="14" r="3" fill="white" />
								<circle cx="22" cy="18" r="4.5" fill="white" />
								<circle cx="14" cy="23" r="2.5" fill="white" />
							</svg>
							<span class="text-2xl font-bold text-[#0F172A] tracking-tight">what funnel</span>
						</div>

						<!-- Decorative 4x3 Dot Matrix (matching Figma mock) -->
						<div class="hidden sm:grid grid-cols-4 gap-2 opacity-50 pr-4">
							<div class="w-1.5 h-1.5 rounded-full bg-[#4F6BFF]"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-[#4F6BFF]"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-[#4F6BFF]"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-[#4F6BFF]"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-[#4F6BFF]"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-[#4F6BFF]"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-[#4F6BFF]"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-[#4F6BFF]"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-[#4F6BFF]"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-[#4F6BFF]"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-[#4F6BFF]"></div>
							<div class="w-1.5 h-1.5 rounded-full bg-[#4F6BFF]"></div>
						</div>
					</div>

					<!-- Hero Headline and Subhead -->
					<div class="mt-6 sm:mt-8">
						<h1 class="text-3xl sm:text-4xl lg:text-[48px] font-medium text-[#0F172A] tracking-tight leading-[1.12]">
							All your conversations.<br />
							<span class="text-[#4F6BFF]">Every lead.</span> One place.
						</h1>
						<p class="text-slate-500 text-sm sm:text-base lg:text-[17px] font-normal leading-relaxed max-w-xl mt-3.5">
							Unify every channel, automate answers, and track leads from hello to happy customer.
						</p>
					</div>
				</div>

				<!-- 3D Hero Illustration (blends seamlessly with #F8F9FD background) -->
				<div class="relative w-full flex items-center justify-center mt-2 lg:mt-0 select-none pointer-events-none">
					<img
						src={heroImage}
						alt="What Funnel Dashboard & Customer Experience in 3D"
						class="w-full max-h-[590px] lg:max-h-[650px] object-contain"
						loading="eager"
					/>
					
					<!-- Decorative 2x2 Green Dots (matching bottom left of mockup) -->
					<div class="absolute bottom-4 left-2 grid grid-cols-2 gap-1.5 opacity-60">
						<div class="w-1.5 h-1.5 rounded-full bg-[#22C55E]"></div>
						<div class="w-1.5 h-1.5 rounded-full bg-[#22C55E]"></div>
						<div class="w-1.5 h-1.5 rounded-full bg-[#22C55E]"></div>
						<div class="w-1.5 h-1.5 rounded-full bg-[#22C55E]"></div>
					</div>
				</div>

			</div>

			<!-- Right Column: Sign In Card matching mockup -->
			<div class="lg:col-span-5 flex justify-center lg:justify-end items-center">
				<div class="w-full max-w-[440px] bg-white rounded-3xl border border-slate-200/90 shadow-[0_15px_45px_rgba(0,0,0,0.04)] p-7 sm:p-9">
					
					<!-- Form Header -->
					<div>
						<h2 class="text-2xl sm:text-[26px] font-bold text-[#0F172A] tracking-tight">Welcome back</h2>
						<p class="text-slate-500 text-sm mt-1">Sign in to continue to your workspace</p>
					</div>

					<!-- Error alert -->
					{#if error}
						<div class="mt-5 p-3.5 bg-rose-50 border border-rose-200 rounded-xl text-rose-700 text-xs sm:text-sm flex items-start gap-2.5 leading-relaxed">
							<svg class="w-4 h-4 text-rose-500 shrink-0 mt-0.5" viewBox="0 0 20 20" fill="currentColor">
								<path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z" clip-rule="evenodd" />
							</svg>
							<span>{error}</span>
						</div>
					{/if}

					<!-- Sign In Form -->
					<form onsubmit={handleLogin} class="mt-6 space-y-4">
						<!-- Email Input -->
						<div>
							<label for="email-input" class="block text-xs font-semibold text-slate-700 mb-1.5">Email</label>
							<div class="relative">
								<div class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-400">
									<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
										<rect width="20" height="16" x="2" y="4" rx="2" />
										<path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7" />
									</svg>
								</div>
								<input
									type="email"
									id="email-input"
									bind:value={email}
									placeholder="you@email.com"
									required
									disabled={loading}
									class="w-full pl-10 pr-4 py-3 bg-white border border-slate-200 rounded-xl text-sm text-slate-900 placeholder:text-slate-400 focus:border-[#4F6BFF] focus:ring-4 focus:ring-[#4F6BFF]/10 outline-none transition-all"
								/>
							</div>
						</div>

						<!-- Password Input -->
						<div>
							<label for="password-input" class="block text-xs font-semibold text-slate-700 mb-1.5">Password</label>
							<div class="relative">
								<div class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-400">
									<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
										<rect width="18" height="11" x="3" y="11" rx="2" ry="2" />
										<path d="M7 11V7a5 5 0 0 1 10 0v4" />
									</svg>
								</div>
								<input
									type={showPassword ? 'text' : 'password'}
									id="password-input"
									bind:value={password}
									placeholder="Enter your password"
									required
									disabled={loading}
									class="w-full pl-10 pr-11 py-3 bg-white border border-slate-200 rounded-xl text-sm text-slate-900 placeholder:text-slate-400 focus:border-[#4F6BFF] focus:ring-4 focus:ring-[#4F6BFF]/10 outline-none transition-all"
								/>
								<button
									type="button"
									onclick={() => (showPassword = !showPassword)}
									class="absolute inset-y-0 right-0 pr-3.5 flex items-center text-slate-400 hover:text-slate-600 focus:outline-none cursor-pointer"
									aria-label={showPassword ? 'Hide password' : 'Show password'}
								>
									{#if showPassword}
										<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
											<path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
											<line x1="1" y1="1" x2="23" y2="23" />
										</svg>
									{:else}
										<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
											<path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z" />
											<circle cx="12" cy="12" r="3" />
										</svg>
									{/if}
								</button>
							</div>
						</div>

						<!-- Remember Me & Forgot Password -->
						<div class="flex items-center justify-between pt-1">
							<label class="flex items-center gap-2 cursor-pointer select-none">
								<input
									type="checkbox"
									bind:checked={rememberMe}
									class="w-4 h-4 rounded border-slate-300 text-[#4F6BFF] focus:ring-[#4F6BFF]/20 accent-[#4F6BFF] cursor-pointer"
								/>
								<span class="text-xs sm:text-sm text-slate-500">Remember me</span>
							</label>
							<a href="#forgot" onclick={(e) => { e.preventDefault(); toastMessage = 'Password reset instructions have been dispatched to your email.'; }} class="text-xs sm:text-sm font-medium text-[#4F6BFF] hover:text-[#3D5AE8] hover:underline transition-colors">
								Forgot password?
							</a>
						</div>

						<!-- Submit Button -->
						<button
							type="submit"
							disabled={loading}
							class="w-full mt-2 py-3.5 px-4 bg-[#4F6BFF] hover:bg-[#3D5AE8] active:bg-[#254EDB] text-white font-semibold rounded-xl text-sm shadow-md shadow-[#4F6BFF]/25 hover:shadow-lg hover:shadow-[#4F6BFF]/35 active:scale-[0.99] transition-all duration-150 cursor-pointer disabled:opacity-60 disabled:cursor-not-allowed flex items-center justify-center gap-2"
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

					<!-- Divider -->
					<div class="relative flex py-5 items-center">
						<div class="flex-grow border-t border-slate-200"></div>
						<span class="flex-shrink mx-3 text-xs text-slate-400 font-normal">or continue with</span>
						<div class="flex-grow border-t border-slate-200"></div>
					</div>

					<!-- Social Login Buttons -->
					<div class="space-y-3">
						<!-- Continue with Google -->
						<button
							type="button"
							onclick={() => handleSocialLogin('Google')}
							class="w-full py-3 px-4 rounded-xl border border-slate-200 hover:border-slate-300 bg-white hover:bg-slate-50 active:bg-slate-100 text-slate-700 font-medium text-sm flex items-center justify-center gap-3 transition-all duration-150 cursor-pointer shadow-xs active:scale-[0.99]"
						>
							<svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24">
								<path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/>
								<path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
								<path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.06H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.94l2.85-2.22.81-.63z"/>
								<path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.06l3.66 2.84c.87-2.6 3.3-4.52 6.16-4.52z"/>
							</svg>
							<span>Continue with Google</span>
						</button>

						<!-- Continue with Microsoft -->
						<button
							type="button"
							onclick={() => handleSocialLogin('Microsoft')}
							class="w-full py-3 px-4 rounded-xl border border-slate-200 hover:border-slate-300 bg-white hover:bg-slate-50 active:bg-slate-100 text-slate-700 font-medium text-sm flex items-center justify-center gap-3 transition-all duration-150 cursor-pointer shadow-xs active:scale-[0.99]"
						>
							<svg class="w-4 h-4 shrink-0" viewBox="0 0 21 21">
								<rect x="1" y="1" width="9" height="9" fill="#f25022"/>
								<rect x="11" y="1" width="9" height="9" fill="#7fba00"/>
								<rect x="1" y="11" width="9" height="9" fill="#00a4ef"/>
								<rect x="11" y="11" width="9" height="9" fill="#ffb900"/>
							</svg>
							<span>Continue with Microsoft</span>
						</button>
					</div>

					<!-- Bottom Create Account Link -->
					<div class="mt-8 text-center text-xs sm:text-sm text-slate-500">
						Don't have an account? <a href="/signup" class="text-[#4F6BFF] font-semibold hover:text-[#3D5AE8] hover:underline transition-colors">Create account</a>
					</div>

				</div>
			</div>

		</div>
	</div>
</div>
