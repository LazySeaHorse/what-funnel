import { apiRequest, type ApiRequestOptions } from '$lib/api';
import type { InboxState } from '$lib/store.svelte';

type Request = (path: string, options?: ApiRequestOptions) => Promise<any>;

export class LeadEditor {
	conversationID = $state<string | null>(null);
	leadID = $state<string | null>(null);
	notes = $state<any[]>([]);
	history = $state<any[]>([]);
	loading = $state(false);
	error = $state('');

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
		this.error = '';
		await this.loadDetails(leadID);
	}

	clear() {
		this.cancelDetailsRequest();
		this.conversationID = null;
		this.leadID = null;
		this.notes = [];
		this.history = [];
		this.loading = false;
		this.error = '';
	}

	dispose() {
		this.clear();
	}

	async changeStage(stateKey: string) {
		const leadID = this.requireLeadID();
		if (!leadID) return;
		this.error = '';
		try {
			const updated = await this.request(`/leads/${leadID}/state`, {
				method: 'PATCH',
				body: { state_key: stateKey }
			});
			if (this.leadID === leadID && this.lead) {
				this.lead.current_state_key = updated?.current_state_key ?? stateKey;
			}
			await this.inbox.loadConversations();
		} catch (error) {
			this.fail('Failed to change the lead stage. Please try again.', error, leadID);
		}
	}

	async addTag(tag: string) {
		const leadID = this.requireLeadID();
		if (!leadID) return;
		const value = tag.trim();
		const tags = this.lead?.tags ?? [];
		if (!value || tags.includes(value)) return;
		await this.updateTags(leadID, [...tags, value]);
	}

	async removeTag(tag: string) {
		const leadID = this.requireLeadID();
		if (!leadID) return;
		await this.updateTags(leadID, (this.lead?.tags ?? []).filter((value: string) => value !== tag));
	}

	async toggleAssignee(userID: string) {
		const conversationID = this.requireConversationID();
		if (!conversationID) return;
		this.error = '';
		const current = this.conversation?.assigned_user_ids ?? [];
		try {
			await this.inbox.assignConversation(
				conversationID,
				current.includes(userID)
					? current.filter((id: string) => id !== userID)
					: [...current, userID]
			);
		} catch (error) {
			this.fail('Failed to update assignees. Please try again.', error, this.leadID);
			return;
		}
		// assignConversation records its own failure instead of throwing.
		const mutationError = this.inbox.mutationErrors[conversationID];
		if (mutationError && this.conversationID === conversationID) this.error = mutationError;
	}

	async addNote(body: string) {
		const leadID = this.requireLeadID();
		if (!leadID) return;
		const value = body.trim();
		if (!value) return;
		this.error = '';
		try {
			await this.request(`/leads/${leadID}/notes`, { method: 'POST', body: { body: value } });
		} catch (error) {
			this.fail('Failed to add the note. Please try again.', error, leadID);
			return;
		}
		if (this.leadID === leadID) await this.loadDetails(leadID);
	}

	private async updateTags(leadID: string, tags: string[]) {
		this.error = '';
		try {
			const updated = await this.request(`/leads/${leadID}/tags`, {
				method: 'PATCH',
				body: { tags }
			});
			if (this.leadID === leadID && this.lead) this.lead.tags = updated.tags;
			await this.inbox.loadConversations();
		} catch (error) {
			this.fail('Failed to update tags. Please try again.', error, leadID);
		}
	}

	private async loadDetails(leadID: string) {
		this.cancelDetailsRequest();
		const version = this.requestVersion;
		const controller = new AbortController();
		this.detailsRequest = controller;
		this.loading = true;
		this.error = '';
		this.notes = [];
		this.history = [];
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
			if (!(error instanceof DOMException && error.name === 'AbortError')) {
				this.fail('Failed to load lead details. Please try again.', error, leadID);
			}
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

	private fail(message: string, cause: unknown, leadID: string | null) {
		console.error(message, cause);
		if (leadID === null || this.leadID === leadID) this.error = message;
	}

	// Surfaces the "nothing selected" case instead of silently ignoring the action.
	private requireLeadID(): string | null {
		if (!this.leadID) {
			this.error = 'No lead is selected.';
			return null;
		}
		return this.leadID;
	}

	private requireConversationID(): string | null {
		if (!this.conversationID) {
			this.error = 'No conversation is selected.';
			return null;
		}
		return this.conversationID;
	}
}
