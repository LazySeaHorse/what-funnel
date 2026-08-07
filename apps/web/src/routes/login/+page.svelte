<script lang="ts">
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
	import Icon from '$lib/Icon.svelte';

	let email = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);

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
			error = err.message;
		} finally {
			loading = false;
		}
	}
</script>

<div class="auth-container">
	<div class="auth-card">
		<div class="auth-header">
			<div style="display: flex; justify-content: center; margin-bottom: 12px;">
				<div style="width: 40px; height: 40px; border-radius: 8px; background: var(--blue-bg); border: 1px solid var(--blue-border); display: flex; align-items: center; justify-content: center;">
					<Icon name="bot" size={22} color="var(--blue-text)" />
				</div>
			</div>
			<h1>What Funnel</h1>
			<p>Sign in to your workspace</p>
		</div>

		<form onsubmit={handleLogin} style="display: flex; flex-direction: column; gap: 16px;">
			{#if error}
				<div style="padding: 10px 14px; background: var(--danger-bg); border: 1px solid rgba(235, 87, 87, 0.3); border-radius: 6px; color: var(--danger); font-size: 13px;">
					{error}
				</div>
			{/if}

			<div style="display: flex; flex-direction: column; gap: 6px;">
				<label for="email" style="font-size: 12px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: 0.3px;">Email address</label>
				<input type="email" id="email" class="input-field" bind:value={email} placeholder="you@example.com" required disabled={loading} />
			</div>

			<div style="display: flex; flex-direction: column; gap: 6px;">
				<label for="password" style="font-size: 12px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: 0.3px;">Password</label>
				<input type="password" id="password" class="input-field" bind:value={password} placeholder="••••••••" required disabled={loading} />
			</div>

			<button type="submit" class="btn-primary" style="margin-top: 6px; height: 40px;" disabled={loading}>
				{#if loading}
					Signing in...
				{:else}
					Sign In
					<Icon name="arrow-right" size={16} />
				{/if}
			</button>
		</form>

		<div style="margin-top: 24px; text-align: center; font-size: 13px; color: var(--text-secondary);">
			Don't have an account? <a href="/signup" style="color: var(--blue-text); text-decoration: none; font-weight: 500;">Create one</a>
		</div>
	</div>
</div>
