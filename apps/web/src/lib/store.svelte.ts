import { apiRequest } from '$lib/api';
import type { UICapabilities } from '$lib/ui-capabilities';

export interface AIReplyDraft {
	id: string;
	conversation_id: string;
	source_message_id: string;
	draft_text: string;
	stage_matched: 'pattern' | 'embedding' | 'llm_grounded';
	confidence?: number;
	status: 'pending';
	created_at: string;
	updated_at: string;
}

export class InboxState {
	conversations = $state<any[]>([]);
	activeConvoID = $state<string | null>(null);
	pendingConvoID = $state<string | null>(null);
	activeConvo = $state<any | null>(null);
	messages = $state<any[]>([]);
	replyDrafts = $state<Record<string, AIReplyDraft>>({});
	nextCursor = $state<string | null>(null);
	filter = $state<'all' | 'mine' | 'unassigned'>('mine');
	stateFilter = $state<string>('');
	currentUser = $state<any | null>(null);
	users = $state<any[]>([]);
	
	wsStatus = $state<'connecting' | 'connected' | 'disconnected'>('disconnected');
	ws: WebSocket | null = null;
	reconnectAttempts = 0;
	private conversationRequest: AbortController | null = null;
	private conversationRequestVersion = 0;
	private replyDraftsEnabled = false;

	configureCapabilities(capabilities: UICapabilities) {
		this.replyDraftsEnabled = capabilities.useReplyDrafts;
		if (!this.replyDraftsEnabled) this.replyDrafts = {};
	}
	
	async init() {
		try {
			this.currentUser = await apiRequest('/auth/me');
			if (this.currentUser.role === 'manager') {
				this.filter = 'all';
			} else {
				this.filter = 'mine';
			}
			await this.loadConversations();
			this.connectWS();
		} catch (err) {
			console.error('Failed to init inbox', err);
		}
	}

	async loadConversations() {
		try {
			let url = `/conversations?filter=${this.filter}`;
			if (this.stateFilter) {
				url += `&state=${this.stateFilter}`;
			}
			const conversations = await apiRequest(url);
			this.conversations = Array.isArray(conversations) ? conversations : [];
		} catch (err) {
			console.error(err);
		}
	}
	
	async selectConversation(convoID: string) {
		if (convoID === this.activeConvoID && !this.pendingConvoID) return;

		// Keep the existing conversation rendered until the replacement is complete.
		// This avoids a blank pane on slow connections and prevents late responses
		// from overwriting a newer selection.
		this.conversationRequest?.abort();
		const controller = new AbortController();
		this.conversationRequest = controller;
		const requestVersion = ++this.conversationRequestVersion;
		this.pendingConvoID = convoID;

		try {
			const [conversation, messageResponse, draftResponse] = await Promise.all([
				apiRequest(`/conversations/${convoID}`, { signal: controller.signal }),
				apiRequest(`/conversations/${convoID}/messages?limit=20`, { signal: controller.signal }),
				this.replyDraftsEnabled ? apiRequest(`/conversations/${convoID}/reply-draft`, { signal: controller.signal })
					.catch((err) => {
						if (!(err instanceof DOMException && err.name === 'AbortError')) {
							console.error('Failed to load AI reply draft', err);
						}
						return { draft: null };
					}) : Promise.resolve({ draft: null })
			]);
			if (requestVersion !== this.conversationRequestVersion) return;

			this.activeConvoID = convoID;
			this.activeConvo = conversation;
			this.messages = (messageResponse?.messages ?? []).reverse();
			this.nextCursor = messageResponse?.next_cursor ?? null;
			this.setReplyDraft(convoID, draftResponse?.draft ?? null);

			void apiRequest(`/conversations/${convoID}/read`, { method: 'POST' }).catch((err) => console.error(err));
			const index = this.conversations.findIndex(c => c.id === convoID);
			if (index !== -1) {
				this.conversations[index].unread = false;
			}
		} catch (err) {
			if (!(err instanceof DOMException && err.name === 'AbortError')) {
				console.error(err);
			}
		} finally {
			if (requestVersion === this.conversationRequestVersion) {
				this.pendingConvoID = null;
				this.conversationRequest = null;
			}
		}
	}

