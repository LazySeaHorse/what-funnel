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
	composers = $state<Record<string, { text: string; aiReplyDraftID: string | null; sending: boolean; error: string }>>({});
	aiControlPending = $state<Record<string, boolean>>({});
	assignmentPending = $state<Record<string, boolean>>({});
	mutationErrors = $state<Record<string, string>>({});
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
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	private running = false;
	private conversationRefreshPending = false;
	private conversationRefreshPromise: Promise<void> | null = null;
	private fallbackPollTimer: ReturnType<typeof setInterval> | null = null;
	private consecutiveFetchFailures = 0;
	private onlineHandler: (() => void) | null = null;
	private offlineHandler: (() => void) | null = null;
	private visibilityHandler: (() => void) | null = null;

	private isOnline(): boolean {
		return typeof navigator === 'undefined' || typeof navigator.onLine !== 'boolean' || navigator.onLine;
	}

	configureCapabilities(capabilities: UICapabilities) {
		this.replyDraftsEnabled = capabilities.useReplyDrafts;
		if (!this.replyDraftsEnabled) this.replyDrafts = {};
	}
	
	async init() {
		this.running = true;

		if (typeof window !== 'undefined') {
			this.onlineHandler = () => {
				if (!this.running) return;
				console.log('Network online');
				this.reconnectAttempts = 0;
				if (this.reconnectTimer) {
					clearTimeout(this.reconnectTimer);
					this.reconnectTimer = null;
				}
				this.connectWS();
				void this.loadConversations();
			};
			this.offlineHandler = () => {
				if (!this.running) return;
				console.log('Network offline');
				const socket = this.ws;
				this.ws = null;
				if (socket) {
					socket.onopen = null;
					socket.onmessage = null;
					socket.onerror = null;
					socket.onclose = null;
					socket.close();
				}
				this.wsStatus = 'disconnected';
				if (this.reconnectTimer) {
					clearTimeout(this.reconnectTimer);
					this.reconnectTimer = null;
				}
			};
			this.visibilityHandler = () => {
				if (!this.running) return;
				if (document.visibilityState === 'visible' && this.isOnline()) {
					if (this.wsStatus === 'connected') {
						void this.loadConversations();
					} else {
						this.connectWS();
					}
				}
			};
			window.addEventListener('online', this.onlineHandler);
			window.addEventListener('offline', this.offlineHandler);
			document.addEventListener('visibilitychange', this.visibilityHandler);

			this.fallbackPollTimer = setInterval(() => {
				if (!this.running) return;
				if (this.wsStatus === 'connected' || !this.isOnline()) return;
				void this.loadConversations();
			}, 30000);
		}

		try {
			const currentUser = await apiRequest('/auth/me');
			if (!this.running) return;
			this.currentUser = currentUser;
			if (this.currentUser.role === 'manager') {
				this.filter = 'all';
			} else {
				this.filter = 'mine';
			}
			await this.loadConversations();
			if (!this.running) return;
			this.connectWS();
		} catch (err) {
			if (this.running) console.error('Failed to init inbox', err);
		}
	}

	dispose() {
		this.running = false;
		this.conversationRefreshPending = false;
		this.cancelConversationLoad();

		if (this.fallbackPollTimer) {
			clearInterval(this.fallbackPollTimer);
			this.fallbackPollTimer = null;
		}

		if (this.reconnectTimer) {
			clearTimeout(this.reconnectTimer);
			this.reconnectTimer = null;
		}

		if (typeof window !== 'undefined') {
			if (this.onlineHandler) {
				window.removeEventListener('online', this.onlineHandler);
				this.onlineHandler = null;
			}
			if (this.offlineHandler) {
				window.removeEventListener('offline', this.offlineHandler);
				this.offlineHandler = null;
			}
			if (this.visibilityHandler) {
				document.removeEventListener('visibilitychange', this.visibilityHandler);
				this.visibilityHandler = null;
			}
		}

		const socket = this.ws;
		this.ws = null;
		if (socket) {
			socket.onopen = null;
			socket.onmessage = null;
			socket.onerror = null;
			socket.onclose = null;
			socket.close(1000, 'Inbox disposed');
		}

		this.reconnectAttempts = 0;
		this.wsStatus = 'disconnected';
	}

	clearConversationSelection() {
		this.cancelConversationLoad();
		this.activeConvoID = null;
		this.activeConvo = null;
		this.messages = [];
		this.nextCursor = null;
	}

	private cancelConversationLoad() {
		this.conversationRequestVersion++;
		this.conversationRequest?.abort();
		this.conversationRequest = null;
		this.pendingConvoID = null;
	}

	loadConversations(): Promise<void> {
		if (!this.running) return Promise.resolve();

		this.conversationRefreshPending = true;
		if (!this.conversationRefreshPromise) {
			this.conversationRefreshPromise = this.drainConversationRefreshes().finally(() => {
				this.conversationRefreshPromise = null;
			});
		}
		return this.conversationRefreshPromise;
	}

	private async drainConversationRefreshes() {
		while (this.running && this.conversationRefreshPending) {
			this.conversationRefreshPending = false;
			const filter = this.filter;
			const stateFilter = this.stateFilter;
			const params = new URLSearchParams({ filter });
			if (stateFilter) params.set('state', stateFilter);

			try {
				const conversations = await apiRequest(`/conversations?${params}`);
				if (!this.running) return;
				if (filter !== this.filter || stateFilter !== this.stateFilter) continue;
				this.conversations = Array.isArray(conversations) ? conversations : [];
				this.consecutiveFetchFailures = 0;
			} catch (err) {
				this.consecutiveFetchFailures++;
				if (this.running && this.consecutiveFetchFailures <= 1 && this.isOnline()) {
					console.error(err);
				}
			}
		}
	}
	
	async selectConversation(convoID: string) {
		if (convoID === this.activeConvoID && !this.pendingConvoID) return;

		// Keep the existing conversation rendered until the replacement is complete.
		// This avoids a blank pane on slow connections and prevents late responses
		// from overwriting a newer selection.
		this.cancelConversationLoad();
		const controller = new AbortController();
		this.conversationRequest = controller;
		const requestVersion = this.conversationRequestVersion;
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
			this.composers[convoID] ??= { text: '', aiReplyDraftID: null, sending: false, error: '' };
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
	
	async sendMessage(convoID: string, text: string, aiReplyDraftID?: string): Promise<boolean> {
		if (!convoID || !text.trim()) return false;
		const composer = this.composers[convoID] ??= { text: '', aiReplyDraftID: null, sending: false, error: '' };
		if (composer.sending) return false;
		composer.sending = true;
		composer.error = '';
		const submittedText = composer.text;
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
			if (this.activeConvoID === convoID && res && res.id) {
				if (!this.messages.some(m => m.id === res.id)) {
					this.messages = [...this.messages, res];
				}
			} else if (this.activeConvoID === convoID && !res?.id) {
				await this.loadMessages(true);
			}
			await this.loadConversations();
			if (pendingDraftID && this.replyDrafts[convoID]?.id === pendingDraftID) {
				this.setReplyDraft(convoID, null);
			}
			if (composer.text === submittedText) {
				composer.text = '';
				composer.aiReplyDraftID = null;
			}
			return true;
		} catch (err) {
			console.error('Failed to send message:', err);
			composer.error = 'Failed to send message. Please try again.';
			return false;
		} finally {
			composer.sending = false;
		}
	}
	
	async assignConversation(convoID: string, userIDs: string[]) {
		if (!convoID || this.assignmentPending[convoID]) return;
		this.assignmentPending[convoID] = true;
		this.mutationErrors[convoID] = '';
		try {
			await apiRequest(`/conversations/${convoID}/assign`, {
				method: 'PATCH',
				body: { user_ids: userIDs }
			});
			if (this.activeConvo?.id === convoID) {
				this.activeConvo.assigned_user_ids = userIDs;
			}
			const item = this.conversations.find((conversation) => conversation.id === convoID);
			if (item) item.assigned_user_ids = userIDs;
			await this.loadConversations();
		} catch (err) {
			console.error(err);
			this.mutationErrors[convoID] = 'Failed to update assignment. Please try again.';
		} finally {
			this.assignmentPending[convoID] = false;
		}
	}

	async updateConversationAIControl(convoID: string, action = '', replyOverride = '') {
		if (!convoID || this.aiControlPending[convoID]) return false;
		this.aiControlPending[convoID] = true;
		this.mutationErrors[convoID] = '';
		try {
			const response = await apiRequest(`/conversations/${convoID}/ai-control`, {
				method: 'PATCH',
				body: {
					...(action ? { action } : {}),
					...(replyOverride ? { reply_override: replyOverride } : {})
				}
			});
			const control = response?.ai_control;
			if (this.activeConvo?.id === convoID && control) this.activeConvo.ai_control = control;
			const item = this.conversations.find((conversation) => conversation.id === convoID);
			if (item && control) item.ai_control = control;
			return true;
		} catch (err) {
			console.error('Failed to update conversation AI auto-reply:', err);
			this.mutationErrors[convoID] = 'Failed to update AI controls. Please try again.';
			return false;
		} finally {
			this.aiControlPending[convoID] = false;
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
		if (!this.running || this.ws) return;
		
		this.wsStatus = 'connecting';
		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const wsUrl = `${protocol}//${window.location.host}/ws`;
		const socket = new WebSocket(wsUrl);
		this.ws = socket;
		
		socket.onopen = () => {
			if (!this.running || this.ws !== socket) return;
			this.wsStatus = 'connected';
			this.reconnectAttempts = 0;
			this.consecutiveFetchFailures = 0;
			console.log('WS connected');
			void this.loadConversations();
			if (this.activeConvoID) {
				void this.loadMessages(true);
			}
			if (this.replyDraftsEnabled) void this.loadReplyDraft();
		};
		
		socket.onmessage = async (e) => {
			if (!this.running || this.ws !== socket) return;
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
		
		socket.onclose = () => {
			if (this.ws !== socket) return;
			this.ws = null;
			this.wsStatus = 'disconnected';
			if (this.running) this.reconnect();
		};
		
		socket.onerror = (err) => {
			if (this.ws !== socket) return;
			console.error('WS error', err);
			socket.close();
		};
	}
	
	reconnect() {
		if (!this.running || this.reconnectTimer) return;
		if (!this.isOnline()) return;
		this.reconnectAttempts++;
		const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000);
		console.log(`WS reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`);
		this.reconnectTimer = setTimeout(() => {
			this.reconnectTimer = null;
			if (this.running && this.wsStatus === 'disconnected' && this.isOnline()) {
				this.connectWS();
			}
		}, delay);
	}
}
