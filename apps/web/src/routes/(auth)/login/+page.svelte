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
	import { Button, Input } from '$lib/components/ui';

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
			await goto('/inbox');
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
							<Input
								id="identifier-input"
								label="Email or username"
								type="text"
								bind:value={identifier}
								placeholder="you@company.com or acme-username"
								required
								disabled={loading}
								class="text-sm"
							>
								{#snippet leading()}
									<EnvelopeIcon class="w-4 h-4" />
								{/snippet}
							</Input>

							<!-- Password Input -->
							<Input
								id="password-input"
								label="Password"
								type={showPassword ? 'text' : 'password'}
								bind:value={password}
								placeholder="Enter your password"
								required
								disabled={loading}
								class="text-sm"
							>
								{#snippet leading()}
									<LockClosedIcon class="w-4 h-4" />
								{/snippet}
								{#snippet trailing()}
									<button
										type="button"
										onclick={() => (showPassword = !showPassword)}
										class="flex items-center text-slate-400 hover:text-slate-600 focus:outline-none cursor-pointer"
										aria-label={showPassword ? 'Hide password' : 'Show password'}
									>
										{#if showPassword}
											<EyeSlashIcon class="w-4 h-4" />
										{:else}
											<EyeIcon class="w-4 h-4" />
										{/if}
									</button>
								{/snippet}
							</Input>

							<!-- Submit Button -->
							<Button
								type="submit"
								variant="primary"
								size="lg"
								busy={loading}
								class="mt-2 w-full py-3 text-sm hover:shadow-sm"
							>
								{loading ? 'Signing in...' : 'Sign in'}
							</Button>
						</form>

						<!-- Bottom Create Account Link -->
						<div class="mt-8 text-center text-xs sm:text-sm text-slate-500">
							Do not have an account? <a href="/signup" class="text-blue-600 font-medium hover:text-blue-700 hover:underline transition-colors">Create account</a>
						</div>

</div>