	private setReplyDraft(conversationID: string, draft: AIReplyDraft | null) {
		if (draft) {
			this.replyDrafts = { ...this.replyDrafts, [conversationID]: draft };
			return;
		}
		const { [conversationID]: _removed, ...remaining } = this.replyDrafts;
		this.replyDrafts = remaining;
	}

	async loadReplyDraft(conversationID = this.activeConvoID) {
		if (!conversationID) return;
		try {
			const response = await apiRequest(`/conversations/${conversationID}/reply-draft`);
			this.setReplyDraft(conversationID, response?.draft ?? null);
		} catch (err) {
			console.error('Failed to load AI reply draft', err);
		}
	}

	async dismissReplyDraft(conversationID: string, draftID: string) {
		await apiRequest(`/conversations/${conversationID}/reply-draft/${draftID}/dismiss`, { method: 'POST' });
		if (this.replyDrafts[conversationID]?.id === draftID) {
			this.setReplyDraft(conversationID, null);
		}
	}
	
	async loadMessages(reset = false) {
		if (!this.activeConvoID) return;
		const conversationID = this.activeConvoID;
		try {
			const url = `/conversations/${conversationID}/messages?limit=20` +
				(this.nextCursor && !reset ? `&before=${this.nextCursor}` : '');
			const res = await apiRequest(url);
			if (conversationID !== this.activeConvoID) return;
			if (reset) {
				this.messages = res.messages.reverse();
			} else {
				const olderMessages = res.messages.reverse();
				this.messages = [...olderMessages, ...this.messages];
			}
			this.nextCursor = res.next_cursor;
		} catch (err) {
			console.error(err);
		}
	}
	
	async sendMessage(text: string, aiReplyDraftID?: string): Promise<boolean> {
		const convoID = this.activeConvoID || this.pendingConvoID;
		if (!convoID || !text.trim()) return false;
		const pendingDraftID = this.replyDrafts[convoID]?.id;
		try {
			const senderUserId = this.currentUser?.user_id || this.currentUser?.id;
			const body: any = {
				content_type: 'text',
				text: text,
				sender_type: 'human'
			};
			if (senderUserId) {
				body.sender_user_id = senderUserId;
			}
			if (aiReplyDraftID) {
				body.ai_reply_draft_id = aiReplyDraftID;
			}
			const res = await apiRequest(`/internal/conversations/${convoID}/send`, {
				method: 'POST',
				body
			});
			if (res && res.id) {
				if (!this.messages.some(m => m.id === res.id)) {
					this.messages = [...this.messages, res];
				}
			} else {
				await this.loadMessages(true);
			}
			await this.loadConversations();
			if (pendingDraftID && this.replyDrafts[convoID]?.id === pendingDraftID) {
				this.setReplyDraft(convoID, null);
			}
			window.dispatchEvent(new CustomEvent('dev-message-sent'));
			return true;
		} catch (err) {
			console.error('Failed to send message:', err);
			return false;
		}
	}
	
	async assignConversation(userIDs: string[]) {
		if (!this.activeConvoID) return;
		try {
			await apiRequest(`/conversations/${this.activeConvoID}/assign`, {
				method: 'PATCH',
				body: { user_ids: userIDs }
			});
			if (this.activeConvo) {
				this.activeConvo.assigned_user_ids = userIDs;
			}
			await this.loadConversations();
		} catch (err) {
			console.error(err);
		}
	}

	async updateConversationAIControl(action = '', replyOverride = '') {
		if (!this.activeConvoID) return false;
		try {
			const response = await apiRequest(`/conversations/${this.activeConvoID}/ai-control`, {
				method: 'PATCH',
				body: {
					...(action ? { action } : {}),
					...(replyOverride ? { reply_override: replyOverride } : {})
				}
			});
			const control = response?.ai_control;
			if (this.activeConvo && control) this.activeConvo.ai_control = control;
			const item = this.conversations.find((conversation) => conversation.id === this.activeConvoID);
			if (item && control) item.ai_control = control;
			return true;
		} catch (err) {
			console.error('Failed to update conversation AI auto-reply:', err);
			return false;
		}
	}

