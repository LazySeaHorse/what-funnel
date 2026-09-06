import { apiRequest } from '$lib/api';
import { DEFAULT_CONTACTS, PRESET_CATEGORIES, SIMULATOR_STORAGE_KEY } from './fixtures';
import { buildNativePayload } from './native-payload';
import type { CascadeTelemetry, ConversationSnapshot, SimulatorPlatform, TestContact } from './types';

const EMPTY_TELEMETRY: CascadeTelemetry = {
	stageMatched: 'none',
	confidence: null,
	action: 'none',
	lastPayload: null,
	latencyMs: 0
};

export class SimulatorController {
	isSending = $state(false);
	lastStatus = $state<'idle' | 'success' | 'error'>('idle');
	lastError = $state('');
	channels = $state<any[]>([]);
	selectedChannelID = $state('');
	convoMessages = $state<any[]>([]);
	activeConvoID = $state<string | null>(null);
	isConvoLoading = $state(false);
	testContacts = $state<TestContact[]>(DEFAULT_CONTACTS.map((contact) => ({ ...contact })));
	selectedContactID = $state('c1');
	selectedPlatform = $state<SimulatorPlatform>('whatsapp');
	currentTelemetry = $state<CascadeTelemetry>({ ...EMPTY_TELEMETRY });

	private pollTimer: ReturnType<typeof setTimeout> | null = null;
	private refreshTimers = new Set<ReturnType<typeof setTimeout>>();
	private polling = false;
	private conversationLoadVersion = 0;
	private sentHandler: (() => void) | null = null;

	get selectedContact() {
		return this.testContacts.find((contact) => contact.id === this.selectedContactID) ?? null;
	}

	get presetCategories() {
		return PRESET_CATEGORIES[this.selectedPlatform];
	}

	start() {
		this.polling = true;
		try {
			const stored = localStorage.getItem(SIMULATOR_STORAGE_KEY);
			if (stored) this.testContacts = JSON.parse(stored);
		} catch {
			// Invalid or unavailable local storage should not disable the simulator.
		}

		void (async () => {
			await this.loadChannels();
			await this.refreshCurrentConversation();
			this.schedulePoll();
		})();

		this.sentHandler = () => void this.refreshCurrentConversation(false);
		window.addEventListener('dev-message-sent', this.sentHandler);
	}

	dispose() {
		this.polling = false;
		this.conversationLoadVersion++;
		if (this.pollTimer) clearTimeout(this.pollTimer);
		this.pollTimer = null;
		for (const timer of this.refreshTimers) clearTimeout(timer);
		this.refreshTimers.clear();
		if (this.sentHandler) window.removeEventListener('dev-message-sent', this.sentHandler);
		this.sentHandler = null;
	}

	async selectPlatform(platform: SimulatorPlatform) {
		this.selectedPlatform = platform;
		const contact = this.selectedContact;
		if (contact) {
			contact.platform = platform;
			this.persistContacts();
		}
		await this.ensureChannelForPlatform(platform);
		await this.refreshCurrentConversation(false);
	}

	selectContact(id: string) {
		this.selectedContactID = id;
		this.convoMessages = [];
		this.activeConvoID = null;
		const contact = this.selectedContact;
		if (contact) {
			this.selectedPlatform = contact.platform;
			void this.ensureChannelForPlatform(contact.platform);
		}
		void this.refreshCurrentConversation(true);
	}

	addContact(name: string) {
		const trimmedName = name.trim();
		if (!trimmedName) return;
		const contact: TestContact = {
			id: `c-${Date.now()}`,
			name: trimmedName,
			externalID: `test-${trimmedName.toLowerCase().replace(/\s+/g, '-')}-${Date.now()}`,
			avatar: trimmedName.charAt(0).toUpperCase() || 'C',
			platform: this.selectedPlatform
		};
		this.testContacts = [...this.testContacts, contact];
		this.selectContact(contact.id);
		this.persistContacts();
	}

	async sendMessage(text: string) {
		const textToSend = text.trim();
		const contact = this.selectedContact;
		if (!textToSend || !contact) return false;

		const targetPlatform = this.selectedPlatform || contact.platform || 'whatsapp';
		contact.platform = targetPlatform;
		const channelID = await this.ensureChannelForPlatform(targetPlatform);
		if (!channelID) return false;

		this.isSending = true;
		this.lastStatus = 'idle';
		this.lastError = '';
		const startTime = performance.now();
		try {
			const nativePayload = buildNativePayload(targetPlatform, contact, textToSend);
			await apiRequest(`/webhooks/${targetPlatform}?channel_id=${channelID}`, { method: 'POST', body: nativePayload });
			this.lastStatus = 'success';
			const stageMatched = inferStage(textToSend);
			this.currentTelemetry = {
				stageMatched,
				confidence: stageMatched === 'pattern' ? 0.98 : stageMatched === 'llm_grounded' ? 0.92 : 0.45,
				action: stageMatched === 'none' ? 'flagged_human' : 'auto_sent',
				lastInboundText: textToSend,
				lastPayload: nativePayload,
				channelID,
				channelType: `matrix_${targetPlatform}`,
				latencyMs: Math.round(performance.now() - startTime),
				timestamp: new Date().toISOString()
			};

			window.dispatchEvent(new CustomEvent('dev-message-sent'));
			await this.refreshCurrentConversation(false);
			this.scheduleRefresh(1000, true);
			this.scheduleRefresh(2500, false);
			return true;
		} catch (error) {
			this.lastStatus = 'error';
			this.lastError = error instanceof Error ? error.message : 'Failed to send message';
			return false;
		} finally {
			this.isSending = false;
		}
	}

