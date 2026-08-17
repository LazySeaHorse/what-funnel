import { apiRequest } from '$lib/api';

export class InboxState {
	conversations = $state<any[]>([]);
	activeConvoID = $state<string | null>(null);
	activeConvo = $state<any | null>(null);
	messages = $state<any[]>([]);
	nextCursor = $state<string | null>(null);
	filter = $state<'all' | 'mine' | 'unassigned'>('mine');
	stateFilter = $state<string>('');
	currentUser = $state<any | null>(null);
	users = $state<any[]>([]);
	
	wsStatus = $state<'connecting' | 'connected' | 'disconnected'>('disconnected');
	ws: WebSocket | null = null;
	reconnectAttempts = 0;
	
	async init() {
		try {
			this.currentUser = await apiRequest('/auth/me');
			if (this.currentUser.role === 'admin') {
				this.filter = 'all';
				this.users = await apiRequest('/workspace/users').catch(() => []);
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
			this.conversations = await apiRequest(url);
		} catch (err) {
			console.error(err);
		}
	}
	
	async selectConversation(convoID: string) {
		this.activeConvoID = convoID;
		this.messages = [];
		this.nextCursor = null;
		
		try {
			this.activeConvo = await apiRequest(`/conversations/${convoID}`);
			await this.loadMessages(true);
			await apiRequest(`/conversations/${convoID}/read`, { method: 'POST' });
			const index = this.conversations.findIndex(c => c.id === convoID);
			if (index !== -1) {
				this.conversations[index].unread = false;
			}
		} catch (err) {
			console.error(err);
		}
	}
	
	async loadMessages(reset = false) {
		if (!this.activeConvoID) return;
		try {
			const url = `/conversations/${this.activeConvoID}/messages?limit=20` + 
				(this.nextCursor && !reset ? `&before=${this.nextCursor}` : '');
			const res = await apiRequest(url);
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
	
	async sendMessage(text: string) {
		if (!this.activeConvoID || !text.trim()) return;
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
			const res = await apiRequest(`/internal/conversations/${this.activeConvoID}/send`, {
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
			window.dispatchEvent(new CustomEvent('dev-message-sent'));
		} catch (err) {
			console.error('Failed to send message:', err);
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
