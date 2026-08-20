import { apiRequest } from '$lib/api';

/**
 * Workspace data is shared by all dashboard tabs. Keeping it here removes
 * duplicate requests when moving between Inbox, Leads, and Settings.
 */
export class WorkspaceState {
	account = $state<any | null>(null);
	pipeline = $state<any | null>(null);
	users = $state<any[]>([]);
	channels = $state<any[]>([]);
	coreReady = $state(false);
	settingsReady = $state(false);
	coreLoading = $state(false);
	settingsLoading = $state(false);

	private corePromise: Promise<void> | null = null;
	private settingsPromise: Promise<void> | null = null;

	async loadCore(currentUser?: any) {
		if (this.coreReady) return;
		if (this.corePromise) return this.corePromise;

		this.coreLoading = true;
		this.corePromise = (async () => {
			const [account, pipelines, users] = await Promise.all([
				apiRequest('/workspace/account'),
				apiRequest('/workspace/pipelines'),
				currentUser?.role === 'admin' && (!Array.isArray(this.users) || this.users.length === 0) ? apiRequest('/workspace/users') : Promise.resolve(this.users)
			]);
			this.account = account;
			this.pipeline = Array.isArray(pipelines) ? pipelines[0] ?? null : null;
			this.users = Array.isArray(users) ? users : [];
			this.coreReady = true;
		})().finally(() => {
			this.coreLoading = false;
			this.corePromise = null;
		});

		return this.corePromise;
	}

	async loadSettings(currentUser?: any) {
		if (this.settingsReady) return;
		if (this.settingsPromise) return this.settingsPromise;

		this.settingsLoading = true;
		this.settingsPromise = (async () => {
			await Promise.all([
				this.loadCore(currentUser),
				this.refreshChannels()
			]);
			this.settingsReady = true;
		})().finally(() => {
			this.settingsLoading = false;
			this.settingsPromise = null;
		});

		return this.settingsPromise;
	}

	async refreshAccount() {
		this.account = await apiRequest('/workspace/account');
	}

	async refreshUsers() {
		const users = await apiRequest('/workspace/users');
		this.users = Array.isArray(users) ? users : [];
	}

	async refreshChannels() {
		const channels = await apiRequest('/channels');
		this.channels = Array.isArray(channels) ? channels : [];
	}

	async refreshPipeline() {
		const pipelines = await apiRequest('/workspace/pipelines');
		this.pipeline = Array.isArray(pipelines) ? pipelines[0] ?? null : null;
	}
}
