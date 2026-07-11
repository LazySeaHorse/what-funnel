<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';

	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let successMsg = $state('');
	let currentUser = $state<any | null>(null);
	
	let settings = $state({
		lead_tracking_enabled: true,
		unassigned_conversations_visible_to_members: true
	});

	onMount(async () => {
		try {
			currentUser = await apiRequest('/auth/me');
			if (currentUser.role !== 'admin') {
				goto('/inbox');
				return;
			}
			await loadAccountSettings();
		} catch (err) {
			goto('/login');
		} finally {
			loading = false;
		}
	});

	async function loadAccountSettings() {
		try {
			const account = await apiRequest('/workspace/account');
			if (account && account.settings) {
				try {
					const decoded = atob(account.settings);
					const parsed = JSON.parse(decoded);
					
					// Set defaults if null
					settings.lead_tracking_enabled = parsed.lead_tracking_enabled !== false;
					settings.unassigned_conversations_visible_to_members = parsed.unassigned_conversations_visible_to_members !== false;
				} catch (e) {
					console.error('Failed to parse settings JSON', e);
				}
			}
		} catch (err: any) {
			error = 'Failed to load account settings: ' + err.message;
		}
	}

	async function handleSaveSettings() {
		error = '';
		successMsg = '';
		saving = true;

		try {
			await apiRequest('/workspace/account/settings', {
				method: 'PUT',
				body: settings
			});
			successMsg = 'Settings saved successfully.';
		} catch (err: any) {
			error = 'Failed to save settings: ' + err.message;
		} finally {
			saving = false;
		}
	}
</script>

