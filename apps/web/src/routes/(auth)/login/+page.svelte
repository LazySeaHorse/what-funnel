<script lang="ts">
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
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
