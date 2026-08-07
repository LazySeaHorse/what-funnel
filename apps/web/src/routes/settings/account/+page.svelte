<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
	import Icon from '$lib/Icon.svelte';

	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let successMsg = $state('');
	let currentUser = $state<any | null>(null);
	
	let settings = $state({
		lead_tracking_enabled: true,
		unassigned_conversations_visible_to_members: true
	});
	let productMode = $state('full_workspace');

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
			if (account) {
				productMode = account.product_mode || 'full_workspace';
				if (account.settings) {
					try {
						const decoded = atob(account.settings);
						const parsed = JSON.parse(decoded);
						
						settings.lead_tracking_enabled = parsed.lead_tracking_enabled !== false;
						settings.unassigned_conversations_visible_to_members = parsed.unassigned_conversations_visible_to_members !== false;
					} catch (e) {
						console.error('Failed to parse settings JSON', e);
					}
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

	async function handleSwitchMode(newMode: string) {
		error = '';
		successMsg = '';
		saving = true;

		try {
			await apiRequest('/workspace/account/product-mode', {
				method: 'PATCH',
				body: { product_mode: newMode }
			});
			productMode = newMode;
			await loadAccountSettings();
			successMsg = 'Product mode updated successfully.';
		} catch (err: any) {
			error = 'Failed to switch product mode: ' + err.message;
		} finally {
			saving = false;
		}
	}
</script>

<div class="settings-container">
	<div class="settings-sidebar glass-panel">
		<h2 class="sidebar-title">Settings</h2>
		<nav class="sidebar-nav">
			<a href="/inbox" class="nav-item back-item">
				<Icon name="arrow-left" size={14} /> Back to Inbox
			</a>
			<a href="/settings/account" class="nav-item active">
				<Icon name="settings" size={14} /> Account Settings
			</a>
			<a href="/settings/channels" class="nav-item">
				<Icon name="channels" size={14} /> Channels
			</a>
			<a href="/settings/users" class="nav-item">
				<Icon name="users" size={14} /> Workspace Users
			</a>
			{#if productMode !== 'chatbot_only'}
				<a href="/settings/pipeline" class="nav-item">
					<Icon name="pipeline" size={14} /> Lead Pipeline
				</a>
			{/if}
			<a href="/settings/knowledge-base" class="nav-item">
				<Icon name="kb" size={14} /> Knowledge Base
			</a>
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
					<h3>Product Mode</h3>
					<p class="card-desc">Choose between automating customer replies or managing your full sales pipeline.</p>
					
					<div class="radio-group">
						<label class="mode-option" class:selected={productMode === 'chatbot_only'}>
							<input 
								type="radio" 
								name="product_mode" 
								value="chatbot_only" 
								checked={productMode === 'chatbot_only'} 
								onchange={() => handleSwitchMode('chatbot_only')}
								disabled={saving}
							/>
							<Icon name="bot" size={16} color={productMode === 'chatbot_only' ? 'var(--blue-text)' : 'var(--text-secondary)'} />
							<span>Automated replies only (Chatbot-only)</span>
						</label>
						<label class="mode-option" class:selected={productMode === 'full_workspace'}>
							<input 
								type="radio" 
								name="product_mode" 
								value="full_workspace" 
								checked={productMode === 'full_workspace'} 
								onchange={() => handleSwitchMode('full_workspace')}
								disabled={saving}
							/>
							<Icon name="layout" size={16} color={productMode === 'full_workspace' ? 'var(--blue-text)' : 'var(--text-secondary)'} />
							<span>Full lead workspace</span>
						</label>
					</div>
				</div>

				{#if productMode === 'full_workspace'}
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
								{#if settings.lead_tracking_enabled}
									<span class="badge-blue">Enabled</span>
								{:else}
									<span class="badge-yellow">Disabled</span>
								{/if}
							</span>
						</div>
					</div>
				{/if}

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
							{#if settings.unassigned_conversations_visible_to_members}
								<span class="badge-blue">Visible to Members</span>
							{:else}
								<span class="badge-pink">Hidden from Members</span>
							{/if}
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
		gap: 20px;
		max-width: 1100px;
		margin: 24px auto;
		padding: 0 16px;
		height: calc(100vh - 48px);
	}

	.settings-sidebar {
		width: 240px;
		padding: 20px;
		display: flex;
		flex-direction: column;
		gap: 16px;
		background: var(--bg-sidebar);
		height: 100%;
	}

	.sidebar-title {
		font-size: 16px;
		font-weight: 700;
		color: var(--text-primary);
	}

	.sidebar-nav {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.nav-item {
		padding: 8px 12px;
		border-radius: 6px;
		color: var(--text-secondary);
		text-decoration: none;
		font-size: 13px;
		font-weight: 500;
		display: flex;
		align-items: center;
		gap: 8px;
		transition: all 0.15s;
	}

	.nav-item:hover {
		background: var(--bg-hover);
		color: var(--text-primary);
	}

	.nav-item.active {
		background: var(--blue-bg);
		color: var(--blue-text);
		font-weight: 600;
	}

	.back-item {
		margin-bottom: 8px;
		color: var(--text-muted);
	}

	.settings-content {
		flex: 1;
		padding: 28px;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: 20px;
		background: #FFFFFF;
		height: 100%;
	}

	.content-header h1 {
		font-size: 20px;
		font-weight: 700;
		margin-bottom: 4px;
		color: var(--text-primary);
	}

	.subtitle {
		color: var(--text-secondary);
		font-size: 13.5px;
	}

	.loading-state {
		color: var(--text-secondary);
		font-size: 13.5px;
		text-align: center;
		padding: 40px 0;
	}

	.banner {
		padding: 10px 14px;
		border-radius: 6px;
		font-size: 13px;
	}

	.banner.error {
		background: var(--danger-bg);
		border: 1px solid rgba(235, 87, 87, 0.3);
		color: var(--danger);
	}

	.banner.success {
		background: var(--success-bg);
		border: 1px solid rgba(46, 125, 50, 0.3);
		color: var(--success);
	}

	.settings-form-container {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.settings-card {
		padding: 20px;
		display: flex;
		flex-direction: column;
		gap: 6px;
		background: #FFFFFF;
	}

	.settings-card h3 {
		font-size: 15px;
		font-weight: 600;
		color: var(--text-primary);
	}

	.card-desc {
		font-size: 13px;
		color: var(--text-secondary);
		margin-bottom: 8px;
	}

	.radio-group {
		display: flex;
		flex-direction: column;
		gap: 8px;
		margin-top: 4px;
	}

	.mode-option {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 10px 14px;
		border: 1px solid var(--border-color);
		border-radius: 6px;
		cursor: pointer;
		font-size: 13px;
		font-weight: 500;
		transition: all 0.15s;
		background: #FFFFFF;
	}

	.mode-option:hover {
		background: var(--bg-hover);
	}

	.mode-option.selected {
		border-color: var(--blue-primary);
		background: var(--blue-bg);
		color: var(--blue-text);
	}

	.toggle-container {
		display: flex;
		align-items: center;
		gap: 14px;
		margin-top: 4px;
	}

	.toggle-label {
		font-size: 13px;
		font-weight: 500;
	}

	/* Toggle Switch Styles */
	.switch {
		position: relative;
		display: inline-block;
		width: 42px;
		height: 22px;
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
		background-color: #E8E8E5;
		transition: .2s;
		border: 1px solid var(--border-color);
	}

	.slider:before {
		position: absolute;
		content: "";
		height: 16px;
		width: 16px;
		left: 2px;
		bottom: 2px;
		background-color: white;
		transition: .2s;
		box-shadow: 0 1px 2px rgba(0,0,0,0.15);
	}

	input:checked + .slider {
		background-color: var(--blue-primary);
		border-color: var(--blue-primary);
	}

	input:checked + .slider:before {
		transform: translateX(20px);
	}

	.slider.round {
		border-radius: 22px;
	}

	.slider.round:before {
		border-radius: 50%;
	}

	.action-bar {
		margin-top: 8px;
		display: flex;
		justify-content: flex-start;
	}
</style>
