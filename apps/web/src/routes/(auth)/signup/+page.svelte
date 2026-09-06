<script lang="ts">
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
	import {
		BuildingOffice2Icon,
		EnvelopeIcon,
		LockClosedIcon,
		EyeIcon,
		EyeSlashIcon,
		ExclamationCircleIcon,
		InformationCircleIcon,
		ViewColumnsIcon,
		CpuChipIcon
	} from '@fvilers/heroicons-svelte/24/outline';

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
	<title>Create account — What Funnel</title>
</svelte:head>

{#if toastMessage}
	<div class="fixed top-6 right-6 z-50 bg-slate-900 text-white text-xs sm:text-sm px-4 py-2.5 rounded-xl shadow-md flex items-center gap-2 transition-all">
		<InformationCircleIcon class="w-4 h-4 text-blue-400 shrink-0" />
		<span>{toastMessage}</span>
		<button type="button" onclick={() => toastMessage = ''} class="ml-2 text-slate-400 hover:text-white" aria-label="Close notification">×</button>
	</div>
{/if}

<div class="wf-card w-full max-w-[460px] p-6 sm:p-9 shadow-sm sm:shadow-xs">
					
					<!-- Form Header (Centered on mobile) -->
					<div class="text-center lg:text-left">
						<h2 class="text-2xl font-medium text-slate-900 tracking-tight">Create workspace</h2>
						<p class="text-slate-500 text-sm mt-1 font-normal">Enter details to create your workspace.</p>
					</div>

					<!-- Error alert -->
					{#if error}
						<div class="wf-alert-error mt-5 flex items-start gap-2.5 p-3.5 text-xs leading-relaxed sm:text-sm">
							<ExclamationCircleIcon class="w-4 h-4 text-rose-500 shrink-0 mt-0.5" />
							<span>{error}</span>
						</div>
					{/if}

					<!-- Signup Form -->
					<form onsubmit={handleSignup} class="mt-6 space-y-4">
						<!-- Business Name Input -->
						<div>
							<label for="account-name-input" class="block text-xs font-medium text-slate-700 mb-1.5">Business name</label>
							<div class="relative">
								<div class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-400">
									<BuildingOffice2Icon class="w-4 h-4" />
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
									<EnvelopeIcon class="w-4 h-4" />
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
									<LockClosedIcon class="w-4 h-4" />
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
										<EyeSlashIcon class="w-4 h-4" />
									{:else}
										<EyeIcon class="w-4 h-4" />
									{/if}
								</button>
							</div>
						</div>

						<!-- Workspace Type Selection -->
						<div>
							<span class="block text-xs font-medium text-slate-700 mb-2">Workspace type</span>
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
										<ViewColumnsIcon class="w-3.5 h-3.5" />
									</div>
									<div class="text-left">
										<div class="text-xs font-medium text-slate-800 leading-tight">Full workspace</div>
										<div class="text-[11px] text-slate-400 leading-tight mt-0.5 font-normal">Inbox and leads</div>
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
										<CpuChipIcon class="w-3.5 h-3.5" />
									</div>
									<div class="text-left">
										<div class="text-xs font-medium text-slate-800 leading-tight">Chatbot only</div>
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
								<span>Create workspace</span>
							{/if}
						</button>
					</form>

					<!-- Bottom Sign In Link -->
					<div class="mt-8 text-center text-xs sm:text-sm text-slate-500">
						Already have an account? <a href="/login" class="text-blue-600 font-medium hover:text-blue-700 hover:underline transition-colors">Sign in</a>
					</div>

</div>
