<script lang="ts">
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
	import heroImage from '$lib/assets/sign-in-hero.webp';
	import BrandLogo from '$lib/components/BrandLogo.svelte';

	let accountName = $state('');
	let email = $state('');
	let password = $state('');
	let productMode = $state('full_workspace');
	let showPassword = $state(false);
	let error = $state('');
	let loading = $state(false);
	let toastMessage = $state('');

	async function handleSignup(e: Event) {
		e.preventDefault();
		loading = true;
		error = '';
		try {
			await apiRequest('/auth/signup', {
				method: 'POST',
				body: { account_name: accountName, email, password, product_mode: productMode }
			});
			// Log in automatically after signup
			await apiRequest('/auth/login', {
				method: 'POST',
				body: { email, password }
			});
			goto('/onboarding');
		} catch (err: any) {
			error = err.message || 'Failed to create workspace. Please try again.';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Create Account — What Funnel</title>
</svelte:head>

<div class="wf-page min-h-screen flex flex-col justify-between p-4 pt-6 pb-0 sm:p-8 lg:p-12 selection:bg-blue-100 selection:text-blue-900 overflow-x-hidden">
	<div class="w-full max-w-[1360px] mx-auto relative flex-1 flex flex-col justify-between lg:justify-center">
		
		{#if toastMessage}
			<div class="fixed top-6 right-6 z-50 bg-slate-900 text-white text-xs sm:text-sm px-4 py-2.5 rounded-xl shadow-md flex items-center gap-2 transition-all">
				<svg class="w-4 h-4 text-blue-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
				</svg>
				<span>{toastMessage}</span>
				<button type="button" onclick={() => toastMessage = ''} class="ml-2 text-slate-400 hover:text-white" aria-label="Close notification">×</button>
			</div>
		{/if}

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
							All your conversations.<br />
							<span class="text-blue-600">Every lead.</span> One place.
						</h1>
						<p class="text-slate-500 text-sm sm:text-base font-normal leading-relaxed max-w-xl mt-3.5">
							Unify every channel, automate answers, and track leads from hello to happy customer.
						</p>
					</div>
				</div>

				<!-- 3D Hero Illustration -->
				<div class="relative w-full flex items-center justify-center mt-2 lg:mt-0 pointer-events-none">
					<img
						src={heroImage}
						alt="What Funnel Dashboard & Customer Experience in 3D"
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

			<!-- Right Column: Create Account Card (Mobile & Desktop) -->
			<div class="lg:col-span-5 flex flex-col justify-start lg:justify-center items-center lg:items-end w-full">
				<!-- Mobile Brand Header -->
				<div class="lg:hidden flex flex-col items-center text-center mb-4 pt-1">
					<BrandLogo size="md" />
				</div>

				<div class="wf-card w-full max-w-[460px] p-6 sm:p-9 shadow-sm sm:shadow-xs">
					
					<!-- Form Header (Centered on mobile) -->
					<div class="text-center lg:text-left">
						<h2 class="text-2xl font-medium text-slate-900 tracking-tight">Create workspace</h2>
						<p class="text-slate-500 text-sm mt-1 font-normal">Get started with your new account</p>
					</div>

					<!-- Error alert -->
					{#if error}
						<div class="wf-alert-error mt-5 flex items-start gap-2.5 p-3.5 text-xs leading-relaxed sm:text-sm">
							<svg class="w-4 h-4 text-rose-500 shrink-0 mt-0.5" viewBox="0 0 20 20" fill="currentColor">
								<path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z" clip-rule="evenodd" />
							</svg>
							<span>{error}</span>
						</div>
					{/if}

					<!-- Signup Form -->
					<form onsubmit={handleSignup} class="mt-6 space-y-4">
						<!-- Business Name Input -->
						<div>
							<label for="account-name-input" class="block text-xs font-medium text-slate-700 mb-1.5">Business Name</label>
							<div class="relative">
								<div class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-400">
									<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
										<rect x="2" y="7" width="20" height="14" rx="2" ry="2" />
										<path d="M16 21V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16" />
									</svg>
								</div>
								<input
									type="text"
									id="account-name-input"
									bind:value={accountName}
									placeholder="Acme Corp"
									required
									disabled={loading}
									class="wf-input py-2.5 pl-10 pr-4 text-sm placeholder:text-slate-400"
								/>
							</div>
						</div>

						<!-- Email Input -->
						<div>
							<label for="signup-email-input" class="block text-xs font-medium text-slate-700 mb-1.5">Email</label>
							<div class="relative">
								<div class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-400">
									<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
										<rect width="20" height="16" x="2" y="4" rx="2" />
										<path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7" />
									</svg>
								</div>
								<input
									type="email"
									id="signup-email-input"
									bind:value={email}
									placeholder="you@email.com"
									required
									disabled={loading}
									class="wf-input py-2.5 pl-10 pr-4 text-sm placeholder:text-slate-400"
								/>
							</div>
						</div>

						<!-- Password Input -->
						<div>
							<label for="signup-password-input" class="block text-xs font-medium text-slate-700 mb-1.5">Password</label>
							<div class="relative">
								<div class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-400">
									<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
										<rect width="18" height="11" x="3" y="11" rx="2" ry="2" />
										<path d="M7 11V7a5 5 0 0 1 10 0v4" />
									</svg>
								</div>
								<input
									type={showPassword ? 'text' : 'password'}
									id="signup-password-input"
									bind:value={password}
									placeholder="At least 8 characters"
									required
									minlength={8}
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

						<!-- Workspace Type Selection -->
						<div>
							<span class="block text-xs font-medium text-slate-700 mb-2">Workspace Type</span>
							<div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
								<label
									class="flex items-center gap-2.5 p-3 border rounded-xl cursor-pointer transition-all duration-150 {productMode === 'full_workspace' ? 'border-blue-600 bg-blue-50/50 ring-1 ring-blue-600' : 'border-slate-200 bg-white hover:bg-slate-50'}"
								>
									<input
										type="radio"
										name="product_mode"
										value="full_workspace"
										checked={productMode === 'full_workspace'}
										onchange={() => (productMode = 'full_workspace')}
										disabled={loading}
										class="sr-only"
									/>
									<div class="w-7 h-7 rounded-lg {productMode === 'full_workspace' ? 'bg-blue-600 text-white' : 'bg-slate-100 text-slate-500'} flex items-center justify-center shrink-0">
										<svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
											<rect x="3" y="3" width="18" height="18" rx="2" />
											<path d="M9 3v18" />
											<path d="M15 3v18" />
											<path d="M3 9h18" />
										</svg>
									</div>
									<div class="text-left">
										<div class="text-xs font-medium text-slate-800 leading-tight">Full Workspace</div>
										<div class="text-[11px] text-slate-400 leading-tight mt-0.5 font-normal">Inbox & leads</div>
									</div>
								</label>

								<label
									class="flex items-center gap-2.5 p-3 border rounded-xl cursor-pointer transition-all duration-150 {productMode === 'chatbot_only' ? 'border-blue-600 bg-blue-50/50 ring-1 ring-blue-600' : 'border-slate-200 bg-white hover:bg-slate-50'}"
								>
									<input
										type="radio"
										name="product_mode"
										value="chatbot_only"
										checked={productMode === 'chatbot_only'}
										onchange={() => (productMode = 'chatbot_only')}
										disabled={loading}
										class="sr-only"
									/>
									<div class="w-7 h-7 rounded-lg {productMode === 'chatbot_only' ? 'bg-blue-600 text-white' : 'bg-slate-100 text-slate-500'} flex items-center justify-center shrink-0">
										<svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
											<rect x="3" y="11" width="18" height="10" rx="3" />
											<circle cx="9" cy="16" r="1" fill="currentColor" />
											<circle cx="15" cy="16" r="1" fill="currentColor" />
											<path d="M12 7v4" />
										</svg>
									</div>
									<div class="text-left">
										<div class="text-xs font-medium text-slate-800 leading-tight">Chatbot Only</div>
										<div class="text-[11px] text-slate-400 leading-tight mt-0.5 font-normal">Automations</div>
									</div>
								</label>
							</div>
						</div>

						<!-- Submit Button -->
						<button
							type="submit"
							disabled={loading}
							class="wf-button-primary mt-4 w-full py-3 text-sm hover:shadow-sm"
						>
							{#if loading}
								<svg class="animate-spin h-4 w-4 text-white" viewBox="0 0 24 24" fill="none">
									<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
									<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
								</svg>
								<span>Creating workspace...</span>
							{:else}
								<span>Create Workspace</span>
							{/if}
						</button>
					</form>

					<!-- Bottom Sign In Link -->
					<div class="mt-8 text-center text-xs sm:text-sm text-slate-500">
						Already have an account? <a href="/login" class="text-blue-600 font-medium hover:text-blue-700 hover:underline transition-colors">Sign in</a>
					</div>

				</div>

			</div>

		</div>
	</div>

	<!-- Mobile Bottom Hero Illustration (Moved to the very bottom of the screen) -->
	<div class="lg:hidden w-[calc(100%+2rem)] -mx-4 mt-auto pointer-events-none select-none flex items-end justify-center overflow-hidden leading-none z-0">
		<img
			src={heroImage}
			alt="What Funnel Dashboard & Customer Experience in 3D"
			class="w-full h-auto max-h-[260px] sm:max-h-[320px] object-cover object-bottom block"
			loading="eager"
		/>
	</div>

</div>
