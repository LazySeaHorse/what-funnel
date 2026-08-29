import { apiRequest } from '$lib/api';
import { getUICapabilities, type UICapabilities } from '$lib/ui-capabilities';

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
	capabilities = $state<UICapabilities>(getUICapabilities());

	private corePromise: Promise<void> | null = null;
	private settingsPromise: Promise<void> | null = null;
	private currentUser: any | null = null;

	async loadCore(currentUser?: any) {
		if (this.coreReady) return;
		if (this.corePromise) return this.corePromise;

		this.coreLoading = true;
		this.corePromise = (async () => {
			this.currentUser = currentUser ?? this.currentUser;
			const account = await apiRequest('/workspace/account');
			this.account = account;
			this.capabilities = getUICapabilities(account, this.currentUser);

			const [pipelines, users] = await Promise.all([
				this.capabilities.leadTracking ? apiRequest('/workspace/pipelines') : Promise.resolve([]),
				this.capabilities.manageTeam && (!Array.isArray(this.users) || this.users.length === 0)
					? apiRequest('/workspace/users')
					: Promise.resolve(this.users)
			]);
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
			await this.loadCore(currentUser);
			if (this.capabilities.manageChannels) await this.refreshChannels();
			this.settingsReady = true;
		})().finally(() => {
			this.settingsLoading = false;
			this.settingsPromise = null;
		});

		return this.settingsPromise;
	}

	async refreshAccount() {
		this.account = await apiRequest('/workspace/account');
		this.capabilities = getUICapabilities(this.account, this.currentUser);
		if (!this.capabilities.leadTracking) {
			this.pipeline = null;
			this.users = [];
			return;
		}
		await Promise.all([
			this.pipeline ? Promise.resolve() : this.refreshPipeline(),
			this.capabilities.manageTeam && this.users.length === 0 ? this.refreshUsers() : Promise.resolve()
		]);
	}

	async refreshUsers() {
		if (!this.capabilities.manageTeam) return;
		const users = await apiRequest('/workspace/users');
		this.users = Array.isArray(users) ? users : [];
	}

	async refreshChannels() {
		if (!this.capabilities.manageChannels) return;
		const channels = await apiRequest('/channels');
		this.channels = Array.isArray(channels) ? channels : [];
	}

	async refreshPipeline() {
		if (!this.capabilities.leadTracking) return;
		const pipelines = await apiRequest('/workspace/pipelines');
		this.pipeline = Array.isArray(pipelines) ? pipelines[0] ?? null : null;
	}
}
