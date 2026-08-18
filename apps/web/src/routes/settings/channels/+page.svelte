<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
	import Icon from '$lib/Icon.svelte';

	let channels = $state<any[]>([]);
	let loading = $state(true);
	let error = $state('');
	let currentUser = $state<any | null>(null);
	let productMode = $state('full_workspace');

	// Modal & connect wizard state
	let isModalOpen = $state(false);
	let channelType = $state('matrix_whatsapp');
	let bridgeIdentity = $state('');
	let bridgeCredentials = $state('');
	let qrCodeVisible = $state(false);
	let connectionStatus = $state('idle'); // idle, generating, scanning, connected
	let activeChannelID = $state<string | null>(null);

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
			}
			await loadChannels();
			
			window.addEventListener('channel-status-changed', handleLiveStatusChange as EventListener);
		} catch (err: any) {
			error = 'Failed to load channels: ' + err.message;
		} finally {
			loading = false;
		}
	});

	onDestroy(() => {
		if (typeof window !== 'undefined') {
			window.removeEventListener('channel-status-changed', handleLiveStatusChange as EventListener);
		}
	});

	function handleLiveStatusChange(e: CustomEvent) {
		const event = e.detail;
		const index = channels.findIndex((c) => c.id === event.channel_id);
		if (index !== -1) {
			channels[index].status = event.status;
			channels[index].detail = event.detail;
		}
		
		if (activeChannelID === event.channel_id) {
			if (event.status === 'connected') {
				connectionStatus = 'connected';
				setTimeout(() => {
					closeModal();
					loadChannels();
				}, 1500);
			}
		}
	}

	async function loadChannels() {
		try {
			channels = await apiRequest('/channels');
		} catch (err: any) {
			error = err.message;
		}
	}

	async function handleCreateChannel(e: Event) {
		e.preventDefault();
		error = '';
		
		let creds = bridgeCredentials.trim();
		if (!creds) {
			creds = JSON.stringify({
				homeserver_url: 'http://localhost:8008',
				user_id: `@whatsapp_bridge:localhost`,
				access_token: 'mock-token'
			});
		}

		try {
			const body = {
				type: channelType,
				bridge_identity: bridgeIdentity || `whatsapp-${Date.now()}`,
				bridge_credentials: creds
			};

			const newChan = await apiRequest('/channels', {
				method: 'POST',
				body
			});

			activeChannelID = newChan.id;
			await loadChannels();

			if (channelType === 'matrix_whatsapp') {
				qrCodeVisible = true;
				connectionStatus = 'scanning';
				
				setTimeout(async () => {
					if (connectionStatus === 'scanning') {
						connectionStatus = 'connected';
						setTimeout(() => {
							closeModal();
							loadChannels();
						}, 1500);
					}
				}, 6000);
			} else {
				closeModal();
				await loadChannels();
			}
		} catch (err: any) {
			error = err.message;
		}
	}

	async function handleDisconnect(id: string) {
		if (!confirm('Are you sure you want to disconnect this channel?')) return;
		try {
			await apiRequest(`/channels/${id}/disconnect`, { method: 'POST' });
			await loadChannels();
		} catch (err: any) {
			error = err.message;
		}
	}

	function closeModal() {
		isModalOpen = false;
		qrCodeVisible = false;
		connectionStatus = 'idle';
		bridgeIdentity = '';
		bridgeCredentials = '';
		activeChannelID = null;
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
			<a href="/settings/channels" class="nav-item active">
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
				<h1>Connected Channels</h1>
				<p class="subtitle">Manage WhatsApp and communication channel integrations</p>
			</div>
			<button class="btn-primary" onclick={() => isModalOpen = true}>
				<Icon name="plus" size={14} /> Connect Channel
			</button>
		</div>

		{#if error}
			<div class="banner error">{error}</div>
		{/if}

		{#if loading}
			<div class="loading-state">Loading channels...</div>
		{:else}
			<div class="channels-grid">
				{#each channels as chan}
					<div class="channel-card glass-panel">
						<div class="channel-card-header">
							<div class="badge-blue channel-type-badge">
								<Icon name="whatsapp" size={13} color="var(--blue-text)" />
								<span>{chan.type.replace('matrix_', '')}</span>
							</div>
							{#if chan.status === 'connected'}
								<span class="badge-blue">connected</span>
							{:else}
								<span class="badge-yellow">{chan.status}</span>
							{/if}
						</div>
						<div class="channel-details">
							<div class="detail-row">
								<span class="label">Bridge ID:</span>
								<span class="value">{chan.bridge_identity || 'N/A'}</span>
							</div>
							{#if chan.detail}
								<div class="detail-row">
									<span class="label">Details:</span>
									<span class="value detail-text">{chan.detail}</span>
								</div>
							{/if}
						</div>
						<div class="channel-actions">
							<button 
								class="btn-secondary disconnect-btn" 
								onclick={() => handleDisconnect(chan.id)}
								disabled={chan.status === 'disconnected'}
							>
								Disconnect
							</button>
						</div>
					</div>
				{:else}
					<div class="empty-state">
						<div style="width: 40px; height: 40px; border-radius: 8px; background: var(--blue-bg); border: 1px solid var(--blue-border); display: flex; align-items: center; justify-content: center; margin: 0 auto 8px;">
							<Icon name="channels" size={20} color="var(--blue-text)" />
						</div>
						<h3>No Channels Connected</h3>
						<p>Connect a WhatsApp bridge to start receiving customer messages.</p>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>

<!-- Connect Channel Modal -->
{#if isModalOpen}
	<div class="modal-backdrop">
		<div class="modal-card glass-panel">
			<div class="modal-header">
				<h3>Connect New Channel</h3>
				<button class="close-btn" onclick={closeModal}>
					<Icon name="x" size={16} />
				</button>
			</div>

			{#if !qrCodeVisible}
				<form onsubmit={handleCreateChannel} class="modal-form">
					<div class="form-group">
						<label for="type">Integration Type</label>
						<select id="type" class="input-field" bind:value={channelType}>
							<option value="matrix_whatsapp">WhatsApp (via Matrix Bridge)</option>
							<option value="matrix_instagram">Instagram (via Matrix Bridge)</option>
							<option value="matrix_messenger">Messenger (via Matrix Bridge)</option>
							<option value="matrix_telegram">Telegram (via Matrix Bridge)</option>
						</select>
					</div>

					<div class="form-group">
						<label for="identity">Bridge Identity (Optional)</label>
						<input 
							type="text" 
							id="identity" 
							class="input-field" 
							bind:value={bridgeIdentity} 
							placeholder="e.g. whatsapp-main"
						/>
					</div>

					<div class="form-group">
						<label for="credentials">Bridge Credentials JSON (Optional)</label>
						<textarea 
							id="credentials" 
							class="input-field credentials-area" 
							bind:value={bridgeCredentials} 
							placeholder="Enter JSON credentials here..."
						></textarea>
					</div>

					<div class="modal-actions">
						<button type="button" class="btn-secondary" onclick={closeModal}>Cancel</button>
						<button type="submit" class="btn-primary">Connect</button>
					</div>
				</form>
			{:else}
				<div class="qr-container">
					{#if connectionStatus === 'scanning'}
						<div class="qr-box">
							<div class="mock-qr">
								<div class="qr-pattern"></div>
								<div class="qr-scanner-line"></div>
							</div>
						</div>
						<div class="qr-info">
							<h4>Scan QR Code</h4>
							<p>Open WhatsApp on your mobile device, tap Linked Devices, and scan this QR code.</p>
							<span class="badge-yellow">Waiting for scan...</span>
						</div>
					{:else if connectionStatus === 'connected'}
						<div class="success-box">
							<Icon name="check" size={32} color="var(--success)" strokeWidth={3} />
							<h4>Connection Established!</h4>
							<p>The channel is connected. Re-routing back to channel list.</p>
						</div>
					{/if}
				</div>
			{/if}
		</div>
	</div>
{/if}

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

	.content-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		border-bottom: 1px solid var(--border-color);
		padding-bottom: 14px;
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

	.loading-state {
		text-align: center;
		padding: 40px;
		color: var(--text-secondary);
		font-size: 13.5px;
	}

	.channels-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
		gap: 14px;
	}

	.channel-card {
		padding: 16px;
		display: flex;
		flex-direction: column;
		gap: 12px;
		background: #FFFFFF;
	}

	.channel-card-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.channel-type-badge {
		display: flex;
		align-items: center;
		gap: 6px;
		text-transform: capitalize;
	}

	.channel-details {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: 12.5px;
	}

	.detail-row {
		display: flex;
		gap: 6px;
	}

	.label {
		color: var(--text-muted);
	}

	.value {
		color: var(--text-primary);
		font-weight: 500;
	}

	.disconnect-btn {
		width: 100%;
		font-size: 12.5px;
	}

	.empty-state {
		grid-column: 1 / -1;
		text-align: center;
		padding: 40px;
		color: var(--text-secondary);
	}

	.empty-state h3 {
		font-size: 16px;
		font-weight: 500;
		color: var(--text-primary);
		margin-bottom: 4px;
	}

	.empty-state p {
		font-size: 13px;
	}

	/* Modal */
	.modal-backdrop {
		position: fixed;
		inset: 0;
		background: rgba(15, 15, 15, 0.4);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 300;
	}

	.modal-card {
		width: 100%;
		max-width: 440px;
		padding: 24px;
		background: #FFFFFF;
		border-radius: 8px;
	}

	.modal-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 16px;
	}

	.modal-header h3 {
		font-size: 16px;
		font-weight: 500;
	}

	.close-btn {
		background: none;
		border: none;
		cursor: pointer;
		color: var(--text-muted);
	}

	.modal-form {
		display: flex;
		flex-direction: column;
		gap: 14px;
	}

	.form-group {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.form-group label {
		font-size: 12px;
		font-weight: 500;
		color: var(--text-secondary);
	}

	.credentials-area {
		height: 70px;
		resize: none;
	}

	.modal-actions {
		display: flex;
		justify-content: flex-end;
		gap: 8px;
		margin-top: 8px;
	}

	.qr-container {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 16px;
		padding: 16px 0;
		text-align: center;
	}

	.qr-box {
		width: 160px;
		height: 160px;
		border: 1px solid var(--blue-primary);
		border-radius: 8px;
		background: var(--blue-bg);
		display: flex;
		align-items: center;
		justify-content: center;
		position: relative;
		overflow: hidden;
	}

	.qr-scanner-line {
		position: absolute;
		left: 0; right: 0;
		height: 2px;
		background: var(--blue-primary);
		animation: scan 2s ease-in-out infinite;
	}

	@keyframes scan {
		0% { top: 10%; }
		50% { top: 88%; }
		100% { top: 10%; }
	}

	.qr-info h4 {
		font-size: 15px;
		font-weight: 500;
		margin-bottom: 4px;
	}

	.qr-info p {
		font-size: 12.5px;
		color: var(--text-secondary);
		margin-bottom: 10px;
	}

	.success-box {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
	}
</style>
