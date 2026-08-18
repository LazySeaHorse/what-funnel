<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
	import Icon from '$lib/Icon.svelte';

	let users = $state<any[]>([]);
	let loading = $state(true);
	let error = $state('');
	let successMsg = $state('');
	let currentUser = $state<any | null>(null);
	let productMode = $state('full_workspace');

	// Invite Form State
	let inviteEmail = $state('');
	let inviteRole = $state('member');
	let lastInviteToken = $state('');
	let inviting = $state(false);

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
			await loadUsers();
		} catch (err: any) {
			error = 'Failed to load users: ' + err.message;
		} finally {
			loading = false;
		}
	});

	async function loadUsers() {
		try {
			users = await apiRequest('/workspace/users');
		} catch (err: any) {
			error = err.message;
		}
	}

	async function handleInvite(e: Event) {
		e.preventDefault();
		error = '';
		successMsg = '';
		lastInviteToken = '';
		inviting = true;

		try {
			const res = await apiRequest('/workspace/users/invite', {
				method: 'POST',
				body: {
					email: inviteEmail,
					role: inviteRole
				}
			});

			successMsg = `Successfully invited ${inviteEmail}!`;
			if (res.invite_token) {
				lastInviteToken = res.invite_token;
			}
			inviteEmail = '';
			await loadUsers();
		} catch (err: any) {
			error = err.message;
		} finally {
			inviting = false;
		}
	}

	async function handleChangeRole(userID: string, newRole: string) {
		error = '';
		successMsg = '';
		try {
			await apiRequest(`/workspace/users/${userID}/role`, {
				method: 'PUT',
				body: { role: newRole }
			});
			successMsg = 'User role updated successfully.';
			await loadUsers();
		} catch (err: any) {
			error = err.message;
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
			<a href="/settings/users" class="nav-item active">
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
				<h1>Workspace Users</h1>
				<p class="subtitle">Invite team members and manage member roles</p>
			</div>
		</div>

		{#if error}
			<div class="banner error">{error}</div>
		{/if}

		{#if successMsg}
			<div class="banner success">{successMsg}</div>
		{/if}

		{#if loading}
			<div class="loading-state">Loading users...</div>
		{:else}
			<div class="users-sections-container">
				<!-- Invite User Section -->
				<div class="settings-card glass-panel">
					<h3>Invite Team Member</h3>
					<p class="card-desc">Send an invitation to join your workspace.</p>

					<form onsubmit={handleInvite} class="invite-form">
						<div class="form-row">
							<div class="form-group flex-2">
								<label for="inviteEmail">Email Address</label>
								<input 
									type="email" 
									id="inviteEmail" 
									class="input-field" 
									bind:value={inviteEmail} 
									placeholder="colleague@example.com" 
									required
									disabled={inviting}
								/>
							</div>
							<div class="form-group flex-1">
								<label for="inviteRole">Role</label>
								<select id="inviteRole" class="input-field" bind:value={inviteRole} disabled={inviting}>
									<option value="member">Member</option>
									<option value="admin">Admin</option>
								</select>
							</div>
							<button type="submit" class="btn-primary invite-btn" disabled={inviting || !inviteEmail.trim()}>
								{inviting ? 'Inviting...' : 'Send Invite'}
							</button>
						</div>
					</form>

					{#if lastInviteToken}
						<div class="notion-callout token-box">
							<span>Invite Token:</span>
							<code>{lastInviteToken}</code>
						</div>
					{/if}
				</div>

				<!-- Users List Section -->
				<div class="settings-card glass-panel">
					<div class="section-header-row">
						<h3>Active Members</h3>
						<span class="badge-blue">{users.length} members</span>
					</div>
					<p class="card-desc">Manage roles for existing members in this workspace.</p>

					<div class="users-list">
						{#each users as u}
							<div class="user-row glass-panel">
								<div class="user-avatar">
									<Icon name="user" size={16} color="var(--text-secondary)" />
								</div>
								<div class="user-info">
									<span class="user-email">{u.email}</span>
									{#if u.id === currentUser?.id}
										<span class="badge-yellow">You</span>
									{/if}
								</div>
								<div class="user-role-control">
									{#if u.id === currentUser?.id}
										<span class="badge-blue">{u.role}</span>
									{:else}
										<select 
											class="input-field role-select" 
											value={u.role}
											onchange={(e) => handleChangeRole(u.id, (e.target as HTMLSelectElement).value)}
										>
											<option value="member">Member</option>
											<option value="admin">Admin</option>
										</select>
									{/if}
								</div>
							</div>
						{/each}
					</div>
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

	.users-sections-container {
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

	.section-header-row {
		display: flex;
		justify-content: space-between;
		align-items: center;
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
	}

	.flex-2 { flex: 2; }
	.flex-1 { flex: 1; }

	.form-group label {
		font-size: 12px;
		font-weight: 500;
		color: var(--text-secondary);
	}

	.invite-btn {
		height: 34px;
	}

	.token-box {
		margin-top: 10px;
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 12.5px;
	}

	.token-box code {
		font-family: monospace;
		background: #FFFFFF;
		padding: 2px 6px;
		border-radius: 4px;
		border: 1px solid var(--yellow-border);
	}

	.users-list {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.user-row {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 10px 14px;
		background: var(--bg-hover);
	}

	.user-avatar {
		width: 32px;
		height: 32px;
		border-radius: 50%;
		background: #FFFFFF;
		border: 1px solid var(--border-color);
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.user-info {
		flex: 1;
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.user-email {
		font-size: 13.5px;
		font-weight: 500;
		color: var(--text-primary);
	}

	.role-select {
		height: 30px;
		font-size: 12px;
		width: 110px;
	}
</style>