<div class="settings-container">
	<div class="settings-sidebar glass-panel">
		<h2 class="sidebar-title">Settings</h2>
		<nav class="sidebar-nav">
			<a href="/inbox" class="nav-item">← Back to Inbox</a>
			<a href="/settings/account" class="nav-item active">Account Settings</a>
			<a href="/settings/channels" class="nav-item">Channels</a>
			<a href="/settings/users" class="nav-item">Workspace Users</a>
			<a href="/settings/pipeline" class="nav-item">Lead Pipeline</a>
		</nav>
	</div>

	<div class="settings-content glass-panel">
		<div class="content-header">
			<div>
				<h1>Account Settings</h1>
				<p class="subtitle">Configure general business features and member access permissions</p>
			</div>
		</div>

		{#if error}
			<div class="banner error">{error}</div>
		{/if}

		{#if successMsg}
			<div class="banner success">{successMsg}</div>
		{/if}

		{#if loading}
			<div class="loading-state">Loading settings...</div>
		{:else}
			<div class="settings-form-container">
				<div class="settings-card glass-panel">
					<h3>Lead Tracking</h3>
					<p class="card-desc">Enable lead stages, tagging, and notes in conversation threads to manage your sales funnel.</p>
					
					<div class="toggle-container">
						<label class="switch">
							<input 
								type="checkbox" 
								bind:checked={settings.lead_tracking_enabled}
								disabled={saving}
							/>
							<span class="slider round"></span>
						</label>
						<span class="toggle-label">
							{settings.lead_tracking_enabled ? 'Enabled' : 'Disabled'}
						</span>
					</div>
				</div>

				<div class="settings-card glass-panel">
					<h3>Member Visibility</h3>
					<p class="card-desc">Allow workspace members (non-admins) to see and reply to unassigned customer conversations.</p>
					
					<div class="toggle-container">
						<label class="switch">
							<input 
								type="checkbox" 
								bind:checked={settings.unassigned_conversations_visible_to_members}
								disabled={saving}
							/>
							<span class="slider round"></span>
						</label>
						<span class="toggle-label">
							{settings.unassigned_conversations_visible_to_members ? 'Visible to Members' : 'Hidden from Members'}
						</span>
					</div>
				</div>

				<div class="action-bar">
					<button 
						type="button" 
						class="btn-primary" 
						onclick={handleSaveSettings}
						disabled={saving}
					>
						{saving ? 'Saving...' : 'Save Settings'}
					</button>
				</div>
			</div>
		{/if}
	</div>
</div>

<style>
	.settings-container {
		display: flex;
		gap: 24px;
		max-width: 1200px;
		margin: 40px auto;
		padding: 0 24px;
		height: calc(100vh - 80px);
	}

	.settings-sidebar {
		width: 280px;
		padding: 24px;
		display: flex;
		flex-direction: column;
		gap: 20px;
	}

	.sidebar-title {
		font-size: 18px;
		font-weight: 600;
		color: var(--text-primary);
		letter-spacing: 0.5px;
	}

	.sidebar-nav {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.nav-item {
		padding: 10px 14px;
		border-radius: 8px;
		color: var(--text-secondary);
		text-decoration: none;
		font-size: 14px;
		font-weight: 500;
		transition: background-color 0.2s, color 0.2s;
	}

	.nav-item:hover {
		background: rgba(255, 255, 255, 0.04);
		color: var(--text-primary);
	}

	.nav-item.active {
		background: rgba(var(--primary-rgb), 0.15);
		color: #fff;
		border: 1px solid rgba(var(--primary-rgb), 0.3);
	}

	.settings-content {
		flex: 1;
		padding: 32px;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: 24px;
	}

	.content-header h1 {
		font-size: 24px;
		font-weight: 700;
		margin-bottom: 4px;
	}

	.subtitle {
		color: var(--text-secondary);
		font-size: 14px;
	}

	.loading-state {
		color: var(--text-secondary);
		font-size: 14px;
		text-align: center;
		padding: 40px 0;
	}

	.banner {
		padding: 12px 16px;
		border-radius: 8px;
		font-size: 14px;
	}

	.banner.error {
		background: rgba(239, 68, 68, 0.15);
		border: 1px solid rgba(239, 68, 68, 0.3);
		color: #fca5a5;
	}

	.banner.success {
		background: rgba(34, 197, 94, 0.15);
		border: 1px solid rgba(34, 197, 94, 0.3);
		color: #86efac;
	}

	.settings-form-container {
		display: flex;
		flex-direction: column;
		gap: 20px;
	}

	.settings-card {
		padding: 24px;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.settings-card h3 {
		font-size: 16px;
		font-weight: 600;
		color: var(--text-primary);
	}

	.card-desc {
		font-size: 14px;
		color: var(--text-secondary);
		margin-bottom: 12px;
	}

	.toggle-container {
		display: flex;
		align-items: center;
		gap: 16px;
	}

	.toggle-label {
		font-size: 14px;
		font-weight: 500;
		color: var(--text-primary);
	}

	/* Toggle Switch Styles */
	.switch {
		position: relative;
		display: inline-block;
		width: 48px;
		height: 24px;
	}

	.switch input {
		opacity: 0;
		width: 0;
		height: 0;
	}

	.slider {
		position: absolute;
		cursor: pointer;
		top: 0;
		left: 0;
		right: 0;
		bottom: 0;
		background-color: rgba(255, 255, 255, 0.1);
		transition: .3s;
		border: 1px solid var(--border-color);
	}

	.slider:before {
		position: absolute;
		content: "";
		height: 16px;
		width: 16px;
		left: 3px;
		bottom: 3px;
		background-color: white;
		transition: .3s;
	}

	input:checked + .slider {
		background-color: rgb(var(--primary-rgb));
		border-color: rgba(var(--primary-rgb), 0.5);
	}

	input:checked + .slider:before {
		transform: translateX(24px);
	}

	.slider.round {
		border-radius: 24px;
	}

	.slider.round:before {
		border-radius: 50%;
	}

	.action-bar {
		margin-top: 10px;
		display: flex;
		justify-content: flex-start;
	}
</style>
