<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';

	let users = $state<any[]>([]);
	let loading = $state(true);
	let error = $state('');
	let successMsg = $state('');
	let currentUser = $state<any | null>(null);

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
			await loadUsers();
		} catch (err) {
			goto('/login');
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
			<a href="/inbox" class="nav-item">← Back to Inbox</a>
			<a href="/settings/account" class="nav-item">Account Settings</a>
			<a href="/settings/channels" class="nav-item">Channels</a>
			<a href="/settings/users" class="nav-item active">Workspace Users</a>
			<a href="/settings/pipeline" class="nav-item">Lead Pipeline</a>
		</nav>
	</div>

	<div class="settings-content glass-panel">
		<div class="content-header">
			<div>
				<h1>Workspace Users</h1>
				<p class="subtitle">Invite and manage roles for agents in your organization</p>
			</div>
		</div>

		{#if error}
			<div class="banner error">{error}</div>
		{/if}

		{#if successMsg}
			<div class="banner success">
				<p>{successMsg}</p>
				{#if lastInviteToken}
					<div class="token-container">
						<span class="label">Invite Token:</span>
						<code class="token-code">{lastInviteToken}</code>
						<p class="token-note">Note: Share this token with the user to let them sign up.</p>
					</div>
				{/if}
			</div>
		{/if}

		<div class="users-split">
			<!-- User List -->
			<div class="users-list-pane">
				<h3>Current Team Members</h3>
				{#if loading}
					<div class="loading-state">Loading users...</div>
				{:else}
					<table class="users-table">
						<thead>
							<tr>
								<th>User Email</th>
								<th>Role</th>
								<th>Actions</th>
							</tr>
						</thead>
						<tbody>
							{#each users as u}
								<tr>
									<td>
										<div class="user-email-cell">
											{u.email}
											{#if u.id === currentUser?.user_id}
												<span class="self-badge">(You)</span>
											{/if}
										</div>
									</td>
									<td>
										<span class="role-badge {u.role}">{u.role}</span>
									</td>
									<td>
										{#if u.id !== currentUser?.user_id}
											<select 
												class="input-field role-select" 
												value={u.role} 
												onchange={(e) => handleChangeRole(u.id, (e.target as HTMLSelectElement).value)}
											>
												<option value="member">Member</option>
												<option value="admin">Admin</option>
											</select>
										{:else}
											<span class="disabled-action">Cannot change self</span>
										{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				{/if}
			</div>

			<!-- Invite Form -->
			<div class="invite-pane glass-panel">
				<h3>Invite New User</h3>
				<form onsubmit={handleInvite} class="invite-form">
					<div class="form-group">
						<label for="email">User Email Address</label>
						<input 
							type="email" 
							id="email" 
							class="input-field" 
							bind:value={inviteEmail} 
							placeholder="agent@example.com" 
							required 
							disabled={inviting}
						/>
					</div>

					<div class="form-group">
						<label for="role">Assign Role</label>
						<select id="role" class="input-field" bind:value={inviteRole} disabled={inviting}>
							<option value="member">Workspace Member</option>
							<option value="admin">Workspace Admin</option>
						</select>
					</div>

					<button type="submit" class="btn-primary" disabled={inviting}>
						{inviting ? 'Inviting...' : 'Send Invitation'}
					</button>
				</form>
			</div>
		</div>
	</div>
</div>

<style>
	.settings-container {
		display: grid;
		grid-template-columns: 240px 1fr;
		height: 100vh;
		background-color: var(--bg-dark);
		padding: 16px;
		gap: 16px;
	}

	.settings-sidebar {
		padding: 24px 16px;
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.sidebar-title {
		font-size: 18px;
		font-weight: 700;
		color: var(--text-primary);
		padding-left: 8px;
	}

	.sidebar-nav {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.nav-item {
		padding: 10px 12px;
		font-size: 14px;
		color: var(--text-secondary);
		text-decoration: none;
		border-radius: 6px;
		transition: background-color 0.2s, color 0.2s;
	}

	.nav-item:hover {
		background: rgba(255, 255, 255, 0.03);
		color: var(--text-primary);
	}

	.nav-item.active {
		background: rgba(99, 102, 241, 0.1);
		color: #818cf8;
		font-weight: 500;
	}

	.settings-content {
		padding: 24px;
		display: flex;
		flex-direction: column;
		overflow-y: auto;
	}

	.content-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 24px;
		border-bottom: 1px solid var(--border-color);
		padding-bottom: 16px;
	}

	.subtitle {
		font-size: 14px;
		color: var(--text-secondary);
		margin-top: 4px;
	}

	.banner {
		padding: 12px 16px;
		border-radius: 8px;
		font-size: 13px;
		margin-bottom: 20px;
	}

	.banner.error {
		background: rgba(239, 68, 68, 0.1);
		border: 1px solid var(--danger);
		color: var(--danger);
	}

	.banner.success {
		background: rgba(34, 197, 94, 0.1);
		border: 1px solid var(--success);
		color: #4ade80;
	}

	.token-container {
		margin-top: 12px;
		padding: 12px;
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid var(--border-color);
		border-radius: 6px;
	}

	.token-code {
		display: block;
		font-family: monospace;
		font-size: 14px;
		color: #818cf8;
		background: rgba(99, 102, 241, 0.1);
		padding: 6px 10px;
		border-radius: 4px;
		margin: 6px 0;
		user-select: all;
	}

	.token-note {
		font-size: 11px;
		color: var(--text-muted);
	}

	.users-split {
		display: grid;
		grid-template-columns: 1fr 300px;
		gap: 24px;
		align-items: start;
	}

	.users-list-pane h3,
	.invite-pane h3 {
		font-size: 16px;
		font-weight: 600;
		margin-bottom: 16px;
	}

	.invite-pane {
		padding: 20px;
	}

	.invite-form {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.form-group {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.form-group label {
		font-size: 13px;
		font-weight: 500;
		color: var(--text-secondary);
	}

	.loading-state {
		text-align: center;
		padding: 24px;
		color: var(--text-secondary);
	}

	.users-table {
		width: 100%;
		border-collapse: collapse;
	}

	.users-table th,
	.users-table td {
		text-align: left;
		padding: 12px;
		border-bottom: 1px solid var(--border-color);
		font-size: 14px;
	}

	.users-table th {
		color: var(--text-secondary);
		font-weight: 500;
	}

	.user-email-cell {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.self-badge {
		font-size: 11px;
		color: var(--text-muted);
	}

	.role-badge {
		font-size: 11px;
		font-weight: 600;
		text-transform: uppercase;
		padding: 2px 6px;
		border-radius: 4px;
	}

	.role-badge.admin {
		background: rgba(168, 85, 247, 0.15);
		color: #c084fc;
	}

	.role-badge.member {
		background: rgba(99, 102, 241, 0.15);
		color: #818cf8;
	}

	.role-select {
		padding: 4px 8px;
		font-size: 13px;
		width: 120px;
	}

	.disabled-action {
		font-size: 12px;
		color: var(--text-muted);
	}
</style>
