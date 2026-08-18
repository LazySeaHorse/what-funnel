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

	let pipeline = $state<any>(null);
	let states = $state<any[]>([]);
	let productMode = $state('full_workspace');

	// Form state for adding new state
	let newStateKey = $state('');
	let newStateLabel = $state('');
	let newStateColor = $state('#0B6E99');

	onMount(async () => {
		try {
			currentUser = await apiRequest('/auth/me');
			if (currentUser.role !== 'admin') {
				goto('/inbox');
				return;
			}
		} catch (err) {
			goto('/login');
			return;
		}

		try {
			const account = await apiRequest('/workspace/account').catch(() => null);
			if (account) {
				productMode = account.product_mode || 'full_workspace';
				if (productMode === 'chatbot_only') {
					goto('/settings/account');
					return;
				}
			}
			await loadPipeline();
		} catch (err: any) {
			error = 'Failed to load pipeline: ' + err.message;
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
		
		if (states.some(s => s.key === key)) {
			error = `State with key "${key}" already exists.`;
			return;
		}

		states.push({
			key,
			label: newStateLabel.trim() || newStateKey.trim(),
			color: newStateColor
		});

		newStateKey = '';
		newStateLabel = '';
		newStateColor = '#0B6E99';
	}

	function removeState(index: number) {
		error = '';
		successMsg = '';
		states.splice(index, 1);
	}

	async function handleSavePipeline() {
		if (!pipeline) return;
		error = '';
		successMsg = '';
		saving = true;

		try {
			await apiRequest(`/workspace/pipelines/${pipeline.id}`, {
				method: 'PUT',
				body: {
					name: pipeline.name,
					states
				}
			});
			successMsg = 'Pipeline stages saved successfully.';
			await loadPipeline();
		} catch (err: any) {
			error = 'Failed to save pipeline: ' + err.message;
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
			<a href="/settings/account" class="nav-item">
				<Icon name="settings" size={14} /> Account Settings
			</a>
			<a href="/settings/channels" class="nav-item">
				<Icon name="channels" size={14} /> Channels
			</a>
			<a href="/settings/users" class="nav-item">
				<Icon name="users" size={14} /> Workspace Users
			</a>
			<a href="/settings/pipeline" class="nav-item active">
				<Icon name="pipeline" size={14} /> Lead Pipeline
			</a>
			<a href="/settings/knowledge-base" class="nav-item">
				<Icon name="kb" size={14} /> Knowledge Base
			</a>
		</nav>
	</div>

	<div class="settings-content glass-panel">
		<div class="content-header">
			<div>
				<h1>Lead Pipeline Stages</h1>
				<p class="subtitle">Customize stages, colors, and sequence for tracking customer leads</p>
			</div>
		</div>

		{#if error}
			<div class="banner error">{error}</div>
		{/if}

		{#if successMsg}
			<div class="banner success">{successMsg}</div>
		{/if}

		{#if loading}
			<div class="loading-state">Loading pipeline stages...</div>
		{:else}
			<div class="pipeline-editor-container">
				<!-- Current Stages List -->
				<div class="settings-card glass-panel">
					<h3>Current Stages Sequence</h3>
					<p class="card-desc">Drag or re-order stages to represent your funnel progression.</p>

					<div class="states-list">
						{#each states as st, i}
							<div class="state-row glass-panel">
								<div class="color-dot" style="background-color: {st.color}"></div>
								<div class="state-info">
									<input 
										type="text" 
										class="input-field inline-edit" 
										bind:value={st.label} 
										placeholder="Stage Label"
									/>
									<span class="state-key">({st.key})</span>
								</div>
								<input 
									type="color" 
									class="color-picker" 
									bind:value={st.color} 
									title="Pick stage color"
								/>
								<div class="row-actions">
									<button 
										class="icon-btn" 
										onclick={() => moveState(i, 'up')} 
										disabled={i === 0}
										title="Move Up"
									>
										▲
									</button>
									<button 
										class="icon-btn" 
										onclick={() => moveState(i, 'down')} 
										disabled={i === states.length - 1}
										title="Move Down"
									>
										▼
									</button>
									<button 
										class="icon-btn delete-btn" 
										onclick={() => removeState(i)} 
										title="Remove Stage"
									>
										<Icon name="trash" size={13} color="var(--danger)" />
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
							disabled={saving}
						>
							{saving ? 'Saving...' : 'Save Pipeline Changes'}
						</button>
					</div>
				</div>

				<!-- Add New Stage Form -->
				<div class="settings-card glass-panel">
					<h3>Add New Funnel Stage</h3>
					<p class="card-desc">Create a custom stage for your sales funnel.</p>

					<form onsubmit={addState} class="add-state-form">
						<div class="form-row">
							<div class="form-group">
								<label for="stateKey">Stage Key</label>
								<input 
									type="text" 
									id="stateKey" 
									class="input-field" 
									bind:value={newStateKey} 
									placeholder="e.g. quote_sent" 
									required
								/>
							</div>

							<div class="form-group">
								<label for="stateLabel">Stage Label</label>
								<input 
									type="text" 
									id="stateLabel" 
									class="input-field" 
									bind:value={newStateLabel} 
									placeholder="e.g. Quote Sent" 
								/>
							</div>

							<div class="form-group color-group">
								<label for="stateColor">Color</label>
								<input 
									type="color" 
									id="stateColor" 
									class="color-picker-large" 
									bind:value={newStateColor} 
								/>
							</div>
						</div>

						<button type="submit" class="btn-secondary add-btn">
							<Icon name="plus" size={14} /> Add Stage
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
		font-weight: 500;
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
		font-weight: 500;
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
		font-weight: 500;
		margin-bottom: 2px;
	}

	.subtitle {
		font-size: 13.5px;
		color: var(--text-secondary);
	}

	.banner.error {
		padding: 10px 14px;
		background: var(--danger-bg);
		border: 1px solid rgba(235, 87, 87, 0.3);
		border-radius: 6px;
		color: var(--danger);
		font-size: 13px;
	}

	.banner.success {
		padding: 10px 14px;
		background: var(--success-bg);
		border: 1px solid rgba(46, 125, 50, 0.3);
		border-radius: 6px;
		color: var(--success);
		font-size: 13px;
	}

	.loading-state {
		text-align: center;
		padding: 40px;
		color: var(--text-secondary);
		font-size: 13.5px;
	}

	.pipeline-editor-container {
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
		font-weight: 500;
		color: var(--text-primary);
	}

	.card-desc {
		font-size: 13px;
		color: var(--text-secondary);
		margin-bottom: 10px;
	}

	.states-list {
		display: flex;
		flex-direction: column;
		gap: 8px;
		margin-bottom: 14px;
	}

	.state-row {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 8px 12px;
		background: var(--bg-hover);
	}

	.color-dot {
		width: 10px;
		height: 10px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.state-info {
		display: flex;
		align-items: center;
		gap: 8px;
		flex: 1;
	}

	.inline-edit {
		height: 32px;
		font-size: 13px;
		padding: 4px 8px;

	}

	.state-key {
		font-size: 11px;
		color: var(--text-muted);
	}

	.color-picker {
		width: 28px;
		height: 28px;
		border: none;
		background: none;
		cursor: pointer;
	}

	.row-actions {
		display: flex;
		gap: 4px;
	}

	.icon-btn {
		background: #FFFFFF;
		border: 1px solid var(--border-color);
		border-radius: 4px;
		width: 26px;
		height: 26px;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 10px;
		cursor: pointer;
	}

	.icon-btn:hover:not(:disabled) {
		background: var(--bg-hover);
	}

	.icon-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.add-state-form {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.form-row {
		display: flex;
		gap: 10px;
		align-items: flex-end;
	}

	.form-group {
		display: flex;
		flex-direction: column;
		gap: 4px;
		flex: 1;
	}

	.form-group label {
		font-size: 12px;
		font-weight: 500;
		color: var(--text-secondary);
	}

	.color-group {
		flex: 0 0 60px;
	}

	.color-picker-large {
		width: 100%;
		height: 36px;
		border: 1px solid var(--border-color);
		border-radius: 6px;
		cursor: pointer;
		background: #FFFFFF;
	}

	.add-btn {
		align-self: flex-start;
	}

	.action-bar {
		margin-top: 8px;
		display: flex;
		justify-content: flex-start;
	}
</style>
