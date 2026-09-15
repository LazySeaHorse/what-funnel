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
		InformationCircleIcon
	} from '@fvilers/heroicons-svelte/24/outline';
	import { Button, Input } from '$lib/components/ui';
	import WorkspaceTypeSelector from '$lib/components/WorkspaceTypeSelector.svelte';

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
			await goto('/onboarding');
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
						<Input
							id="account-name-input"
							label="Business name"
							type="text"
							bind:value={accountName}
							placeholder="Acme Corp"
							required
							disabled={loading}
							class="text-sm"
						>
							{#snippet leading()}
								<BuildingOffice2Icon class="w-4 h-4" />
							{/snippet}
						</Input>

						<!-- Email Input -->
						<Input
							id="signup-email-input"
							label="Email"
							type="email"
							bind:value={email}
							placeholder="you@email.com"
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
							id="signup-password-input"
							label="Password"
							type={showPassword ? 'text' : 'password'}
							bind:value={password}
							placeholder="At least 8 characters"
							required
							minlength={8}
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

						<!-- Workspace Type Selection -->
						<div>
							<span class="block text-xs font-medium text-slate-700 mb-2">Workspace type</span>
							<WorkspaceTypeSelector bind:value={productMode} disabled={loading} />
						</div>

						<!-- Submit Button -->
						<Button
							type="submit"
							variant="primary"
							size="lg"
							busy={loading}
							class="mt-4 w-full py-3 text-sm hover:shadow-sm"
						>
							{loading ? 'Creating workspace...' : 'Create workspace'}
						</Button>
					</form>

					<!-- Bottom Sign In Link -->
					<div class="mt-8 text-center text-xs sm:text-sm text-slate-500">
						Already have an account? <a href="/login" class="text-blue-600 font-medium hover:text-blue-700 hover:underline transition-colors">Sign in</a>
					</div>

</div>