	async refreshCurrentConversation(showLoading = true) {
		const requestVersion = ++this.conversationLoadVersion;
		const contact = this.selectedContact;
		if (!contact) return;
		const contactSnapshot = { ...contact };
		const platform = this.selectedPlatform;
		if (showLoading && !this.convoMessages.length) this.isConvoLoading = true;

		try {
			const snapshot = await this.loadConversationSnapshot(contactSnapshot, platform);
			if (requestVersion !== this.conversationLoadVersion || this.selectedContactID !== snapshot.contactID || this.selectedPlatform !== snapshot.platform) return;
			this.activeConvoID = snapshot.conversationID;
			this.convoMessages = snapshot.messages;
			if (snapshot.telemetry) this.currentTelemetry = { ...this.currentTelemetry, ...snapshot.telemetry };
		} catch {
			// Polling failures are transient; retain the last complete snapshot.
		} finally {
			if (showLoading && requestVersion === this.conversationLoadVersion) this.isConvoLoading = false;
		}
	}

	private schedulePoll() {
		if (!this.polling || this.pollTimer) return;
		this.pollTimer = setTimeout(async () => {
			this.pollTimer = null;
			await this.refreshCurrentConversation(false);
			this.schedulePoll();
		}, 2500);
	}

	private scheduleRefresh(delay: number, resetStatus: boolean) {
		const timer = setTimeout(() => {
			this.refreshTimers.delete(timer);
			void this.refreshCurrentConversation(false);
			window.dispatchEvent(new CustomEvent('dev-message-sent'));
			if (resetStatus) this.lastStatus = 'idle';
		}, delay);
		this.refreshTimers.add(timer);
	}

	private async loadChannels() {
		await this.ensureChannelForPlatform(this.selectedPlatform);
	}

	private async ensureChannelForPlatform(platform: SimulatorPlatform): Promise<string | null> {
		try {
			const data = await apiRequest('/simulate/channels');
			this.channels = Array.isArray(data) ? data : [];
			const matching = this.channels.find((channel) => channel.type === `matrix_${platform}` || channel.type === platform);
			if (matching) {
				this.selectedChannelID = matching.id;
				return matching.id;
			}

			const created = await apiRequest('/channels', { method: 'POST', body: { type: `matrix_${platform}` } });
			const updated = await apiRequest('/simulate/channels');
			this.channels = Array.isArray(updated) ? updated : [];
			const channelID = created?.id ?? this.channels.find((channel) => channel.type === `matrix_${platform}` || channel.type === platform)?.id;
			if (channelID) {
				this.selectedChannelID = channelID;
				return channelID;
			}
		} catch (error) {
			this.lastStatus = 'error';
			this.lastError = error instanceof Error ? error.message : 'Failed to ensure channel';
		}
		return this.selectedChannelID || null;
	}

	private async loadConversationSnapshot(contact: TestContact, platform: SimulatorPlatform): Promise<ConversationSnapshot> {
		const conversations = await apiRequest('/conversations?filter=all');
		const match = findConversation(Array.isArray(conversations) ? conversations : [], contact);
		if (!match) return { contactID: contact.id, platform, conversationID: null, messages: [], telemetry: null };

		const [messageResponse, draftResponse] = await Promise.all([
			apiRequest(`/conversations/${match.id}/messages?limit=30`),
			apiRequest(`/conversations/${match.id}/reply-draft`).catch(() => null)
		]);
		const draft = draftResponse?.draft;
		return {
			contactID: contact.id,
			platform,
			conversationID: match.id,
			messages: Array.isArray(messageResponse?.messages) ? [...messageResponse.messages].reverse() : [],
			telemetry: draft ? {
				stageMatched: draft.stage_matched || 'none', confidence: draft.confidence, action: 'drafted',
				draftText: draft.draft_text, draftStatus: draft.status, channelID: match.channel_id,
				channelType: match.channel_type || `matrix_${platform}`, timestamp: new Date().toISOString()
			} : null
		};
	}

	private persistContacts() {
		try {
			localStorage.setItem(SIMULATOR_STORAGE_KEY, JSON.stringify(this.testContacts));
		} catch {
			// Persistence is optional in restricted browser contexts.
		}
	}
}

function findConversation(conversations: any[], contact: TestContact) {
	return conversations.find((conversation) =>
		(conversation.contact_name && (conversation.contact_name.startsWith(contact.name) || conversation.contact_name.includes(contact.name))) ||
		(conversation.contact?.display_name && (conversation.contact.display_name.startsWith(contact.name) || conversation.contact.display_name.includes(contact.name))) ||
		conversation.contact?.external_identity === contact.externalID || conversation.external_identity === contact.externalID ||
		(conversation.display_name && (conversation.display_name.startsWith(contact.name) || conversation.display_name.includes(contact.name))) ||
		(conversation.contact_display_name && (conversation.contact_display_name.startsWith(contact.name) || conversation.contact_display_name.includes(contact.name)))
	);
}

function inferStage(text: string): CascadeTelemetry['stageMatched'] {
	const lower = text.toLowerCase();
	if (['weekend', 'hour', 'located', 'cancel', 'open'].some((word) => lower.includes(word))) return 'pattern';
	if (['pricing', 'package', 'concierge', 'quote', 'hair'].some((word) => lower.includes(word))) return 'llm_grounded';
	return 'none';
}
