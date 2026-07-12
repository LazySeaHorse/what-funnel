<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';

	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let successMsg = $state('');
	let currentUser = $state<any | null>(null);

	let pipeline = $state<any>(null);
	let states = $state<any[]>([]);
	let productMode = $state('full_workspace');

	// Form state for adding new state
	let newStateKey = $state('');
	let newStateLabel = $state('');
	let newStateColor = $state('#6366f1');

	onMount(async () => {
		try {
			currentUser = await apiRequest('/auth/me');
			if (currentUser.role !== 'admin') {
				goto('/inbox');
				return;
			}
			const account = await apiRequest('/workspace/account');
			if (account) {
				productMode = account.product_mode || 'full_workspace';
				if (productMode === 'chatbot_only') {
					goto('/settings/account');
					return;
				}
			}
			await loadPipeline();
		} catch (err) {
			goto('/login');
		} finally {
			loading = false;
		}
	});

	async function loadPipeline() {
		try {
			const pipelines = await apiRequest('/workspace/pipelines');
			if (pipelines && pipelines.length > 0) {
				pipeline = pipelines[0];
				states = [...pipeline.states];
			}
		} catch (err: any) {
			error = 'Failed to load pipeline: ' + err.message;
		}
	}

	function moveState(index: number, direction: 'up' | 'down') {
		if (direction === 'up' && index > 0) {
			const temp = states[index];
			states[index] = states[index - 1];
			states[index - 1] = temp;
		} else if (direction === 'down' && index < states.length - 1) {
			const temp = states[index];
			states[index] = states[index + 1];
			states[index + 1] = temp;
		}
	}

	function addState(e: Event) {
		e.preventDefault();
		error = '';
		
		const key = newStateKey.trim().toLowerCase().replace(/\s+/g, '_');
		if (!key) {
			error = 'State key cannot be empty.';
			return;
		}
		
		// Check for duplicate keys
		if (states.some(s => s.key === key)) {
			error = `State with key "${key}" already exists.`;
			return;
		}

		states.push({
			key,
			label: newStateLabel.trim() || newStateKey.trim(),
			color: newStateColor
		});

		// Clear form
		newStateKey = '';
		newStateLabel = '';
		newStateColor = '#6366f1';
	}

	function removeState(index: number) {
		error = '';
		successMsg = '';
		states.splice(index, 1);
	}

	async function handleSavePipeline() {
		error = '';
		successMsg = '';
		saving = true;

		try {
			await apiRequest(`/workspace/pipelines/${pipeline.id}`, {
				method: 'PUT',
				body: {
					name: pipeline.name,
					states: states
				}
			});
			successMsg = 'Pipeline states saved successfully.';
			await loadPipeline();
		} catch (err: any) {
			error = err.message;
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
			<a href="/settings/account" class="nav-item">Account Settings</a>
			<a href="/settings/channels" class="nav-item">Channels</a>
			<a href="/settings/users" class="nav-item">Workspace Users</a>
			{#if productMode !== 'chatbot_only'}
				<a href="/settings/pipeline" class="nav-item active">Lead Pipeline</a>
			{/if}
			<a href="/settings/knowledge-base" class="nav-item">Knowledge Base</a>
		</nav>
	</div>


	<div class="settings-content glass-panel">
		<div class="content-header">
			<div>
				<h1>Lead Pipeline Editor</h1>
				<p class="subtitle">Customize lead states, order, and colors for your business pipeline</p>
			</div>
		</div>

		{#if error}
			<div class="banner error">{error}</div>
		{/if}

		{#if successMsg}
			<div class="banner success">{successMsg}</div>
		{/if}

		{#if loading}
			<div class="loading-state">Loading pipeline config...</div>
		{:else if pipeline}
			<div class="pipeline-split">
				<!-- Pipeline States List -->
				<div class="pipeline-list-pane">
					<h3>Current Pipeline States</h3>
					<p class="section-desc">Reorder, rename, or delete stages in your workflow.</p>
					
					<div class="states-list">
						{#each states as state, index}
							<div class="state-row glass-panel">
								<div class="state-color-indicator" style="background-color: {state.color}"></div>
								
								<div class="state-info">
									<div class="state-key"><code>{state.key}</code></div>
									<input 
										type="text" 
										class="input-field state-label-input" 
										bind:value={state.label}
										placeholder="State Label"
										required
									/>
								</div>

								<div class="state-color-picker">
									<input 
										type="color" 
										class="color-picker-input" 
										bind:value={state.color}
									/>
								</div>

								<div class="state-actions">
									<button 
										type="button" 
										class="btn-icon" 
										disabled={index === 0} 
										onclick={() => moveState(index, 'up')}
										title="Move Up"
									>
										▲
									</button>
									<button 
										type="button" 
										class="btn-icon" 
										disabled={index === states.length - 1} 
										onclick={() => moveState(index, 'down')}
										title="Move Down"
									>
										▼
									</button>
									<button 
										type="button" 
										class="btn-danger-icon" 
										onclick={() => removeState(index)}
										title="Delete State"
									>
										&times;
									</button>
								</div>
							</div>
						{/each}
					</div>

					<div class="action-bar">
						<button 
							type="button" 
							class="btn-primary" 
							onclick={handleSavePipeline}
							disabled={saving || states.length === 0}
						>
							{saving ? 'Saving...' : 'Save Pipeline Configuration'}
						</button>
					</div>
				</div>

				<!-- Add New State Form -->
				<div class="add-state-pane glass-panel">
					<h3>Add New Lead State</h3>
					<form onsubmit={addState} class="add-state-form">
						<div class="form-group">
							<label for="state-key">State Key (unique, lowercase)</label>
							<input 
								type="text" 
								id="state-key" 
								class="input-field" 
								bind:value={newStateKey} 
								placeholder="e.g. negotiation" 
								required 
							/>
						</div>

						<div class="form-group">
							<label for="state-label">Display Label</label>
							<input 
								type="text" 
								id="state-label" 
								class="input-field" 
								bind:value={newStateLabel} 
								placeholder="e.g. Negotiation" 
							/>
						</div>

						<div class="form-group">
							<label for="state-color">State Color</label>
							<div class="color-select-group">
								<input 
									type="color" 
									id="state-color" 
									class="color-picker-input-large" 
									bind:value={newStateColor} 
								/>
								<input 
									type="text" 
									class="input-field color-hex-text" 
									bind:value={newStateColor}
									placeholder="#ffffff"
								/>
							</div>
						</div>

						<button type="submit" class="btn-secondary">
							+ Add to Pipeline
						</button>
					</form>
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

	.pipeline-split {
		display: flex;
		gap: 24px;
		align-items: flex-start;
	}

	.pipeline-list-pane {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.section-desc {
		font-size: 13px;
		color: var(--text-secondary);
		margin-bottom: 8px;
	}

	.states-list {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.state-row {
		display: flex;
		align-items: center;
		padding: 14px 18px;
		gap: 16px;
	}

	.state-color-indicator {
		width: 16px;
		height: 16px;
		border-radius: 50%;
		flex-shrink: 0;
		box-shadow: 0 0 10px rgba(255, 255, 255, 0.1);
	}

	.state-info {
		flex: 1;
		display: flex;
		gap: 16px;
		align-items: center;
	}

	.state-key {
		font-size: 13px;
		color: var(--text-secondary);
		min-width: 100px;
	}

	.state-label-input {
		flex: 1;
		padding: 6px 10px;
	}

	.state-color-picker {
		display: flex;
		align-items: center;
	}

	.color-picker-input {
		border: none;
		outline: none;
		width: 28px;
		height: 28px;
		border-radius: 6px;
		background: transparent;
		cursor: pointer;
	}

	.state-actions {
		display: flex;
		gap: 4px;
	}

	.btn-icon {
		background: rgba(255, 255, 255, 0.05);
		border: 1px solid var(--border-color);
		color: var(--text-secondary);
		font-size: 11px;
		width: 28px;
		height: 28px;
		border-radius: 6px;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.btn-icon:hover:not(:disabled) {
		background: rgba(255, 255, 255, 0.1);
		color: var(--text-primary);
	}

	.btn-icon:disabled {
		opacity: 0.3;
		cursor: not-allowed;
	}

	.btn-danger-icon {
		background: rgba(239, 68, 68, 0.1);
		border: 1px solid rgba(239, 68, 68, 0.2);
		color: #ef4444;
		font-size: 16px;
		font-weight: 700;
		width: 28px;
		height: 28px;
		border-radius: 6px;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: background-color 0.2s;
	}

	.btn-danger-icon:hover {
		background: rgba(239, 68, 68, 0.2);
	}

	.add-state-pane {
		width: 340px;
		padding: 24px;
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.add-state-form {
		display: flex;
		flex-direction: column;
		gap: 14px;
	}

	.form-group {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.form-group label {
		font-size: 12px;
		font-weight: 500;
		color: var(--text-secondary);
	}

	.color-select-group {
		display: flex;
		gap: 10px;
		align-items: center;
	}

	.color-picker-input-large {
		border: none;
		outline: none;
		width: 40px;
		height: 40px;
		border-radius: 8px;
		background: transparent;
		cursor: pointer;
	}

	.color-hex-text {
		flex: 1;
	}

	.action-bar {
		margin-top: 16px;
	}
</style>