	async closeConversation(convoID?: string) {
		const id = convoID || this.activeConvoID;
		if (!id) return;
		try {
			await apiRequest(`/conversations/${id}/close`, { method: 'POST' });
			if (this.activeConvo && this.activeConvo.id === id) {
				this.activeConvo.status = 'closed';
				this.activeConvo.ai_control = { ...this.activeConvo.ai_control, state: 'active', state_reason: 'conversation_closed', run_state: 'idle' };
			}
			const index = this.conversations.findIndex(c => c.id === id);
			if (index !== -1) {
				this.conversations[index].status = 'closed';
				this.conversations[index].ai_control = { ...this.conversations[index].ai_control, state: 'active', state_reason: 'conversation_closed', run_state: 'idle' };
			}
			await this.loadConversations();
		} catch (err) {
			console.error('Failed to close conversation:', err);
		}
	}
	
	connectWS() {
		if (this.ws) {
			this.ws.close();
		}
		
		this.wsStatus = 'connecting';
		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const wsUrl = `${protocol}//${window.location.host}/ws`;
		
		this.ws = new WebSocket(wsUrl);
		
		this.ws.onopen = () => {
			this.wsStatus = 'connected';
			this.reconnectAttempts = 0;
			console.log('WS connected');
			if (this.replyDraftsEnabled) void this.loadReplyDraft();
		};
		
		this.ws.onmessage = async (e) => {
			try {
				const event = JSON.parse(e.data);
				console.log('WS event received:', event);
				
				switch (event.type) {
					case 'message.received':
					case 'message.sent':
						if (event.conversation_id === this.activeConvoID) {
							if (!this.messages.some(m => m.id === event.message.id)) {
								this.messages = [...this.messages, event.message];
								await apiRequest(`/conversations/${this.activeConvoID}/read`, { method: 'POST' });
							}
						}
						await this.loadConversations();
						break;
						
					case 'conversation.assigned':
						if (event.conversation_id === this.activeConvoID) {
							if (this.activeConvo) {
								this.activeConvo.assigned_user_ids = event.assigned_user_ids;
							}
						}
						await this.loadConversations();
						break;
						
					case 'lead.state_changed':
						if (event.conversation_id === this.activeConvoID) {
							if (this.activeConvo) {
								if (!this.activeConvo.lead) {
									this.activeConvo.lead = { current_state_key: event.to_state };
								} else {
									this.activeConvo.lead.current_state_key = event.to_state;
								}
							}
						}
						await this.loadConversations();
						window.dispatchEvent(new CustomEvent('lead-state-changed', { detail: event }));
						break;
						
					case 'channel.status_changed':
						const channelEvent = new CustomEvent('channel-status-changed', { detail: event });
						window.dispatchEvent(channelEvent);
						break;

					case 'ai.reply_ready':
						if (!this.replyDraftsEnabled) break;
						if (event.action === 'drafted' && event.draft_id && event.draft_text) {
							this.setReplyDraft(event.conversation_id, {
								id: event.draft_id,
								conversation_id: event.conversation_id,
								source_message_id: event.message_id,
								draft_text: event.draft_text,
								stage_matched: event.stage_matched,
								confidence: event.confidence,
								status: 'pending',
								created_at: new Date().toISOString(),
								updated_at: new Date().toISOString()
							});
						} else if (event.action === 'auto_sent') {
							this.setReplyDraft(event.conversation_id, null);
						}
						break;

					case 'ai.reply_draft.updated':
						if (event.draft_id && this.replyDrafts[event.conversation_id]?.id === event.draft_id) {
							this.setReplyDraft(event.conversation_id, null);
						}
						break;

					case 'ai.control.updated': {
						const applyControl = (conversation: any) => {
							if (!conversation) return;
							conversation.ai_control = {
								...(conversation.ai_control || { reply_override: 'inherit' }),
								state: event.state,
								state_reason: event.state_reason,
								run_state: event.run_state
							};
						};
						if (event.conversation_id === this.activeConvoID) applyControl(this.activeConvo);
						applyControl(this.conversations.find((conversation) => conversation.id === event.conversation_id));
						break;
					}
				}
			} catch (err) {
				console.error('Failed to handle WS message', err);
			}
		};
		
		this.ws.onclose = () => {
			this.wsStatus = 'disconnected';
			this.reconnect();
		};
		
		this.ws.onerror = (err) => {
			console.error('WS error', err);
			this.ws?.close();
		};
	}
	
	reconnect() {
		this.reconnectAttempts++;
		const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000);
		console.log(`WS reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`);
		setTimeout(() => {
			if (this.wsStatus === 'disconnected') {
				this.connectWS();
			}
		}, delay);
	}
}
