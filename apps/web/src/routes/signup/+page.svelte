<script lang="ts">
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
	import Icon from '$lib/Icon.svelte';

	let accountName = $state('');
	let email = $state('');
	let password = $state('');
	let productMode = $state('full_workspace');
	let error = $state('');
	let loading = $state(false);

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
			error = err.message;
		} finally {
			loading = false;
		}
	}
</script>

<div class="auth-container">
	<div class="auth-card" style="max-width: 440px;">
		<div class="auth-header">
			<div style="display: flex; justify-content: center; margin-bottom: 12px;">
				<div style="width: 40px; height: 40px; border-radius: 8px; background: var(--blue-bg); border: 1px solid var(--blue-border); display: flex; align-items: center; justify-content: center;">
					<Icon name="bot" size={22} color="var(--blue-text)" />
				</div>
			</div>
			<h1>What Funnel</h1>
			<p>Create your new workspace</p>
		</div>

		<form onsubmit={handleSignup} style="display: flex; flex-direction: column; gap: 16px;">
			{#if error}
				<div style="padding: 10px 14px; background: var(--danger-bg); border: 1px solid rgba(235, 87, 87, 0.3); border-radius: 6px; color: var(--danger); font-size: 13px;">
					{error}
				</div>
			{/if}

			<div style="display: flex; flex-direction: column; gap: 6px;">
				<label for="accountName" style="font-size: 12px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: 0.3px;">Business Name</label>
				<input type="text" id="accountName" class="input-field" bind:value={accountName} placeholder="Acme Corp" required disabled={loading} />
			</div>

			<div style="display: flex; flex-direction: column; gap: 6px;">
				<label for="email" style="font-size: 12px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: 0.3px;">Email address</label>
				<input type="email" id="email" class="input-field" bind:value={email} placeholder="you@example.com" required disabled={loading} />
			</div>

			<div style="display: flex; flex-direction: column; gap: 6px;">
				<label for="password" style="font-size: 12px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: 0.3px;">Password</label>
				<input type="password" id="password" class="input-field" bind:value={password} placeholder="••••••••" required disabled={loading} minlength={8} />
			</div>

			<div style="display: flex; flex-direction: column; gap: 8px;">
				<label style="font-size: 12px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: 0.3px;">Workspace Type</label>
				<div style="display: flex; flex-direction: column; gap: 8px;">
					<label
						style="display: flex; align-items: center; gap: 10px; padding: 10px 12px; border: 1px solid {productMode === 'chatbot_only' ? 'var(--blue-primary)' : 'var(--border-color)'}; background: {productMode === 'chatbot_only' ? 'var(--blue-bg)' : '#FFFFFF'}; border-radius: 6px; cursor: pointer; transition: all 0.15s ease;"
					>
						<input type="radio" name="product_mode" value="chatbot_only" checked={productMode === 'chatbot_only'} onchange={() => productMode = 'chatbot_only'} disabled={loading} />
						<Icon name="bot" size={16} color={productMode === 'chatbot_only' ? 'var(--blue-text)' : 'var(--text-secondary)'} />
						<span style="font-size: 13px; font-weight: 500; color: var(--text-primary);">Automated replies only</span>
					</label>
					<label
						style="display: flex; align-items: center; gap: 10px; padding: 10px 12px; border: 1px solid {productMode === 'full_workspace' ? 'var(--blue-primary)' : 'var(--border-color)'}; background: {productMode === 'full_workspace' ? 'var(--blue-bg)' : '#FFFFFF'}; border-radius: 6px; cursor: pointer; transition: all 0.15s ease;"
					>
						<input type="radio" name="product_mode" value="full_workspace" checked={productMode === 'full_workspace'} onchange={() => productMode = 'full_workspace'} disabled={loading} />
						<Icon name="layout" size={16} color={productMode === 'full_workspace' ? 'var(--blue-text)' : 'var(--text-secondary)'} />
						<span style="font-size: 13px; font-weight: 500; color: var(--text-primary);">Full lead workspace</span>
					</label>
				</div>
			</div>

			<button type="submit" class="btn-primary" style="margin-top: 6px; height: 40px;" disabled={loading}>
				{#if loading}
					Creating account...
				{:else}
					Create Workspace
					<Icon name="arrow-right" size={16} />
				{/if}
			</button>
		</form>

		<div style="margin-top: 24px; text-align: center; font-size: 13px; color: var(--text-secondary);">
			Already have an account? <a href="/login" style="color: var(--blue-text); text-decoration: none; font-weight: 500;">Sign in</a>
		</div>
	</div>
</div>
