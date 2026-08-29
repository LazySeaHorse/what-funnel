<script lang="ts">
	import { onMount } from 'svelte';
	import { apiRequest } from '$lib/api';
	import type { InboxState } from '$lib/store.svelte';
	import type { WorkspaceState } from '$lib/workspace.svelte';
	import ChannelBadge from '$lib/components/ChannelBadge.svelte';
	import PipelineSettings from './PipelineSettings.svelte';
	import SettingsSidebar from './SettingsSidebar.svelte';
	import SettingsInfoPanel from './SettingsInfoPanel.svelte';

	let {
		inbox,
		workspace,
		initialSection = 'general',
		onNavigate
	}: {
		inbox?: InboxState;
		workspace?: WorkspaceState;
		initialSection?: string;
		onNavigate?: (tab: string) => void;
	} = $props();

	// Navigation sections (Only the 4 workspace subsections)
	type SettingsSection =
		| 'general'
		| 'business_profile'
		| 'ai_provider'
		| 'users_permissions'
		| 'channels'
		| 'pipeline';

	let activeSection = $state<SettingsSection>('general');

	type IntlWithTimeZones = typeof Intl & {
		supportedValuesOf?: (key: 'timeZone') => string[];
	};

	const supportedTimeZones = (() => {
		const zones = (Intl as IntlWithTimeZones).supportedValuesOf?.('timeZone') ?? [];
		return ['UTC', ...zones.filter((zone) => zone !== 'UTC')];
	})();

	function normalizeGMTOffset(offset: string): string {
		if (offset === 'GMT' || offset === 'UTC') return 'GMT+00:00';
		const match = offset.match(/^(?:GMT|UTC)([+-])(\d{1,2})(?::(\d{2}))?$/);
		if (!match) return offset;
		return `GMT${match[1]}${match[2].padStart(2, '0')}:${match[3] ?? '00'}`;
	}

	function formatTimeZoneLabel(zone: string): string {
		try {
			const offset = new Intl.DateTimeFormat('en-US', {
				timeZone: zone,
				timeZoneName: 'shortOffset'
			}).formatToParts(new Date()).find((part) => part.type === 'timeZoneName')?.value ?? 'GMT';
			return `(${normalizeGMTOffset(offset)}) ${zone.replaceAll('_', ' ').replaceAll('/', ' / ')}`;
		} catch {
			return zone;
		}
	}

	function normalizeSavedTimeZone(value: string): string {
		if (value === 'UTC' || value.includes('UTC')) return 'UTC';
		const compactValue = value.replace(/\s*\/\s*/g, '/');
		return supportedTimeZones.find((zone) => compactValue.includes(zone)) ?? value;
	}

	// Form & state variables
	let loading = $state(false);
	let saving = $state(false);
	let successMsg = $state('');
	let errorMsg = $state('');

	// General settings fields (matches crap/tabz.webp settings UI)
	let workspaceName = $state('');
	let defaultTimeZone = $state('UTC');
	let language = $state('English');
	let dateFormat = $state('DD MMM YYYY');
	let timeFormat = $state<'12' | '24'>('12');

	// Business profile fields
	let businessCategory = $state('');
	let businessPhone = $state('');
	let businessEmail = $state('');
	let businessAddress = $state('');
	let businessWebsite = $state('');
	let businessHours = $state('');

	// AI provider credentials are write-only. The API reports only whether a
	// configuration exists, so an existing key is never returned to the browser.
	let aiProviderConfigured = $state(false);
	let aiProviderApiKey = $state('');
	let aiProviderBaseURL = $state('https://api.openai.com/v1');
	let aiCompletionModel = $state('gpt-4o-mini');
	let aiEmbeddingModel = $state('text-embedding-3-small');
	let showAIProviderKey = $state(false);
	let savingAIProvider = $state(false);

	// Plan & storage details
	let currentPlan = $state('Pro Plan');
	let storageUsedGB = $state(4.2);
	let storageTotalGB = $state(20);
	let storagePercent = $derived(Math.round((storageUsedGB / storageTotalGB) * 100));

	// Modals & UI
	let showDeleteModal = $state(false);
	let deleteConfirmationInput = $state('');
	let showPlanModal = $state(false);
	let showChannelModal = $state(false);
	let newChannelPlatform = $state<'whatsapp' | 'instagram' | 'messenger' | 'telegram'>('whatsapp');
	let activeConnection = $state<any>(null);
	let bridgeConnections = $state<any[]>([]);
	let connectionSecret = $state('');
	let connectionCode = $state('');
	let connectionBusy = $state(false);
	let qrRefreshToken = $state(Date.now());

	// User Management State
	let accountSlug = $state('');
	let savingSlug = $state(false);
	let showAddUserModal = $state(false);
	let newUsername = $state('');
	let newPassword = $state('');
	let newRole = $state<'agent' | 'manager'>('agent');
	let addingUser = $state(false);
	let userModalError = $state('');

	let createdUserResult = $state<{ username: string; plaintextPassword?: string; role: string } | null>(null);
	let copiedPassword = $state(false);

	let showResetPasswordModal = $state(false);
	let resetUserTarget = $state<any | null>(null);
	let resetNewPassword = $state('');
	let resettingPassword = $state(false);
	let resetPasswordError = $state('');

	let showDeleteUserModal = $state(false);
	let deleteUserTarget = $state<any | null>(null);
	let deletingUser = $state(false);

	// Team users & channels
	let teamUsers = $state<any[]>([]);
	let channelsList = $state<any[]>([]);
	let productMode = $state('full_workspace');
	let leadTracking = $state(true);
	let unassignedVisible = $state(true);

	function closeModal() {
		showAddUserModal = false;
		showDeleteUserModal = false;
		showResetPasswordModal = false;
		createdUserResult = null;
		deleteUserTarget = null;
		resetUserTarget = null;
		showDeleteModal = false;
		showPlanModal = false;
		showChannelModal = false;
		activeConnection = null;
		connectionSecret = '';
		connectionCode = '';
		deleteConfirmationInput = '';
		userModalError = '';
		resetPasswordError = '';
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') closeModal();
	}

	function applyWorkspaceData(account: any) {
		if (!account) return;
		if (account.name) workspaceName = account.name;
		productMode = account.product_mode || 'full_workspace';
		if (!account.settings) return;
		try {
			const parsed = JSON.parse(atob(account.settings));
			if (parsed.timezone) defaultTimeZone = normalizeSavedTimeZone(parsed.timezone);
			if (parsed.language) language = parsed.language;
			if (parsed.date_format) dateFormat = parsed.date_format;
			if (parsed.time_format) timeFormat = parsed.time_format;
			if (parsed.business_category) businessCategory = parsed.business_category;
			if (parsed.business_phone) businessPhone = parsed.business_phone;
			if (parsed.business_email) businessEmail = parsed.business_email;
			if (parsed.business_address) businessAddress = parsed.business_address;
			if (parsed.business_website) businessWebsite = parsed.business_website;
			if (parsed.business_hours) businessHours = parsed.business_hours;
			leadTracking = parsed.lead_tracking_enabled !== false;
			unassignedVisible = parsed.unassigned_conversations_visible_to_members !== false;
		} catch (err) {
			console.error('Failed to parse settings', err);
		}
	}

	function connectedChannels(channels: any[]) {
		return channels.filter((channel) => channel.status !== 'disconnected');
	}

	async function refreshChannelsAndConnections(refreshBridge = false) {
		const connectionPath = refreshBridge ? '/bridge-connections?refresh=true' : '/bridge-connections';
		const [channels, connections] = await Promise.all([apiRequest('/channels'), apiRequest(connectionPath)]);
		channelsList = connectedChannels(channels);
		bridgeConnections = Array.isArray(connections) ? connections : [];
		if (activeConnection) {
			activeConnection = bridgeConnections.find((connection) => connection.channel_id === activeConnection.channel_id) || activeConnection;
		}
	}

	async function refreshAIProviderStatus() {
		const status = await apiRequest('/workspace/account/ai-config/status');
		aiProviderConfigured = status?.configured === true;
		if (status?.base_url) aiProviderBaseURL = status.base_url;
		if (status?.completion_model) aiCompletionModel = status.completion_model;
		if (status?.embedding_model) aiEmbeddingModel = status.embedding_model;
	}

	function connectionForChannel(channelID: string) {
		return bridgeConnections.find((connection) => connection.channel_id === channelID);
	}

	function platformName(platform: string) {
		return ({ whatsapp: 'WhatsApp', instagram: 'Instagram', messenger: 'Messenger', telegram: 'Telegram' } as Record<string, string>)[platform] || platform;
	}

	function channelName(channel: any) {
		return platformName(channel.type.replace('matrix_', ''));
	}

	onMount(async () => {
		if (initialSection && ['general', 'business_profile', 'ai_provider', 'users_permissions', 'channels', 'pipeline'].includes(initialSection)) {
			activeSection = initialSection as SettingsSection;
		}

		try {
			loading = !(workspace?.settingsReady ?? false);
			if (workspace) {
				await workspace.loadSettings(inbox?.currentUser);
				applyWorkspaceData(workspace.account);
				teamUsers = workspace.users;
				await Promise.all([refreshChannelsAndConnections(), refreshAIProviderStatus(), loadAccountSlug()]);
			} else {
				const [account, users] = await Promise.all([
					apiRequest('/workspace/account'),
					apiRequest('/workspace/users')
				]);
				applyWorkspaceData(account);
				teamUsers = users;
				await Promise.all([refreshChannelsAndConnections(), refreshAIProviderStatus(), loadAccountSlug()]);
			}
		} catch (err: any) {
			errorMsg = err?.message || 'Failed to load workspace settings.';
		} finally {
			loading = false;
		}
	});

	async function handleSaveGeneral() {
		if (!workspaceName.trim()) {
			errorMsg = 'Workspace name is required.';
			return;
		}

		saving = true;
		errorMsg = '';
		successMsg = '';

		try {
			// Update workspace name
			await apiRequest('/workspace/account', {
				method: 'PATCH',
				body: { name: workspaceName.trim() }
			});

			// Update settings object
			const settingsPayload = {
				timezone: defaultTimeZone,
				language,
				date_format: dateFormat,
				time_format: timeFormat,
				business_category: businessCategory,
				business_phone: businessPhone,
				business_email: businessEmail,
				business_address: businessAddress,
				business_website: businessWebsite,
				business_hours: businessHours,
				lead_tracking_enabled: leadTracking,
				unassigned_conversations_visible_to_members: unassignedVisible
			};

			await apiRequest('/workspace/account/settings', {
				method: 'PATCH',
				body: settingsPayload
			});
			await workspace?.refreshAccount();

			successMsg = 'Settings saved successfully';
			setTimeout(() => {
				successMsg = '';
			}, 3000);
		} catch (err: any) {
			errorMsg = err?.message || 'Failed to save settings';
		} finally {
			saving = false;
		}
	}

	async function handleSaveAIProvider() {
		if (!aiProviderConfigured && !aiProviderApiKey.trim()) {
			errorMsg = 'API key is required.';
			return;
		}
		if (!aiProviderBaseURL.trim() || !aiCompletionModel.trim() || !aiEmbeddingModel.trim()) {
			errorMsg = 'Base URL, completion model, and embedding model are required.';
			return;
		}

		savingAIProvider = true;
		errorMsg = '';
		successMsg = '';
		try {
			await apiRequest('/workspace/account/ai-config', {
				method: 'PUT',
				body: {
					config: JSON.stringify({
						api_key: aiProviderApiKey.trim(),
						base_url: aiProviderBaseURL.trim().replace(/\/$/, ''),
						completion_model: aiCompletionModel.trim(),
						embedding_model: aiEmbeddingModel.trim()
					})
				}
			});
			aiProviderApiKey = '';
			showAIProviderKey = false;
			aiProviderConfigured = true;
			successMsg = 'AI provider configuration saved.';
			setTimeout(() => successMsg = '', 3000);
		} catch (err: any) {
			errorMsg = err?.message || 'Failed to save AI provider configuration.';
		} finally {
			savingAIProvider = false;
		}
	}

	async function updateUserRole(userID: string, role: string) {
		errorMsg = '';
		try {
			await apiRequest(`/workspace/users/${userID}/role`, { method: 'PUT', body: { role } });
			if (workspace) {
				await workspace.refreshUsers();
				teamUsers = workspace.users;
			} else {
				teamUsers = await apiRequest('/workspace/users');
			}
			successMsg = 'User role updated.';
		} catch (err: any) {
			errorMsg = err.message || 'Failed to update user role.';
		}
	}

	async function connectChannel() {
		connectionBusy = true;
		errorMsg = '';
		try {
			activeConnection = await apiRequest('/bridge-connections', {
				method: 'POST',
				body: { platform: newChannelPlatform }
			});
			qrRefreshToken = Date.now();
			await refreshChannelsAndConnections(true);
		} catch (err: any) {
			errorMsg = err.message || 'Failed to connect channel.';
		} finally {
			connectionBusy = false;
		}
	}

	async function submitConnectionSecret() {
		if (!activeConnection || !connectionSecret.trim()) return;
		connectionBusy = true;
		errorMsg = '';
		try {
			activeConnection = await apiRequest(`/bridge-connections/${activeConnection.channel_id}/session`, {
				method: 'POST', body: { session: connectionSecret }
			});
			connectionSecret = '';
			await refreshChannelsAndConnections(true);
		} catch (err: any) {
			errorMsg = err.message || 'Failed to hand the session to the bridge.';
		} finally {
			connectionBusy = false;
		}
	}

	async function submitConnectionCode() {
		if (!activeConnection || !connectionCode.trim()) return;
		connectionBusy = true;
		errorMsg = '';
		try {
			activeConnection = await apiRequest(`/bridge-connections/${activeConnection.channel_id}/code`, {
				method: 'POST', body: { code: connectionCode }
			});
			connectionCode = '';
			await refreshChannelsAndConnections(true);
		} catch (err: any) {
			errorMsg = err.message || 'Failed to send the login response.';
		} finally {
			connectionBusy = false;
		}
	}

	async function disconnectChannel(channelID: string) {
		if (!confirm('Disconnect this channel? Existing conversations will remain available.')) return;
		try {
			await apiRequest(`/channels/${channelID}/disconnect`, { method: 'POST' });
			if (workspace) {
				await workspace.refreshChannels();
			}
			await refreshChannelsAndConnections();
			successMsg = 'Channel disconnected.';
		} catch (err: any) {
			errorMsg = err.message || 'Failed to disconnect channel.';
		}
	}

	$effect(() => {
		if (!showChannelModal || !activeConnection || ['connected', 'failed', 'cancelled'].includes(activeConnection.state)) return;
		const timer = window.setInterval(async () => {
			try {
				await refreshChannelsAndConnections(true);
				qrRefreshToken = Date.now();
			} catch {
				// Connection polling is best effort. The manual refresh control remains available.
			}
		}, 3500);
		return () => window.clearInterval(timer);
	});

	async function updateProductMode(mode: string) {
		if (mode === productMode) return;
		try {
			await apiRequest('/workspace/account/product-mode', { method: 'PATCH', body: { product_mode: mode } });
			productMode = mode;
			await workspace?.refreshAccount();
			successMsg = 'Workspace type updated.';
		} catch (err: any) {
			errorMsg = err.message || 'Failed to update workspace type.';
		}
	}

	async function deleteWorkspace() {
		if (deleteConfirmationInput !== workspaceName) return;
		saving = true;
		errorMsg = '';
		try {
			await apiRequest('/workspace/account', { method: 'DELETE', body: { confirmation: workspaceName } });
			await apiRequest('/auth/logout', { method: 'POST' }).catch(() => null);
			window.location.assign('/signup');
		} catch (err: any) {
			errorMsg = err.message || 'Failed to delete workspace.';
			saving = false;
		}
	}

	async function loadAccountSlug() {
		try {
			const res = await apiRequest('/workspace/account/slug');
			if (res?.slug) accountSlug = res.slug;
		} catch {}
	}

	async function handleSaveSlug() {
		if (!accountSlug.trim()) {
			errorMsg = 'Slug cannot be empty.';
			return;
		}
		savingSlug = true;
		errorMsg = '';
		successMsg = '';
		try {
			await apiRequest('/workspace/account/slug', {
				method: 'PUT',
				body: { slug: accountSlug.trim() }
			});
			successMsg = 'Workspace slug updated.';
			setTimeout(() => successMsg = '', 3000);
		} catch (err: any) {
			errorMsg = err.message || 'Failed to update workspace slug.';
		} finally {
			savingSlug = false;
		}
	}

	function generateNewUserPassword() {
		const chars = 'abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%';
		let pass = '';
		for (let i = 0; i < 12; i++) {
			pass += chars.charAt(Math.floor(Math.random() * chars.length));
		}
		newPassword = pass;
	}

	function generateResetPassword() {
		const chars = 'abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%';
		let pass = '';
		for (let i = 0; i < 12; i++) {
			pass += chars.charAt(Math.floor(Math.random() * chars.length));
		}
		resetNewPassword = pass;
	}

	async function handleAddUser() {
		userModalError = '';
		if (!newUsername.trim()) {
			userModalError = 'Username is required.';
			return;
		}
		if (!newPassword.trim()) {
			userModalError = 'Password is required.';
			return;
		}
		addingUser = true;
		try {
			const res = await apiRequest('/workspace/users', {
				method: 'POST',
				body: {
					username: newUsername.trim(),
					password: newPassword.trim(),
					role: newRole
				}
			});
			createdUserResult = {
				username: res.username || newUsername.trim(),
				role: res.role || newRole,
				plaintextPassword: res.password || newPassword.trim()
			};
			newUsername = '';
			newPassword = '';
			newRole = 'agent';
			showAddUserModal = false;
			if (workspace) {
				await workspace.refreshUsers();
				teamUsers = workspace.users;
			} else {
				teamUsers = await apiRequest('/workspace/users');
			}
		} catch (err: any) {
			userModalError = err.message || 'Failed to create user.';
		} finally {
			addingUser = false;
		}
	}

	async function handleDeleteUser() {
		if (!deleteUserTarget) return;
		deletingUser = true;
		errorMsg = '';
		try {
			await apiRequest(`/workspace/users/${deleteUserTarget.id}`, {
				method: 'DELETE'
			});
			showDeleteUserModal = false;
			deleteUserTarget = null;
			if (workspace) {
				await workspace.refreshUsers();
				teamUsers = workspace.users;
			} else {
				teamUsers = await apiRequest('/workspace/users');
			}
			successMsg = 'User deleted and conversations unassigned.';
			setTimeout(() => successMsg = '', 3000);
		} catch (err: any) {
			errorMsg = err.message || 'Failed to delete user.';
		} finally {
			deletingUser = false;
		}
	}

	async function handleResetPassword() {
		if (!resetUserTarget || !resetNewPassword.trim()) {
			resetPasswordError = 'New password is required.';
			return;
		}
		resettingPassword = true;
		resetPasswordError = '';
		try {
			await apiRequest(`/workspace/users/${resetUserTarget.id}/password`, {
				method: 'PUT',
				body: { password: resetNewPassword.trim() }
			});
			showResetPasswordModal = false;
			createdUserResult = {
				username: resetUserTarget.username || resetUserTarget.email,
				role: resetUserTarget.role,
				plaintextPassword: resetNewPassword.trim()
			};
			resetUserTarget = null;
			resetNewPassword = '';
			successMsg = 'Password reset successfully.';
			setTimeout(() => successMsg = '', 3000);
		} catch (err: any) {
			resetPasswordError = err.message || 'Failed to reset password.';
		} finally {
			resettingPassword = false;
		}
	}

	async function copyPasswordText(text: string) {
		try {
			await navigator.clipboard.writeText(text);
			copiedPassword = true;
			setTimeout(() => copiedPassword = false, 2000);
		} catch {}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<div class="flex-1 flex flex-col h-full overflow-y-auto bg-white p-8" aria-busy={loading}>

	<!-- Top Title Header -->
	<div class="mb-7">
		<h1 class="text-2xl font-medium text-slate-900 tracking-tight font-sans">Settings</h1>
		<p class="text-xs text-slate-500 mt-1">Manage your workspace and preferences.</p>
	</div>

	<!-- Alert Banners -->
	{#if successMsg}
		<div class="mb-5 px-4 py-3 bg-emerald-50 border border-emerald-200 text-emerald-700 text-xs rounded-xl flex items-center justify-between">
			<div class="flex items-center gap-2">
				<svg class="w-4 h-4 text-emerald-600 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
					<path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
				</svg>
				<span>{successMsg}</span>
			</div>
			<button onclick={() => successMsg = ''} class="text-emerald-500 hover:text-emerald-700 text-sm font-medium">×</button>
		</div>
	{/if}

	{#if errorMsg}
		<div class="mb-5 px-4 py-3 bg-rose-50 border border-rose-200 text-rose-700 text-xs rounded-xl flex items-center justify-between">
			<div class="flex items-center gap-2">
				<svg class="w-4 h-4 text-rose-600 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
					<circle cx="12" cy="12" r="10" />
					<line x1="12" y1="8" x2="12" y2="12" />
					<line x1="12" y1="16" x2="12.01" y2="16" />
				</svg>
				<span>{errorMsg}</span>
			</div>
			<button onclick={() => errorMsg = ''} class="text-rose-500 hover:text-rose-700 text-sm font-medium">×</button>
		</div>
	{/if}

	{#if loading}
		<div role="status" class="py-10 text-xs text-slate-500">Loading workspace settings…</div>
	{:else}
		<!-- 3-Column Layout: Subnav | Active Section Content | Right Info Cards -->
		<div class="grid grid-cols-12 gap-8 items-start">

		<SettingsSidebar bind:activeSection {productMode} />

		<!-- ================= CENTER CONTENT PANEL ================= -->
		<div
			class="col-span-12 md:col-span-9 lg:col-span-6"
			role="tabpanel"
				id={activeSection === 'general' ? 'settings-panel-general' : activeSection === 'business_profile' ? 'settings-panel-business_profile' : activeSection === 'ai_provider' ? 'settings-panel-ai_provider' : activeSection === 'users_permissions' ? 'settings-panel-users_permissions' : activeSection === 'pipeline' ? 'settings-panel-pipeline' : 'settings-panel-channels'}
		>

			<!-- SECTION: GENERAL -->
			{#if activeSection === 'general'}
				<div class="space-y-6">
					<div>
						<h2 class="text-base font-medium text-slate-900">General</h2>
					</div>

					<div class="space-y-5 text-xs">
						<!-- Workspace Name Field -->
						<div class="space-y-1.5">
							<label for="inputWorkspaceName" class="block font-medium text-slate-700">Workspace name</label>
							<input
								id="inputWorkspaceName"
								type="text"
								bind:value={workspaceName}
								class="wf-input"
								placeholder="Enter workspace name"
							/>
						</div>

						<!-- Default Time Zone Select -->
						<div class="space-y-1.5">
							<label for="selectTimeZone" class="block font-medium text-slate-700">Default time zone</label>
							<div class="relative">
								<select
									id="selectTimeZone"
									bind:value={defaultTimeZone}
									class="wf-select"
								>
									{#if !supportedTimeZones.includes(defaultTimeZone)}
										<option value={defaultTimeZone}>{formatTimeZoneLabel(defaultTimeZone)}</option>
									{/if}
									{#each supportedTimeZones as zone}
										<option value={zone}>{formatTimeZoneLabel(zone)}</option>
									{/each}
								</select>
								<div class="absolute inset-y-0 right-0 flex items-center pr-3 pointer-events-none text-slate-400">
									<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
										<path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
									</svg>
								</div>
							</div>
						</div>

						<!-- Language Select -->
						<div class="space-y-1.5">
							<label for="selectLanguage" class="block font-medium text-slate-700">Language</label>
							<div class="relative">
								<select
									id="selectLanguage"
									bind:value={language}
									class="wf-select"
								>
									<option value="English">English</option>
									<option value="Spanish">Spanish (Español)</option>
									<option value="French">French (Français)</option>
									<option value="German">German (Deutsch)</option>
									<option value="Italian">Italian (Italiano)</option>
									<option value="Portuguese">Portuguese (Português)</option>
									<option value="Japanese">Japanese (日本語)</option>
									<option value="Chinese">Chinese (Simplified)</option>
									<option value="Arabic">Arabic (العربية)</option>
								</select>
								<div class="absolute inset-y-0 right-0 flex items-center pr-3 pointer-events-none text-slate-400">
									<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
										<path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
									</svg>
								</div>
							</div>
						</div>

						<!-- Date Format Select -->
						<div class="space-y-1.5">
							<label for="selectDateFormat" class="block font-medium text-slate-700">Date format</label>
							<div class="relative">
								<select
									id="selectDateFormat"
									bind:value={dateFormat}
									class="wf-select"
								>
									<option value="DD MMM YYYY">DD MMM YYYY (e.g. 20 May 2024)</option>
									<option value="MM/DD/YYYY">MM/DD/YYYY (e.g. 05/20/2024)</option>
									<option value="DD/MM/YYYY">DD/MM/YYYY (e.g. 20/05/2024)</option>
									<option value="YYYY-MM-DD">YYYY-MM-DD (e.g. 2024-05-20)</option>
								</select>
								<div class="absolute inset-y-0 right-0 flex items-center pr-3 pointer-events-none text-slate-400">
									<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
										<path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
									</svg>
								</div>
							</div>
						</div>

						<!-- Time Format Radio Options -->
						<div class="space-y-2 pt-1">
							<span class="block font-medium text-slate-700">Time format</span>
							<div class="flex items-center gap-6">
								<label class="flex items-center gap-2 cursor-pointer select-none">
									<input
										type="radio"
										name="timeFormat"
										value="12"
										checked={timeFormat === '12'}
										onchange={() => timeFormat = '12'}
										class="w-4 h-4 text-blue-600 border-slate-300 focus:ring-blue-500 accent-blue-600"
									/>
									<span class="text-xs text-slate-700 font-medium">12 hour (1:30 PM)</span>
								</label>

								<label class="flex items-center gap-2 cursor-pointer select-none">
									<input
										type="radio"
										name="timeFormat"
										value="24"
										checked={timeFormat === '24'}
										onchange={() => timeFormat = '24'}
										class="w-4 h-4 text-blue-600 border-slate-300 focus:ring-blue-500 accent-blue-600"
									/>
									<span class="text-xs text-slate-700 font-medium">24 hour (13:30)</span>
								</label>
							</div>
						</div>

							<!-- Save Changes Button -->
							<div class="space-y-2 border-t border-slate-100 pt-5">
								<span class="block font-medium text-slate-700">Workspace type</span>
								<div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
									<label class="flex cursor-pointer items-center gap-2 rounded-xl border p-3 text-xs {productMode === 'full_workspace' ? 'border-blue-500 bg-blue-50/50' : 'border-slate-200'}">
										<input type="radio" name="workspace-mode" checked={productMode === 'full_workspace'} onchange={() => updateProductMode('full_workspace')} />
										<span><span class="block font-medium text-slate-800">Full workspace</span>Inbox and lead tracking</span>
									</label>
									<label class="flex cursor-pointer items-center gap-2 rounded-xl border p-3 text-xs {productMode === 'chatbot_only' ? 'border-blue-500 bg-blue-50/50' : 'border-slate-200'}">
										<input type="radio" name="workspace-mode" checked={productMode === 'chatbot_only'} onchange={() => updateProductMode('chatbot_only')} />
										<span><span class="block font-medium text-slate-800">Chatbot only</span>Automated replies only</span>
									</label>
								</div>
							</div>

							<!-- Save Changes Button -->
						<div class="pt-6 flex justify-end">
							<button
								onclick={handleSaveGeneral}
								disabled={saving}
								class="wf-button-primary px-5 py-2.5"
							>
								{#if saving}
									<svg class="w-3.5 h-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
										<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
										<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
									</svg>
									<span>Saving...</span>
								{:else}
									<span>Save changes</span>
								{/if}
							</button>
						</div>
					</div>
				</div>

			<!-- SECTION: BUSINESS PROFILE -->
			{:else if activeSection === 'business_profile'}
				<div class="space-y-6">
					<div>
						<h2 class="text-base font-medium text-slate-900">Business profile</h2>
						<p class="text-xs text-slate-500 mt-0.5">Details used by AI and customer communications</p>
					</div>

					<div class="space-y-4 text-xs">
						<div class="space-y-1.5">
							<label for="inputCategory" class="block font-medium text-slate-700">Business category</label>
							<input id="inputCategory" type="text" bind:value={businessCategory} class="wf-input" />
						</div>

						<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
							<div class="space-y-1.5">
								<label for="inputPhone" class="block font-medium text-slate-700">Phone number</label>
								<input id="inputPhone" type="text" bind:value={businessPhone} class="wf-input" />
							</div>
							<div class="space-y-1.5">
								<label for="inputEmail" class="block font-medium text-slate-700">Public email</label>
								<input id="inputEmail" type="email" bind:value={businessEmail} class="wf-input" />
							</div>
						</div>

						<div class="space-y-1.5">
							<label for="inputAddress" class="block font-medium text-slate-700">Address / Location</label>
							<input id="inputAddress" type="text" bind:value={businessAddress} class="wf-input" />
						</div>

						<div class="space-y-1.5">
							<label for="inputHours" class="block font-medium text-slate-700">Operating hours</label>
							<input id="inputHours" type="text" bind:value={businessHours} class="wf-input" />
						</div>

						<div class="pt-4 flex justify-end">
							<button onclick={handleSaveGeneral} class="wf-button-primary px-5 py-2.5">
								Save profile
							</button>
						</div>
					</div>
				</div>

			<!-- SECTION: AI PROVIDER -->
			{:else if activeSection === 'ai_provider'}
				<form class="space-y-6" onsubmit={(event) => { event.preventDefault(); void handleSaveAIProvider(); }}>
					<div>
						<div class="flex flex-wrap items-center gap-2">
							<h2 class="text-base font-medium text-slate-900">AI provider</h2>
							<span class="rounded-md px-2 py-0.5 text-[10px] font-medium {aiProviderConfigured ? 'bg-emerald-50 text-emerald-700' : 'bg-amber-50 text-amber-700'}">
								{aiProviderConfigured ? 'Configured' : 'Not configured'}
							</span>
						</div>
						<p class="mt-1 text-xs leading-relaxed text-slate-500">Connect an OpenAI-compatible provider with your own API key. Credentials are encrypted before storage.</p>
					</div>

					{#if aiProviderConfigured}
						<div class="rounded-xl border border-emerald-200 bg-emerald-50/60 p-4 text-xs leading-relaxed text-emerald-800">
							A provider is connected. The saved key is write-only and will be retained unless you enter a replacement. The URL and model defaults can be edited independently.
						</div>
					{/if}

					<div class="space-y-4 text-xs">
						<div class="space-y-1.5">
							<label for="aiSettingsApiKey" class="block font-medium text-slate-700">{aiProviderConfigured ? 'New API key' : 'API key'}</label>
							<div class="relative">
								<input id="aiSettingsApiKey" type={showAIProviderKey ? 'text' : 'password'} autocomplete="new-password" bind:value={aiProviderApiKey} class="wf-input pr-16" placeholder={aiProviderConfigured ? 'Enter a replacement key' : 'Enter your provider API key'} />
								<button type="button" onclick={() => showAIProviderKey = !showAIProviderKey} class="absolute inset-y-0 right-0 px-3 text-[11px] font-medium text-slate-500 hover:text-slate-800" aria-label={showAIProviderKey ? 'Hide API key' : 'Show API key'}>{showAIProviderKey ? 'Hide' : 'Show'}</button>
							</div>
							<p class="text-[11px] leading-relaxed text-slate-500">The key is write-only and will not appear here again after saving.</p>
						</div>

						<div class="space-y-1.5">
							<label for="aiSettingsBaseURL" class="block font-medium text-slate-700">OpenAI-compatible base URL</label>
							<input id="aiSettingsBaseURL" type="url" bind:value={aiProviderBaseURL} class="wf-input" required />
						</div>

						<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
							<div class="space-y-1.5">
								<label for="aiSettingsCompletionModel" class="block font-medium text-slate-700">Completion model</label>
								<input id="aiSettingsCompletionModel" bind:value={aiCompletionModel} class="wf-input" required />
							</div>
							<div class="space-y-1.5">
								<label for="aiSettingsEmbeddingModel" class="block font-medium text-slate-700">Embedding model</label>
								<input id="aiSettingsEmbeddingModel" bind:value={aiEmbeddingModel} class="wf-input" required />
							</div>
						</div>
					</div>

					<div class="flex justify-end border-t border-slate-100 pt-5">
						<button type="submit" disabled={savingAIProvider || (!aiProviderConfigured && !aiProviderApiKey.trim())} class="wf-button-primary px-5 py-2.5 disabled:cursor-not-allowed disabled:opacity-50">
							{savingAIProvider ? 'Saving...' : aiProviderConfigured ? 'Save changes' : 'Save provider'}
						</button>
					</div>
				</form>

			<!-- SECTION: USERS & PERMISSIONS -->
			{:else if activeSection === 'users_permissions'}
				<div class="space-y-6">
					<div class="flex items-center justify-between">
						<div>
							<h2 class="text-base font-medium text-slate-900">Users & permissions</h2>
							<p class="text-xs text-slate-500 mt-0.5">Manage team members and their workspace access</p>
						</div>
						<button
							onclick={() => { userModalError = ''; generateNewUserPassword(); showAddUserModal = true; }}
							class="px-3.5 py-2 bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium rounded-xl flex items-center gap-1.5 transition cursor-pointer shadow-xs"
						>
							<svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
								<path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
							</svg>
							<span>Add user</span>
						</button>
					</div>

					<!-- Workspace Slug Box -->
					<div class="p-4 bg-slate-50/80 border border-slate-200 rounded-2xl space-y-2.5">
						<div class="flex items-center justify-between">
							<label for="settings-workspace-slug" class="text-xs font-medium text-slate-900">Workspace Login Prefix (Slug)</label>
							<button
								type="button"
								onclick={handleSaveSlug}
								disabled={savingSlug}
								class="px-3 py-1 bg-white border border-slate-200 hover:border-slate-300 rounded-lg text-xs font-medium text-slate-700 transition cursor-pointer disabled:opacity-50"
							>
								{savingSlug ? 'Saving...' : 'Update slug'}
							</button>
						</div>
						<input
							id="settings-workspace-slug"
							type="text"
							bind:value={accountSlug}
							placeholder="company-prefix"
							class="w-full px-3 py-2 bg-white border border-slate-200 rounded-xl text-xs font-mono text-slate-900 focus:border-blue-600 focus:ring-1 focus:ring-blue-100 outline-none"
						/>
						<p class="text-[11px] text-slate-500 font-normal">
							Agents log in with: <span class="font-mono font-medium text-slate-800 bg-white px-1.5 py-0.5 rounded border border-slate-200">{accountSlug || 'prefix'}-[username]</span>
						</p>
					</div>

					<!-- Team Users Table -->
					<div class="border border-slate-200 rounded-2xl overflow-hidden divide-y divide-slate-100 text-xs bg-white">
						{#each teamUsers as user}
							<div class="p-4 flex items-center justify-between hover:bg-slate-50/50 transition">
								<div class="flex items-center gap-3 min-w-0">
									<div class="w-8 h-8 rounded-full bg-blue-100 text-blue-700 font-medium flex items-center justify-center text-xs shrink-0">
										{user.username ? user.username.charAt(0).toUpperCase() : (user.name ? user.name.charAt(0).toUpperCase() : user.email.charAt(0).toUpperCase())}
									</div>
									<div class="min-w-0">
										<div class="font-medium text-slate-800 truncate">
											{user.username || user.name || user.email.split('@')[0]}
										</div>
										<div class="text-[11px] text-slate-400 font-mono truncate">
											{user.email ? user.email : (accountSlug ? `${accountSlug}-${user.username}` : user.username)}
										</div>
									</div>
								</div>

								<div class="flex items-center gap-2.5 shrink-0">
									<select
										aria-label="Role for {user.username || user.email}"
										value={user.role}
										onchange={(event) => updateUserRole(user.id, (event.currentTarget as HTMLSelectElement).value)}
										class="rounded-lg border border-slate-200 bg-white px-2 py-1 text-[11px] font-medium capitalize text-slate-700 focus:border-blue-500 focus:outline-none cursor-pointer"
									>
										<option value="agent">Agent</option>
										<option value="manager">Manager</option>
									</select>

									<button
										type="button"
										class="px-2.5 py-1 text-[11px] font-medium text-slate-600 hover:text-slate-900 bg-slate-100 hover:bg-slate-200 rounded-lg transition cursor-pointer"
										onclick={() => { resetUserTarget = user; resetPasswordError = ''; generateResetPassword(); showResetPasswordModal = true; }}
										title="Reset Password"
									>
										Reset password
									</button>

									{#if user.id !== inbox?.currentUser?.id}
										<button
											type="button"
											class="p-1 text-slate-400 hover:text-rose-600 rounded-lg hover:bg-rose-50 transition cursor-pointer"
											onclick={() => { deleteUserTarget = user; showDeleteUserModal = true; }}
											title="Delete user"
										>
											<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
												<path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
											</svg>
										</button>
									{/if}
								</div>
							</div>
						{/each}
					</div>
				</div>

				<!-- SECTION: CHANNELS -->
				{:else if activeSection === 'channels'}
					<div class="space-y-5">
						<div class="flex items-start justify-between gap-3">
							<div><h2 class="text-base font-medium text-slate-900">Connected channels</h2><p class="mt-0.5 text-xs text-slate-500">Link each account through its mautrix bridge, then monitor its connection here.</p></div>
							<button onclick={() => showChannelModal = true} class="rounded-xl border border-blue-200 bg-blue-50 px-3 py-2 text-xs font-medium text-blue-600 hover:bg-blue-100">Connect channel</button>
						</div>
						{#if channelsList.length === 0}
							<p class="rounded-xl border border-slate-200 bg-slate-50 px-3 py-5 text-center text-xs text-slate-500">No channels connected yet.</p>
						{:else}
							<div class="space-y-2">
								{#each channelsList as channel (channel.id)}
									{@const connection = connectionForChannel(channel.id)}
									<div class="flex items-center gap-3 rounded-xl border border-slate-200 bg-white p-3">
										<ChannelBadge channel={channel.type} size="md" />
										<div class="min-w-0 flex-1"><div class="truncate text-xs font-medium text-slate-800">{channelName(channel)}</div><div class="mt-0.5 truncate text-[11px] text-slate-400">{connection?.detail || channel.status_detail || channel.bridge_identity || 'Ready to connect'}</div></div>
										{#if connection && !['connected', 'cancelled'].includes(connection.state)}
											<button onclick={() => { activeConnection = connection; showChannelModal = true; qrRefreshToken = Date.now(); }} class="rounded-lg px-2.5 py-1.5 text-xs font-medium text-blue-600 hover:bg-blue-50">Continue</button>
										{/if}
										<button onclick={() => disconnectChannel(channel.id)} class="rounded-lg px-2.5 py-1.5 text-xs font-medium text-rose-600 hover:bg-rose-50">Disconnect</button>
									</div>
								{/each}
							</div>
						{/if}
					</div>

				{:else if activeSection === 'pipeline'}
					<PipelineSettings {workspace} />
			{/if}

		</div>

		<SettingsInfoPanel
			{currentPlan}
			{storageUsedGB}
			{storageTotalGB}
			{storagePercent}
			onManagePlan={() => showPlanModal = true}
			onDelete={() => showDeleteModal = true}
		/>

	</div>

<!-- Connect channel modal -->
{#if showChannelModal}
	<div class="wf-modal-backdrop">
		<div class="wf-modal max-w-lg" role="dialog" aria-modal="true" aria-labelledby="connect-channel-title">
			<div class="flex items-center justify-between gap-4"><div><h3 id="connect-channel-title" class="text-sm font-medium text-slate-900">{activeConnection ? `Connect ${platformName(activeConnection.platform)}` : 'Connect a channel'}</h3><p class="mt-1 text-xs text-slate-500">{activeConnection ? activeConnection.detail : 'WhatFunnel creates an isolated Matrix bridge user and guides the provider-specific login.'}</p></div><button aria-label="Close channel dialog" onclick={closeModal} class="text-lg text-slate-400 hover:text-slate-600">×</button></div>

			{#if !activeConnection}
				<label class="block text-xs font-medium text-slate-700">Channel
					<select bind:value={newChannelPlatform} class="wf-select mt-1.5"><option value="whatsapp">WhatsApp</option><option value="instagram">Instagram</option><option value="messenger">Messenger</option><option value="telegram">Telegram</option></select>
				</label>
				<p class="rounded-xl border border-slate-200 bg-slate-50 p-3 text-[11px] leading-5 text-slate-600">The bridge runs as a linked device or authenticated session for this channel. Connection credentials remain server-side and encrypted at rest.</p>
				<div class="flex justify-end gap-2"><button onclick={closeModal} class="wf-button px-3 py-2 text-slate-600 hover:bg-slate-100">Cancel</button><button onclick={connectChannel} disabled={connectionBusy} class="wf-button-primary px-3 py-2">{connectionBusy ? 'Starting…' : 'Continue'}</button></div>
			{:else if activeConnection.state === 'awaiting_scan'}
				<div class="space-y-4">
					<div class="mx-auto flex h-52 w-52 items-center justify-center rounded-2xl border border-slate-200 bg-white p-3">
						<img class="h-full w-full object-contain" src={`/api-gateway/bridge-connections/${activeConnection.channel_id}/qr?refresh=${qrRefreshToken}`} alt={`QR code for ${platformName(activeConnection.platform)} connection`} />
					</div>
					<p class="rounded-xl border border-blue-100 bg-blue-50 p-3 text-[11px] leading-5 text-blue-800">{activeConnection.platform === 'telegram' ? 'In Telegram, open Settings, Devices, then Link Desktop Device. Scan this code and complete any two-factor prompt.' : 'In WhatsApp, open Settings, Linked devices, then Link a device. Scan this code with your phone.'}</p>
					<div class="flex justify-end gap-2"><button onclick={() => { qrRefreshToken = Date.now(); refreshChannelsAndConnections(true); }} class="wf-button px-3 py-2 text-slate-600 hover:bg-slate-100">Refresh QR</button><button onclick={closeModal} class="wf-button-primary px-3 py-2">I’ve scanned it</button></div>
				</div>
			{:else if activeConnection.state === 'awaiting_code'}
				<label class="block text-xs font-medium text-slate-700">Telegram response
					<input bind:value={connectionCode} autocomplete="one-time-code" class="wf-input mt-1.5" placeholder="Verification code or 2FA password" />
					<span class="mt-1.5 block text-[11px] font-normal leading-4 text-slate-500">Enter exactly what the bridge requested. It is forwarded to the private bridge-management room and is not stored by WhatFunnel.</span>
				</label>
				<div class="flex justify-end gap-2"><button onclick={closeModal} class="wf-button px-3 py-2 text-slate-600 hover:bg-slate-100">Cancel</button><button onclick={submitConnectionCode} disabled={connectionBusy || !connectionCode.trim()} class="wf-button-primary px-3 py-2">{connectionBusy ? 'Sending…' : 'Submit'}</button></div>
			{:else if activeConnection.state === 'awaiting_session'}
				<div class="space-y-3 text-xs text-slate-600 leading-5">
					<p>Open {activeConnection.platform === 'instagram' ? 'instagram.com' : 'messenger.com'} in a private browser window and sign in. In browser developer tools, copy an authenticated GraphQL request as POSIX cURL.</p>
					<label class="block text-xs font-medium text-slate-700">Bridge session hand-off
						<textarea bind:value={connectionSecret} autocomplete="off" spellcheck="false" class="wf-input mt-1.5 min-h-32 font-mono text-[11px]" placeholder="Paste the copied cURL request"></textarea>
						<span class="mt-1.5 block text-[11px] font-normal leading-4 text-slate-500">This value is sent once over TLS and is not written to WhatFunnel’s database or logs. Your Matrix operator controls bridge-management room retention and encryption.</span>
					</label>
				</div>
				<div class="flex justify-end gap-2"><button onclick={closeModal} class="wf-button px-3 py-2 text-slate-600 hover:bg-slate-100">Cancel</button><button onclick={submitConnectionSecret} disabled={connectionBusy || !connectionSecret.trim()} class="wf-button-primary px-3 py-2">{connectionBusy ? 'Handing off…' : 'Connect'}</button></div>
			{:else if activeConnection.state === 'connected'}
				<div class="rounded-xl border border-emerald-200 bg-emerald-50 p-4 text-xs leading-5 text-emerald-800">{platformName(activeConnection.platform)} is connected. New chats will sync through the bridge.</div>
				<div class="flex justify-end"><button onclick={closeModal} class="wf-button-primary px-3 py-2">Done</button></div>
			{:else if activeConnection.state === 'failed'}
				<div class="rounded-xl border border-rose-200 bg-rose-50 p-4 text-xs leading-5 text-rose-800">{activeConnection.detail || 'The bridge could not complete this login.'}</div>
				<div class="flex justify-end"><button onclick={closeModal} class="wf-button-primary px-3 py-2">Close</button></div>
			{:else}
				<div class="rounded-xl border border-slate-200 bg-slate-50 p-4 text-xs text-slate-600">The bridge is verifying this connection. This dialog checks for updates automatically.</div>
				<div class="flex justify-end"><button onclick={() => refreshChannelsAndConnections(true)} class="wf-button-primary px-3 py-2">Check status</button></div>
			{/if}
		</div>
	</div>
{/if}
	{/if}

</div>

<!-- Plan Modal -->
{#if showPlanModal}
	<div class="wf-modal-backdrop">
		<div class="wf-modal" role="dialog" aria-modal="true" aria-labelledby="workspace-plan-title">
			<div class="flex items-center justify-between">
				<h3 id="workspace-plan-title" class="text-sm font-medium text-slate-900">Workspace Plan</h3>
				<button onclick={() => showPlanModal = false} aria-label="Close workspace plan" class="text-slate-400 hover:text-slate-600 text-lg font-medium cursor-pointer">×</button>
			</div>
			<div class="p-4 rounded-xl bg-blue-50/60 border border-blue-100 space-y-2 text-xs">
				<div class="flex justify-between font-medium text-slate-900">
					<span>Pro Plan</span>
					<span class="text-blue-600">$49 / mo</span>
				</div>
				<p class="text-slate-600 text-[11px]">Unlimited channels, 10 team seats, 20 GB media storage, and autonomous AI replies.</p>
			</div>
			<div class="flex justify-end pt-2">
				<button onclick={() => showPlanModal = false} class="wf-button-primary">
					Done
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Delete Confirmation Modal -->
{#if showDeleteModal}
	<div class="wf-modal-backdrop">
		<div class="wf-modal" role="dialog" aria-modal="true" aria-labelledby="delete-workspace-title">
			<div class="flex items-center gap-3 text-red-600">
				<div class="w-10 h-10 rounded-xl bg-red-50 flex items-center justify-center shrink-0">
					<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
					</svg>
				</div>
				<div>
					<h3 id="delete-workspace-title" class="text-sm font-medium text-slate-900">Delete Workspace</h3>
					<p class="text-xs text-slate-500">This will permanently delete all leads, messages, and settings.</p>
				</div>
			</div>

			<div class="text-xs text-slate-600 space-y-2">
				<p>Please type <span class="font-medium text-slate-900 select-all">{workspaceName}</span> to confirm:</p>
				<input
					type="text"
					bind:value={deleteConfirmationInput}
					placeholder={workspaceName}
					class="wf-input focus:border-red-500 focus:ring-red-100"
				/>
			</div>

			<div class="flex items-center justify-end gap-3 pt-2">
				<button
					onclick={() => { showDeleteModal = false; deleteConfirmationInput = ''; }}
					class="px-4 py-2 text-xs font-medium text-slate-600 hover:bg-slate-100 rounded-xl transition cursor-pointer"
				>
					Cancel
				</button>
				<button
					disabled={deleteConfirmationInput !== workspaceName}
					onclick={deleteWorkspace}
					class="wf-button-danger"
				>
					Delete permanently
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Add User Modal -->
{#if showAddUserModal}
	<div class="wf-modal-backdrop">
		<div class="wf-modal" role="dialog" aria-modal="true" aria-labelledby="add-team-member-title">
			<h3 id="add-team-member-title" class="text-sm font-medium text-slate-900">Add Team Member</h3>
			<div class="space-y-3.5 text-xs">
				<div class="space-y-1">
					<label for="newUsernameInput" class="font-medium text-slate-700">Username</label>
					<input id="newUsernameInput" type="text" bind:value={newUsername} placeholder="e.g. john" class="wf-input" />
					<p class="text-[11px] text-slate-400">Login username: <span class="font-mono">{accountSlug || 'prefix'}-{newUsername || '[username]'}</span></p>
				</div>
				<div class="space-y-1">
					<div class="flex items-center justify-between">
						<label for="newPasswordInput" class="font-medium text-slate-700">Initial Password</label>
						<button type="button" onclick={generateNewUserPassword} class="text-[11px] text-blue-600 hover:underline cursor-pointer">Generate</button>
					</div>
					<input id="newPasswordInput" type="text" bind:value={newPassword} placeholder="Password" class="wf-input font-mono" />
				</div>
				<div class="space-y-1">
					<label for="newRoleSelect" class="font-medium text-slate-700">Role</label>
					<select id="newRoleSelect" bind:value={newRole} class="wf-select">
						<option value="agent">Agent</option>
						<option value="manager">Manager</option>
					</select>
				</div>
				{#if userModalError}
					<p class="text-xs text-rose-600 font-medium">{userModalError}</p>
				{/if}
			</div>
			<div class="flex items-center justify-end gap-3 pt-2">
				<button onclick={closeModal} class="px-4 py-2 text-xs font-medium text-slate-600 hover:bg-slate-100 rounded-xl transition cursor-pointer">
					Cancel
				</button>
				<button onclick={handleAddUser} disabled={addingUser || !newUsername.trim() || !newPassword.trim()} class="wf-button-primary disabled:opacity-50">
					{addingUser ? 'Creating...' : 'Create User'}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Created / Reset User Credentials Modal (Plaintext shown once) -->
{#if createdUserResult}
	<div class="wf-modal-backdrop">
		<div class="wf-modal max-w-md" role="dialog" aria-modal="true" aria-labelledby="user-credentials-title">
			<div class="flex items-center gap-3">
				<div class="w-9 h-9 rounded-xl bg-emerald-50 text-emerald-600 flex items-center justify-center shrink-0">
					<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
					</svg>
				</div>
				<div>
					<h3 id="user-credentials-title" class="text-sm font-medium text-slate-900">User Credentials</h3>
					<p class="text-xs text-slate-500">Save these credentials now. The password will not be shown again.</p>
				</div>
			</div>

			<div class="space-y-3 p-4 bg-slate-50 border border-slate-200 rounded-xl text-xs font-mono">
				<div class="flex items-center justify-between">
					<span class="text-slate-500 font-sans text-[11px]">Login Identifier:</span>
					<span class="font-medium text-slate-900 font-mono">{accountSlug ? `${accountSlug}-${createdUserResult.username}` : createdUserResult.username}</span>
				</div>
				<div class="flex items-center justify-between">
					<span class="text-slate-500 font-sans text-[11px]">Role:</span>
					<span class="font-medium text-slate-900 font-sans capitalize">{createdUserResult.role}</span>
				</div>
				{#if createdUserResult.plaintextPassword}
					<div class="flex items-center justify-between">
						<span class="text-slate-500 font-sans text-[11px]">Password:</span>
						<span class="font-medium text-blue-700 bg-blue-50 px-2 py-0.5 rounded border border-blue-200">{createdUserResult.plaintextPassword}</span>
					</div>
				{/if}
			</div>

			<div class="flex items-center justify-end gap-3 pt-2">
				{#if createdUserResult.plaintextPassword}
					<button
						type="button"
						onclick={() => copyPasswordText(`Login: ${accountSlug ? `${accountSlug}-${createdUserResult!.username}` : createdUserResult!.username}\nPassword: ${createdUserResult!.plaintextPassword}`)}
						class="px-4 py-2 text-xs font-medium text-slate-700 bg-slate-100 hover:bg-slate-200 rounded-xl transition cursor-pointer"
					>
						{copiedPassword ? 'Copied!' : 'Copy Credentials'}
					</button>
				{/if}
				<button onclick={closeModal} class="wf-button-primary">
					Done
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Reset Password Modal -->
{#if showResetPasswordModal && resetUserTarget}
	<div class="wf-modal-backdrop">
		<div class="wf-modal" role="dialog" aria-modal="true" aria-labelledby="reset-user-password-title">
			<h3 id="reset-user-password-title" class="text-sm font-medium text-slate-900">Reset User Password</h3>
			<p class="text-xs text-slate-500">Set a new password for <span class="font-medium text-slate-800">{resetUserTarget.username || resetUserTarget.email}</span>.</p>

			<div class="space-y-3.5 text-xs">
				<div class="space-y-1">
					<div class="flex items-center justify-between">
						<label for="resetPasswordInput" class="font-medium text-slate-700">New Password</label>
						<button type="button" onclick={generateResetPassword} class="text-[11px] text-blue-600 hover:underline cursor-pointer">Generate</button>
					</div>
					<input id="resetPasswordInput" type="text" bind:value={resetNewPassword} placeholder="New password" class="wf-input font-mono" />
				</div>
				{#if resetPasswordError}
					<p class="text-xs text-rose-600 font-medium">{resetPasswordError}</p>
				{/if}
			</div>

			<div class="flex items-center justify-end gap-3 pt-2">
				<button onclick={closeModal} class="px-4 py-2 text-xs font-medium text-slate-600 hover:bg-slate-100 rounded-xl transition cursor-pointer">
					Cancel
				</button>
				<button onclick={handleResetPassword} disabled={resettingPassword || !resetNewPassword.trim()} class="wf-button-primary disabled:opacity-50">
					{resettingPassword ? 'Updating...' : 'Set Password'}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Delete User Confirmation Modal -->
{#if showDeleteUserModal && deleteUserTarget}
	<div class="wf-modal-backdrop">
		<div class="wf-modal" role="dialog" aria-modal="true" aria-labelledby="delete-user-modal-title">
			<h3 id="delete-user-modal-title" class="text-sm font-medium text-slate-900">Delete User</h3>
			<p class="text-xs text-slate-600 leading-relaxed">
				Are you sure you want to delete <span class="font-medium text-slate-900">{deleteUserTarget.username || deleteUserTarget.email}</span>?
				Any conversations currently assigned to them will be unassigned. This action cannot be undone.
			</p>
			<div class="flex items-center justify-end gap-3 pt-2">
				<button onclick={closeModal} class="px-4 py-2 text-xs font-medium text-slate-600 hover:bg-slate-100 rounded-xl transition cursor-pointer">
					Cancel
				</button>
				<button onclick={handleDeleteUser} disabled={deletingUser} class="wf-button-danger disabled:opacity-50">
					{deletingUser ? 'Deleting...' : 'Delete User'}
				</button>
			</div>
		</div>
	</div>
{/if}
