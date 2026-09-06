import { apiRequest, type ApiRequestOptions } from '$lib/api';
import type { InboxState } from '$lib/store.svelte';

type Request = (path: string, options?: ApiRequestOptions) => Promise<any>;

export class LeadEditor {
	conversationID = $state<string | null>(null);
	leadID = $state<string | null>(null);
	notes = $state<any[]>([]);
	history = $state<any[]>([]);
	loading = $state(false);

	private requestVersion = 0;
	private detailsRequest: AbortController | null = null;

	constructor(
		readonly inbox: InboxState,
		private request: Request = apiRequest
	) {}

	get conversation(): any | null {
		if (!this.conversationID) return null;
		return this.inbox.conversations.find((item) => item.id === this.conversationID)
			?? (this.inbox.activeConvo?.id === this.conversationID ? this.inbox.activeConvo : null);
	}

	get lead(): any | null {
		return this.conversation?.lead ?? null;
	}

	async open(conversation: any) {
		const conversationID = conversation?.id;
		const leadID = conversation?.lead?.id;
		if (!conversationID || !leadID) return this.clear();
		if (conversationID === this.conversationID && leadID === this.leadID && !this.loading) return;

		this.conversationID = conversationID;
		this.leadID = leadID;
		await this.loadDetails(leadID);
	}

	clear() {
		this.cancelDetailsRequest();
		this.conversationID = null;
		this.leadID = null;
		this.notes = [];
		this.history = [];
		this.loading = false;
	}

	dispose() {
		this.clear();
	}

	async changeStage(stateKey: string) {
		const leadID = this.requireLeadID();
		const updated = await this.request(`/leads/${leadID}/state`, {
			method: 'PATCH',
			body: { state_key: stateKey }
		});
		if (this.leadID === leadID && this.lead) {
			this.lead.current_state_key = updated?.current_state_key ?? stateKey;
		}
		await this.inbox.loadConversations();
	}

	async addTag(tag: string) {
		const value = tag.trim();
		const leadID = this.requireLeadID();
		const tags = this.lead?.tags ?? [];
		if (!value || tags.includes(value)) return;
		await this.updateTags(leadID, [...tags, value]);
	}

	async removeTag(tag: string) {
		const leadID = this.requireLeadID();
		await this.updateTags(leadID, (this.lead?.tags ?? []).filter((value: string) => value !== tag));
	}

	async toggleAssignee(userID: string) {
		const conversationID = this.requireConversationID();
		const current = this.conversation?.assigned_user_ids ?? [];
		await this.inbox.assignConversation(
			conversationID,
			current.includes(userID)
				? current.filter((id: string) => id !== userID)
				: [...current, userID]
		);
	}

	async addNote(body: string) {
		const leadID = this.requireLeadID();
		const value = body.trim();
		if (!value) return;
		await this.request(`/leads/${leadID}/notes`, { method: 'POST', body: { body: value } });
		if (this.leadID === leadID) await this.loadDetails(leadID);
	}

	private async updateTags(leadID: string, tags: string[]) {
		const updated = await this.request(`/leads/${leadID}/tags`, {
			method: 'PATCH',
			body: { tags }
		});
		if (this.leadID === leadID && this.lead) this.lead.tags = updated.tags;
		await this.inbox.loadConversations();
	}

	private async loadDetails(leadID: string) {
		this.cancelDetailsRequest();
		const version = this.requestVersion;
		const controller = new AbortController();
		this.detailsRequest = controller;
		this.loading = true;
		try {
			const [notes, history] = await Promise.all([
				this.request(`/leads/${leadID}/notes`, { signal: controller.signal }),
				this.request(`/leads/${leadID}/history`, { signal: controller.signal })
			]);
			if (version === this.requestVersion && this.leadID === leadID) {
				this.notes = Array.isArray(notes) ? notes : [];
				this.history = Array.isArray(history) ? history : [];
			}
		} catch (error) {
			if (!(error instanceof DOMException && error.name === 'AbortError')) throw error;
		} finally {
			if (version === this.requestVersion) {
				this.loading = false;
				this.detailsRequest = null;
			}
		}
	}

	private cancelDetailsRequest() {
		this.requestVersion++;
		this.detailsRequest?.abort();
		this.detailsRequest = null;
	}

	private requireLeadID(): string {
		if (!this.leadID) throw new Error('No lead is selected.');
		return this.leadID;
	}

	private requireConversationID(): string {
		if (!this.conversationID) throw new Error('No conversation is selected.');
		return this.conversationID;
	}
}
