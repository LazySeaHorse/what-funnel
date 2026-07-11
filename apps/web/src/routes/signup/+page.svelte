<script lang="ts">
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';

	let accountName = $state('');
	let email = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);

	async function handleSignup(e: Event) {
		e.preventDefault();
		loading = true;
		error = '';
		try {
			await apiRequest('/auth/signup', {
				method: 'POST',
				body: { account_name: accountName, email, password }
			});
			// Log in automatically after signup
			await apiRequest('/auth/login', {
				method: 'POST',
				body: { email, password }
			});
			goto('/inbox');
		} catch (err: any) {
			error = err.message;
		} finally {
			loading = false;
		}
	}
</script>

<div class="auth-container">
	<div class="glass-panel auth-card">
		<div class="auth-header">
			<h1>What Funnel</h1>
			<p>Create your new account</p>
		</div>

		<form onsubmit={handleSignup} style="display: flex; flex-direction: column; gap: 16px;">
			{#if error}
				<div style="padding: 10px; background: rgba(239, 68, 68, 0.1); border: 1px solid var(--danger); border-radius: 8px; color: var(--danger); font-size: 13px;">
					{error}
				</div>
			{/if}

			<div style="display: flex; flex-direction: column; gap: 6px;">
				<label for="accountName" style="font-size: 12px; font-weight: 500; color: var(--text-secondary);">Business Name</label>
				<input type="text" id="accountName" class="input-field" bind:value={accountName} placeholder="Acme Corp" required disabled={loading} />
			</div>

			<div style="display: flex; flex-direction: column; gap: 6px;">
				<label for="email" style="font-size: 12px; font-weight: 500; color: var(--text-secondary);">Email address</label>
				<input type="email" id="email" class="input-field" bind:value={email} placeholder="you@example.com" required disabled={loading} />
			</div>

			<div style="display: flex; flex-direction: column; gap: 6px;">
				<label for="password" style="font-size: 12px; font-weight: 500; color: var(--text-secondary);">Password</label>
				<input type="password" id="password" class="input-field" bind:value={password} placeholder="••••••••" required disabled={loading} />
			</div>

			<button type="submit" class="btn-primary" style="margin-top: 8px; height: 42px;" disabled={loading}>
				{loading ? 'Creating account...' : 'Create Account'}
			</button>
		</form>

		<div style="margin-top: 24px; text-align: center; font-size: 13px; color: var(--text-secondary);">
			Already have an account? <a href="/login" style="color: #6366f1; text-decoration: none; font-weight: 500;">Sign in</a>
		</div>
	</div>
</div>
