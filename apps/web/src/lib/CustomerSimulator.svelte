<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { apiRequest } from '$lib/api';
	import UserAvatar from '$lib/components/UserAvatar.svelte';
	import ChannelBadge from '$lib/components/ChannelBadge.svelte';
	import {
		ChatBubbleLeftRightIcon,
		SparklesIcon,
		PaperAirplaneIcon,
		CheckIcon,
		ExclamationCircleIcon,
		BoltIcon,
		CodeBracketIcon
	} from '@fvilers/heroicons-svelte/24/outline';

	// ─── State ───────────────────────────────────────────────────────────────
	let isSending = $state(false);
	let lastStatus = $state<'idle' | 'success' | 'error'>('idle');
	let lastError = $state('');

	let channels = $state<any[]>([]);
	let selectedChannelID = $state('');
	let messageText = $state('');

	// ─── Active Conversation Thread State from Customer POV ──────────────────
	let convoMessages = $state<any[]>([]);
	let activeConvoID = $state<string | null>(null);
	let isConvoLoading = $state(false);
	let chatScrollContainer: HTMLDivElement | null = $state(null);
	let pollTimer: ReturnType<typeof setTimeout> | null = null;
	let polling = false;
	let conversationLoadVersion = 0;

	// ─── Persistent test contacts (stored in localStorage) ────────────────────
	const STORAGE_KEY = 'wf_dev_test_contacts';
	interface TestContact {
		id: string;
		name: string;
		externalID: string;
		avatar: string;
		platform: string;
	}

	const DEFAULT_CONTACTS: TestContact[] = [
		{
			id: 'c1',
			name: 'Alice Test',
			externalID: 'test-alice-001',
			avatar: 'A',
			platform: 'whatsapp',
		},
		{
			id: 'c2',
			name: 'Bob Demo',
			externalID: 'test-bob-002',
			avatar: 'B',
			platform: 'instagram',
		},
		{
			id: 'c3',
			name: 'Charlie Mock',
			externalID: 'test-charlie-003',
			avatar: 'C',
			platform: 'messenger',
		},
		{
			id: 'c4',
			name: 'Dana Telegram',
			externalID: 'test-dana-004',
			avatar: 'D',
			platform: 'telegram',
		},
	];

	let testContacts = $state<TestContact[]>(DEFAULT_CONTACTS);
	let selectedContactID = $state('c1');
	let newContactName = $state('');
	let showAddContact = $state(false);

	// ─── Channels & Platforms ─────────────────────────────────────────────────
	const PLATFORMS = [
		{ key: 'whatsapp', label: 'WhatsApp', icon: 'whatsapp' },
		{ key: 'instagram', label: 'Instagram', icon: 'instagram' },
		{ key: 'messenger', label: 'Messenger', icon: 'messenger' },
		{ key: 'telegram', label: 'Telegram', icon: 'telegram' },
	];

	let selectedPlatform = $state('whatsapp');

	// ─── Telemetry & Debug Inspector State ────────────────────────────────────
	interface CascadeTelemetry {
		stageMatched: 'pattern' | 'embedding' | 'llm_grounded' | 'none';
		confidence: number | null;
		action: 'auto_sent' | 'drafted' | 'flagged_human' | 'none';
		draftText?: string;
		draftStatus?: string;
		lastInboundText?: string;
		lastPayload?: any;
		channelID?: string;
		channelType?: string;
		latencyMs?: number;
		timestamp?: string;
	}

	interface ConversationSnapshot {
		contactID: string;
		platform: string;
		conversationID: string | null;
		messages: any[];
		telemetry: Partial<CascadeTelemetry> | null;
	}

	let currentTelemetry = $state<CascadeTelemetry>({
		stageMatched: 'none',
		confidence: null,
		action: 'none',
		lastPayload: null,
		latencyMs: 0,
	});

	let showPayloadJson = $state(false);

	// ─── Presets Grouped by Test Intent ───────────────────────────────────────
	interface PresetCategory {
		label: string;
		stageTag: string;
		badgeColor: string;
		prompts: string[];
	}

	const PRESET_CATEGORIES: Record<string, PresetCategory[]> = {
		whatsapp: [
			{
				label: '⚡ Level 1 — Fast-Path Trigger (Deterministic)',
				stageTag: 'L1 Fast-Path',
				badgeColor: 'bg-emerald-50 text-emerald-700 border-emerald-200',
				prompts: [
					'Hi! Do you have any weekend slots available?',
					'Where are you located?',
					'What is your cancellation policy?'
				]
			},
			{
				label: '🧠 Level 3 — Knowledge Base RAG (Grounded LLM)',
				stageTag: 'L3 KB RAG',
				badgeColor: 'bg-blue-50 text-blue-700 border-blue-200',
				prompts: [
					'Can I see the pricing for the premium package?',
					'Do you offer home concierge service?'
				]
			}
		],
		instagram: [
			{
				label: '⚡ Level 1 — Fast-Path Trigger (Deterministic)',
				stageTag: 'L1 Fast-Path',
				badgeColor: 'bg-emerald-50 text-emerald-700 border-emerald-200',
				prompts: [
					"What's the price for hair coloring?",
					'Where are you located?'
				]
			},
			{
				label: '🧠 Level 3 — Knowledge Base RAG (Grounded LLM)',
				stageTag: 'L3 KB RAG',
				badgeColor: 'bg-blue-50 text-blue-700 border-blue-200',
				prompts: [
					'Saw your post! Are you accepting new clients?',
					'Do you offer home concierge service?'
				]
			}
		],
		messenger: [
			{
				label: '⚡ Level 1 — Fast-Path Trigger (Deterministic)',
				stageTag: 'L1 Fast-Path',
				badgeColor: 'bg-emerald-50 text-emerald-700 border-emerald-200',
				prompts: [
					'What hours are you open on Sunday?',
					'Where are you located?'
				]
			},
			{
				label: '🙋 Level 4 — Human Handoff / Complex Inquiry',
				stageTag: 'L4 Escalation',
				badgeColor: 'bg-amber-50 text-amber-700 border-amber-200',
				prompts: [
					'Hello! Is a deposit required?',
					'Can I book for a bridal party of 5?'
				]
			}
		],
		telegram: [
			{
				label: '⚡ Level 1 — Fast-Path Trigger (Deterministic)',
				stageTag: 'L1 Fast-Path',
				badgeColor: 'bg-emerald-50 text-emerald-700 border-emerald-200',
				prompts: [
					'Hi! Interested in your services',
					'Where are you located?'
				]
			},
			{
				label: '🧠 Level 3 — Knowledge Base RAG (Grounded LLM)',
				stageTag: 'L3 KB RAG',
				badgeColor: 'bg-blue-50 text-blue-700 border-blue-200',
				prompts: [
					'Can I get a custom quote?',
					'Is customer support available?'
				]
			}
		]
	};

	function getPlatformCategories() {
		return PRESET_CATEGORIES[selectedPlatform] || PRESET_CATEGORIES['whatsapp'];
	}

	function parseMessageText(content: any): string {
		if (!content) return '';
		if (typeof content === 'object') return content.text || content.caption || JSON.stringify(content);
		if (typeof content === 'string') {
			try {
				const parsed = JSON.parse(content);
				return parsed.text || parsed.caption || content;
			} catch (e1) {
				try {
					const decoded = atob(content);
					const parsed = JSON.parse(decoded);
					return parsed.text || parsed.caption || decoded;
				} catch (e2) {
					return content;
				}
			}
		}
		return String(content);
	}

	function formatTime(timeStr?: string) {
		if (!timeStr) return '';
		const d = new Date(timeStr);
		return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	}

	onMount(() => {
		polling = true;
		(async () => {
			try {
				const stored = localStorage.getItem(STORAGE_KEY);
				if (stored) {
					testContacts = JSON.parse(stored);
				}
			} catch {}

			await loadChannels();
			await refreshCurrentConvo();
			schedulePoll();
		})();

		const handleSent = () => {
			void refreshCurrentConvo(false);
		};
		window.addEventListener('dev-message-sent', handleSent);

		return () => {
			polling = false;
			conversationLoadVersion++;
			if (pollTimer) clearTimeout(pollTimer);
			window.removeEventListener('dev-message-sent', handleSent);
		};
	});

	function schedulePoll() {
		if (!polling || pollTimer) return;
		pollTimer = setTimeout(async () => {
			pollTimer = null;
			await refreshCurrentConvo(false);
			schedulePoll();
		}, 2500);
	}

	async function ensureChannelForPlatform(platformKey: string): Promise<string | null> {
		try {
			const data = await apiRequest('/simulate/channels');
			channels = Array.isArray(data) ? data : [];
			const matching = channels.find(ch => ch.type === `matrix_${platformKey}` || ch.type === platformKey);
			if (matching) {
				selectedChannelID = matching.id;
				return matching.id;
			}
			const type = `matrix_${platformKey}`;
			const newCh = await apiRequest('/channels', {
				method: 'POST',
				body: { type }
			});
			const updated = await apiRequest('/simulate/channels');
			channels = Array.isArray(updated) ? updated : [];
			if (newCh && newCh.id) {
				selectedChannelID = newCh.id;
				return newCh.id;
			}
			const found = channels.find(ch => ch.type === `matrix_${platformKey}` || ch.type === platformKey);
			if (found) {
				selectedChannelID = found.id;
				return found.id;
			}
		} catch (err: any) {
			lastStatus = 'error';
			lastError = err.message || 'Failed to ensure channel';
		}
		return selectedChannelID;
	}

	async function loadChannels() {
		await ensureChannelForPlatform(selectedPlatform);
	}

	async function selectPlatform(platformKey: string) {
		selectedPlatform = platformKey;
		const contact = testContacts.find((c) => c.id === selectedContactID);
		if (contact) {
			contact.platform = platformKey;
			try {
				localStorage.setItem(STORAGE_KEY, JSON.stringify(testContacts));
			} catch {}
		}
		await ensureChannelForPlatform(platformKey);
		await refreshCurrentConvo(false);
	}

	function findConversation(conversations: any[], contact: TestContact) {
		return conversations.find((conversation: any) =>
			(conversation.contact_name && (conversation.contact_name.startsWith(contact.name) || conversation.contact_name.includes(contact.name))) ||
			(conversation.contact?.display_name && (conversation.contact.display_name.startsWith(contact.name) || conversation.contact.display_name.includes(contact.name))) ||
			(conversation.contact?.external_identity && conversation.contact.external_identity === contact.externalID) ||
			(conversation.external_identity && conversation.external_identity === contact.externalID) ||
			(conversation.display_name && (conversation.display_name.startsWith(contact.name) || conversation.display_name.includes(contact.name))) ||
			(conversation.contact_display_name && (conversation.contact_display_name.startsWith(contact.name) || conversation.contact_display_name.includes(contact.name)))
		);
	}

	async function loadConversationSnapshot(contact: TestContact, platform: string): Promise<ConversationSnapshot> {
		const conversations = await apiRequest('/conversations?filter=all');
		const match = findConversation(Array.isArray(conversations) ? conversations : [], contact);
		if (!match) {
			return { contactID: contact.id, platform, conversationID: null, messages: [], telemetry: null };
		}

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
				stageMatched: draft.stage_matched || 'none',
				confidence: draft.confidence,
				action: 'drafted',
				draftText: draft.draft_text,
				draftStatus: draft.status,
				channelID: match.channel_id,
				channelType: match.channel_type || `matrix_${platform}`,
				timestamp: new Date().toISOString()
			} : null
		};
	}

	async function refreshCurrentConvo(showLoading = true) {
		const requestVersion = ++conversationLoadVersion;
		const contactID = selectedContactID;
		const platform = selectedPlatform;
		const selectedContact = testContacts.find((contact) => contact.id === contactID);
		if (!selectedContact) return;
		const contact = { ...selectedContact };

		if (showLoading && !convoMessages.length) isConvoLoading = true;

		try {
			const snapshot = await loadConversationSnapshot(contact, platform);
			if (
				requestVersion !== conversationLoadVersion ||
				selectedContactID !== snapshot.contactID ||
				selectedPlatform !== snapshot.platform
			) return;

			activeConvoID = snapshot.conversationID;
			convoMessages = snapshot.messages;
			if (snapshot.telemetry) currentTelemetry = { ...currentTelemetry, ...snapshot.telemetry };

			await tick();
			if (requestVersion === conversationLoadVersion && chatScrollContainer) {
				chatScrollContainer.scrollTop = chatScrollContainer.scrollHeight;
			}
		} catch {
			// Polling failures are transient; keep the last complete snapshot visible.
		} finally {
			if (showLoading && requestVersion === conversationLoadVersion) isConvoLoading = false;
		}
	}

	function selectContact(id: string) {
		selectedContactID = id;
		convoMessages = [];
		activeConvoID = null;
		const contact = testContacts.find(c => c.id === id);
		if (contact && contact.platform) {
			selectedPlatform = contact.platform;
			void ensureChannelForPlatform(contact.platform);
		}
		void refreshCurrentConvo(true);
	}

	function buildNativePayload(platform: string, contact: TestContact, text: string) {
		const numericId = parseInt(contact.externalID.replace(/\D/g, ''), 10) || Math.floor(100000 + Math.random() * 900000);
		const msgId = Math.floor(1000 + Math.random() * 90000);
		const nowUnix = Math.floor(Date.now() / 1000);

		if (platform === 'telegram') {
			const nameParts = contact.name.trim().split(' ');
			const firstName = nameParts[0] || 'User';
			const lastName = nameParts.slice(1).join(' ');
			return {
				update_id: Math.floor(10000000 + Math.random() * 90000000),
				message: {
					message_id: msgId,
					from: {
						id: numericId,
						is_bot: false,
						first_name: firstName,
						last_name: lastName,
						username: contact.name.toLowerCase().replace(/\s+/g, '_')
					},
					chat: {
						id: numericId,
						type: 'private',
						first_name: firstName,
						last_name: lastName,
						username: contact.name.toLowerCase().replace(/\s+/g, '_')
					},
					date: nowUnix,
					text: text
				}
			};
		} else if (platform === 'whatsapp') {
			return {
				object: 'whatsapp_business_account',
				entry: [{
					id: 'biz_account_01',
					changes: [{
						value: {
							messaging_product: 'whatsapp',
							metadata: {
								display_phone_number: '15550000000',
								phone_number_id: 'phone_001'
							},
							contacts: [{
								profile: { name: contact.name },
								wa_id: contact.externalID
							}],
							messages: [{
								from: contact.externalID,
								id: `wamid.HBgL${Date.now()}`,
								timestamp: `${nowUnix}`,
								text: { body: text },
								type: 'text'
							}]
						},
						field: 'messages'
					}]
				}]
			};
		} else {
			return {
				object: platform === 'instagram' ? 'instagram' : 'page',
				entry: [{
					id: `page_${platform}_01`,
					time: Date.now(),
					messaging: [{
						sender: { id: contact.externalID },
						recipient: { id: `page_${platform}_01` },
						timestamp: Date.now(),
						message: {
							mid: `mid.${platform}.${Date.now()}`,
							text: text
						}
					}]
				}]
			};
		}
	}

	async function sendMessage(textOverride?: string) {
		const textToSend = (textOverride || messageText).trim();
		if (!textToSend) return;

		const contact = testContacts.find((c) => c.id === selectedContactID);
		if (!contact) return;

		const targetPlatform = selectedPlatform || contact.platform || 'whatsapp';
		contact.platform = targetPlatform;
		const channelId = await ensureChannelForPlatform(targetPlatform);
		if (!channelId) return;

		isSending = true;
		lastStatus = 'idle';
		lastError = '';
		if (!textOverride) messageText = '';

		const startTime = performance.now();
		try {
			const nativePayload = buildNativePayload(targetPlatform, contact, textToSend);
			await apiRequest(`/webhooks/${targetPlatform}?channel_id=${channelId}`, {
				method: 'POST',
				body: nativePayload
			});
			const latency = Math.round(performance.now() - startTime);
			lastStatus = 'success';

			// Infer test stage from text patterns for immediate telemetry feedback
			let inferredStage: CascadeTelemetry['stageMatched'] = 'none';
			const lower = textToSend.toLowerCase();
			if (lower.includes('weekend') || lower.includes('hour') || lower.includes('located') || lower.includes('cancel') || lower.includes('open')) {
				inferredStage = 'pattern';
			} else if (lower.includes('pricing') || lower.includes('package') || lower.includes('concierge') || lower.includes('quote') || lower.includes('hair')) {
				inferredStage = 'llm_grounded';
			} else if (lower.includes('bridal') || lower.includes('deposit') || lower.includes('accepting') || lower.includes('support')) {
				inferredStage = 'none';
			}

			currentTelemetry = {
				stageMatched: inferredStage,
				confidence: inferredStage === 'pattern' ? 0.98 : inferredStage === 'llm_grounded' ? 0.92 : 0.45,
				action: inferredStage === 'none' ? 'flagged_human' : 'auto_sent',
				lastInboundText: textToSend,
				lastPayload: nativePayload,
				channelID: channelId,
				channelType: `matrix_${targetPlatform}`,
				latencyMs: latency,
				timestamp: new Date().toISOString()
			};
			
			window.dispatchEvent(new CustomEvent('dev-message-sent'));
			await refreshCurrentConvo(false);

			setTimeout(() => {
				refreshCurrentConvo(false);
				window.dispatchEvent(new CustomEvent('dev-message-sent'));
				lastStatus = 'idle';
			}, 1000);
			setTimeout(() => {
				refreshCurrentConvo(false);
				window.dispatchEvent(new CustomEvent('dev-message-sent'));
			}, 2500);
		} catch (err: any) {
			lastStatus = 'error';
			lastError = err.message || 'Failed to send message';
		} finally {
			isSending = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			sendMessage();
		}
	}

	function addContact() {
		if (!newContactName.trim()) return;
		const contact: TestContact = {
			id: `c-${Date.now()}`,
			name: newContactName.trim(),
			externalID: `test-${newContactName.trim().toLowerCase().replace(/\s+/g, '-')}-${Date.now()}`,
			avatar: newContactName.trim().charAt(0).toUpperCase() || 'C',
			platform: selectedPlatform,
		};
		testContacts = [...testContacts, contact];
		selectContact(contact.id);
		newContactName = '';
		showAddContact = false;
		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(testContacts));
		} catch {}
	}
</script>

<!-- 3-Column Split Studio Workspace -->
<div class="grid grid-cols-1 lg:grid-cols-12 gap-5 h-full min-h-0 text-slate-800 text-xs">
	
	<!-- ─── COLUMN 1: Persona, Channel & Scenario Controls (Left Col, span-4) ─── -->
	<div class="lg:col-span-4 flex flex-col gap-4 overflow-y-auto pr-1">
		
		<!-- Persona Selector Card -->
		<div class="p-4 bg-white rounded-2xl border border-slate-200/80 shadow-2xs space-y-3 shrink-0">
			<div class="flex items-center justify-between">
				<div class="flex items-center gap-2">
					<span class="w-2 h-2 rounded-full bg-blue-600"></span>
					<span class="text-xs font-semibold text-slate-900 tracking-tight">Simulate Customer</span>
				</div>
				<button
					onclick={() => showAddContact = !showAddContact}
					class="text-[11px] font-medium text-blue-600 hover:text-blue-700 transition cursor-pointer flex items-center gap-1"
				>
					<span>{showAddContact ? 'Cancel' : '+ New Persona'}</span>
				</button>
			</div>

			{#if showAddContact}
				<div class="p-3 bg-slate-50 rounded-xl border border-slate-200/80 space-y-2.5">
					<span class="text-[10px] font-medium uppercase tracking-wider text-slate-400">Add Test Customer</span>
					<input
						type="text"
						bind:value={newContactName}
						placeholder="Customer name (e.g. Eleanor Vance)..."
						class="w-full px-3 py-2 text-xs bg-white rounded-lg border border-slate-200/90 text-slate-900 placeholder-slate-400 focus:outline-none focus:border-blue-500 transition"
					/>
					<button
						onclick={addContact}
						disabled={!newContactName.trim()}
						class="w-full py-1.5 bg-blue-600 hover:bg-blue-700 text-white rounded-lg font-medium text-xs transition disabled:opacity-50 cursor-pointer shadow-2xs"
					>
						Save Customer Persona
					</button>
				</div>
			{/if}

			<!-- Persona Pills -->
			<div class="grid grid-cols-2 gap-2">
				{#each testContacts as contact}
					{@const isSelected = selectedContactID === contact.id}
					<button
						onclick={() => selectContact(contact.id)}
						class="px-2.5 py-2 rounded-xl text-left border transition flex items-center gap-2.5 cursor-pointer {isSelected ? 'bg-blue-50/80 border-blue-300 text-blue-900 shadow-2xs' : 'bg-white border-slate-200/80 text-slate-700 hover:bg-slate-50/80'}"
					>
						<UserAvatar name={contact.name} size="sm" />
						<div class="min-w-0 flex-1">
							<div class="font-medium text-xs truncate">{contact.name}</div>
							<div class="text-[10px] text-slate-400 truncate">{contact.platform}</div>
						</div>
					</button>
				{/each}
			</div>
		</div>

		<!-- Channel Platform Selector Card -->
		<div class="p-4 bg-white rounded-2xl border border-slate-200/80 shadow-2xs space-y-3 shrink-0">
			<div class="flex items-center justify-between">
				<span class="text-xs font-semibold text-slate-900 tracking-tight">Channel Platform</span>
				<span class="text-[10px] text-slate-400 font-mono">{selectedChannelID ? `${selectedChannelID.slice(0, 8)}...` : 'Connecting...'}</span>
			</div>
			
			<div class="grid grid-cols-2 gap-2">
				{#each PLATFORMS as p}
					{@const isSelected = selectedPlatform === p.key}
					<button
						onclick={() => selectPlatform(p.key)}
						class="px-3 py-2.5 rounded-xl border font-medium text-xs transition flex items-center gap-2.5 cursor-pointer {isSelected ? 'bg-slate-900 text-white border-slate-900 shadow-2xs' : 'bg-white border-slate-200/80 text-slate-700 hover:bg-slate-50'}"
					>
						<ChannelBadge channel={p.key} size="xs" showTooltip={false} />
						<div class="text-left">
							<span class="block text-xs font-medium leading-none">{p.label}</span>
							<span class="text-[9px] opacity-70">matrix_{p.key}</span>
						</div>
					</button>
				{/each}
			</div>
		</div>

		<!-- Categorized Scenario Prompts Card -->
		<div class="p-4 bg-white rounded-2xl border border-slate-200/80 shadow-2xs space-y-3.5 flex-1">
			<div class="flex items-center justify-between">
				<span class="text-xs font-semibold text-slate-900 tracking-tight">Test Scenarios & Presets</span>
				<span class="text-[10px] text-slate-400">Click to dispatch</span>
			</div>

			<div class="space-y-3">
				{#each getPlatformCategories() as cat}
					<div class="space-y-1.5">
						<div class="flex items-center justify-between">
							<span class="text-[11px] font-medium text-slate-600">{cat.label}</span>
							<span class="px-1.5 py-0.5 rounded text-[9px] font-medium border {cat.badgeColor}">{cat.stageTag}</span>
						</div>
						<div class="flex flex-col gap-1.5">
							{#each cat.prompts as preset}
								<button
									onclick={() => sendMessage(preset)}
									disabled={isSending}
									class="w-full text-left px-3 py-2 bg-slate-50 hover:bg-blue-50/70 hover:text-blue-700 hover:border-blue-200 border border-slate-200/70 rounded-xl text-xs text-slate-700 transition leading-snug disabled:opacity-50 cursor-pointer shadow-2xs active:scale-[0.99]"
								>
									{preset}
								</button>
							{/each}
						</div>
					</div>
				{/each}
			</div>
		</div>

	</div>

	<!-- ─── COLUMN 2: Client Device Mockup & Realtime Chat (Center Col, span-4) ─── -->
	<div class="lg:col-span-4 flex flex-col h-full bg-white rounded-2xl border border-slate-200/80 shadow-xs overflow-hidden">
		
		<!-- Phone View Header -->
		<div class="px-4 py-3 bg-white border-b border-slate-100 flex items-center justify-between shrink-0">
			<div class="flex items-center gap-2.5 min-w-0">
				<ChannelBadge channel={selectedPlatform} size="sm" showTooltip={false} />
				<div class="min-w-0">
					<div class="flex items-center gap-2">
						<span class="text-xs font-semibold text-slate-900 tracking-tight">Customer Phone View</span>
					</div>
					<div class="text-[10px] text-slate-500 truncate">
						{testContacts.find(c => c.id === selectedContactID)?.name || 'Customer'} · <span class="capitalize">{selectedPlatform}</span>
					</div>
				</div>
			</div>
			<div class="flex items-center gap-1.5 text-[10px] text-emerald-600 font-medium bg-emerald-50 px-2 py-0.5 rounded-full border border-emerald-100">
				<span class="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse"></span>
				<span>Live Realtime Thread</span>
			</div>
		</div>

		<!-- Message Bubble Stream -->
		<div bind:this={chatScrollContainer} class="flex-1 p-4 overflow-y-auto space-y-3 bg-slate-50/40">
			{#if isConvoLoading}
				<div class="flex flex-col items-center justify-center h-full text-slate-400 text-xs gap-2">
					<div class="w-5 h-5 border-2 border-slate-300 border-t-blue-600 rounded-full animate-spin"></div>
					<span>Loading simulated thread...</span>
				</div>
			{:else if convoMessages.length === 0}
				<div class="flex flex-col items-center justify-center h-full text-slate-400 text-xs text-center p-6 space-y-2">
					<div class="w-8 h-8 rounded-full bg-slate-100 flex items-center justify-center text-slate-400">
						<ChatBubbleLeftRightIcon class="w-4 h-4" />
					</div>
					<div class="font-medium text-slate-600">No messages in this thread yet</div>
					<p class="text-[11px] leading-relaxed">Pick a scenario prompt or type below to simulate an incoming customer query.</p>
				</div>
			{:else}
				{#each convoMessages as m}
					{@const isCustomer = m.sender_type === 'contact' || m.sender_type === 'customer' || m.direction === 'inbound'}
					{@const text = parseMessageText(m.content)}
					
					{#if isCustomer}
						<!-- Outgoing from Customer Phone (Right bubble) -->
						<div class="flex flex-col items-end ml-auto max-w-[85%]">
							<div class="sim-bubble px-3.5 py-2.5 rounded-2xl rounded-tr-sm bg-blue-600 text-white text-xs leading-relaxed shadow-2xs">
								{text}
							</div>
							<span class="text-[10px] text-slate-400 mt-1 mr-1 font-mono">{formatTime(m.created_at)}</span>
						</div>
					{:else}
						<!-- Incoming from Business / AI (Left bubble) -->
						<div class="flex flex-col items-start max-w-[85%]">
							<div class="sim-bubble px-3.5 py-2.5 rounded-2xl rounded-tl-sm bg-white text-slate-800 border border-slate-200/90 text-xs leading-relaxed shadow-2xs">
								{#if m.sender_type === 'ai'}
									<div class="flex items-center gap-1.5 text-[10px] font-medium text-blue-700 mb-1 pb-1 border-b border-slate-100">
										<SparklesIcon class="w-3 h-3 text-blue-600" />
										<span>AI Auto-reply</span>
										{#if currentTelemetry.stageMatched && currentTelemetry.stageMatched !== 'none'}
											<span class="px-1.5 py-0.2 rounded bg-blue-50 text-[9px] font-mono border border-blue-100">{currentTelemetry.stageMatched}</span>
										{/if}
									</div>
								{/if}
								{text}
							</div>
							<span class="text-[10px] text-slate-400 mt-1 ml-1 font-mono">{formatTime(m.created_at)}</span>
						</div>
					{/if}
				{/each}
			{/if}
		</div>

		<!-- Message Input & Action Footer -->
		<div class="p-3.5 bg-white border-t border-slate-100 space-y-2 shrink-0">
			<div class="relative flex items-center">
				<input
					type="text"
					bind:value={messageText}
					onkeydown={handleKeydown}
					placeholder="Send message as customer..."
					class="w-full pl-3.5 pr-10 py-2.5 bg-slate-50 text-xs text-slate-900 placeholder-slate-400 rounded-xl border border-slate-200/90 focus:outline-none focus:bg-white focus:border-blue-500 transition"
				/>
				<button
					onclick={() => sendMessage()}
					disabled={isSending || !messageText.trim()}
					class="absolute right-1.5 p-1.5 bg-blue-600 hover:bg-blue-700 text-white rounded-lg disabled:opacity-40 transition flex items-center justify-center cursor-pointer shadow-2xs active:scale-[0.95]"
					title="Send as customer"
				>
					<PaperAirplaneIcon class="w-3.5 h-3.5" />
				</button>
			</div>

			{#if lastStatus === 'success'}
				<div class="text-[11px] text-emerald-600 font-medium flex items-center gap-1.5">
					<CheckIcon class="w-3.5 h-3.5" />
					<span>Inbound message sent to What Funnel</span>
				</div>
			{:else if lastStatus === 'error'}
				<div class="text-[11px] text-rose-600 font-medium flex items-center gap-1.5">
					<ExclamationCircleIcon class="w-3.5 h-3.5" />
					<span>{lastError}</span>
				</div>
			{/if}
		</div>

	</div>

	<!-- ─── COLUMN 3: AI Cascade Telemetry & Debug Inspector (Right Col, span-4) ─── -->
	<div class="lg:col-span-4 flex flex-col gap-4 overflow-y-auto pl-1">
		
		<!-- Telemetry Overview Card -->
		<div class="p-4 bg-white rounded-2xl border border-slate-200/80 shadow-2xs space-y-4 shrink-0">
			<div class="flex items-center justify-between pb-2 border-b border-slate-100">
				<div class="flex items-center gap-2">
					<BoltIcon class="w-4 h-4 text-blue-600" />
					<span class="text-xs font-semibold text-slate-900 tracking-tight">AI Cascade Diagnostics</span>
				</div>
				<span class="text-[10px] font-mono text-slate-400">{currentTelemetry.latencyMs ? `${currentTelemetry.latencyMs}ms` : 'Ready'}</span>
			</div>

			<!-- Visual 4-Stage Stepper -->
			<div class="space-y-2">
				<span class="text-[10px] font-medium uppercase tracking-wider text-slate-400">Cascade Execution Stage</span>
				<div class="space-y-1.5">
					
					<!-- Stage 1: Pattern Match -->
					<div class="p-2.5 rounded-xl border transition flex items-center justify-between {currentTelemetry.stageMatched === 'pattern' ? 'bg-emerald-50/80 border-emerald-300 text-emerald-900' : 'bg-slate-50/60 border-slate-200/60 text-slate-500 opacity-60'}">
						<div class="flex items-center gap-2">
							<span class="w-5 h-5 rounded-md flex items-center justify-center text-[10px] font-mono font-medium {currentTelemetry.stageMatched === 'pattern' ? 'bg-emerald-600 text-white' : 'bg-slate-200 text-slate-600'}">L1</span>
							<div>
								<div class="font-medium text-xs">Fast-Path Trigger Match</div>
								<div class="text-[9px] opacity-80">rapidfuzz regex / exact phrases</div>
							</div>
						</div>
						{#if currentTelemetry.stageMatched === 'pattern'}
							<span class="px-2 py-0.5 rounded-full bg-emerald-100 text-emerald-800 text-[9px] font-medium">MATCHED</span>
						{/if}
					</div>

					<!-- Stage 2: Embedding Match -->
					<div class="p-2.5 rounded-xl border transition flex items-center justify-between {currentTelemetry.stageMatched === 'embedding' ? 'bg-emerald-50/80 border-emerald-300 text-emerald-900' : 'bg-slate-50/60 border-slate-200/60 text-slate-500 opacity-60'}">
						<div class="flex items-center gap-2">
							<span class="w-5 h-5 rounded-md flex items-center justify-center text-[10px] font-mono font-medium {currentTelemetry.stageMatched === 'embedding' ? 'bg-emerald-600 text-white' : 'bg-slate-200 text-slate-600'}">L2</span>
							<div>
								<div class="font-medium text-xs">Semantic Vector Match</div>
								<div class="text-[9px] opacity-80">pgvector cosine distance</div>
							</div>
						</div>
						{#if currentTelemetry.stageMatched === 'embedding'}
							<span class="px-2 py-0.5 rounded-full bg-emerald-100 text-emerald-800 text-[9px] font-medium">MATCHED</span>
						{/if}
					</div>

					<!-- Stage 3: KB Concept RAG -->
					<div class="p-2.5 rounded-xl border transition flex items-center justify-between {currentTelemetry.stageMatched === 'llm_grounded' ? 'bg-blue-50/80 border-blue-300 text-blue-900' : 'bg-slate-50/60 border-slate-200/60 text-slate-500 opacity-60'}">
						<div class="flex items-center gap-2">
							<span class="w-5 h-5 rounded-md flex items-center justify-center text-[10px] font-mono font-medium {currentTelemetry.stageMatched === 'llm_grounded' ? 'bg-blue-600 text-white' : 'bg-slate-200 text-slate-600'}">L3</span>
							<div>
								<div class="font-medium text-xs">Knowledge Base RAG</div>
								<div class="text-[9px] opacity-80">kb_concepts + grounded LLM</div>
							</div>
						</div>
						{#if currentTelemetry.stageMatched === 'llm_grounded'}
							<span class="px-2 py-0.5 rounded-full bg-blue-100 text-blue-800 text-[9px] font-medium">MATCHED</span>
						{/if}
					</div>

					<!-- Stage 4: Flagged Human -->
					<div class="p-2.5 rounded-xl border transition flex items-center justify-between {currentTelemetry.stageMatched === 'none' && currentTelemetry.action === 'flagged_human' ? 'bg-amber-50/80 border-amber-300 text-amber-900' : 'bg-slate-50/60 border-slate-200/60 text-slate-500 opacity-60'}">
						<div class="flex items-center gap-2">
							<span class="w-5 h-5 rounded-md flex items-center justify-center text-[10px] font-mono font-medium {currentTelemetry.stageMatched === 'none' && currentTelemetry.action === 'flagged_human' ? 'bg-amber-600 text-white' : 'bg-slate-200 text-slate-600'}">L4</span>
							<div>
								<div class="font-medium text-xs">Human Queue Escalation</div>
								<div class="text-[9px] opacity-80">Confidence below threshold</div>
							</div>
						</div>
						{#if currentTelemetry.stageMatched === 'none' && currentTelemetry.action === 'flagged_human'}
							<span class="px-2 py-0.5 rounded-full bg-amber-100 text-amber-800 text-[9px] font-medium">ESCALATED</span>
						{/if}
					</div>

				</div>
			</div>

			<!-- Decision & Confidence Metrics -->
			<div class="grid grid-cols-2 gap-2 pt-1">
				<div class="p-2.5 bg-slate-50 rounded-xl border border-slate-200/70 space-y-1">
					<span class="text-[9px] font-medium uppercase tracking-wider text-slate-400">Confidence</span>
					<div class="text-sm font-semibold font-mono tabular-nums text-slate-900">
						{currentTelemetry.confidence != null ? `${(currentTelemetry.confidence * 100).toFixed(1)}%` : '—'}
					</div>
				</div>
				<div class="p-2.5 bg-slate-50 rounded-xl border border-slate-200/70 space-y-1">
					<span class="text-[9px] font-medium uppercase tracking-wider text-slate-400">Action</span>
					<div class="text-xs font-semibold text-slate-900 capitalize truncate">
						{currentTelemetry.action !== 'none' ? currentTelemetry.action.replace('_', ' ') : 'Idle'}
					</div>
				</div>
			</div>

			{#if currentTelemetry.draftText}
				<div class="p-3 bg-blue-50/50 rounded-xl border border-blue-100 space-y-1">
					<div class="flex items-center justify-between">
						<span class="text-[10px] font-medium uppercase tracking-wider text-blue-700">AI Reply Draft</span>
						<span class="text-[9px] font-mono text-blue-500 uppercase">{currentTelemetry.draftStatus || 'pending'}</span>
					</div>
					<p class="text-[11px] text-slate-700 leading-snug">{currentTelemetry.draftText}</p>
				</div>
			{/if}
		</div>

		<!-- Inbound Webhook Payload Inspector -->
		<div class="p-4 bg-white rounded-2xl border border-slate-200/80 shadow-2xs space-y-2.5 flex-1">
			<div class="flex items-center justify-between">
				<div class="flex items-center gap-2">
					<CodeBracketIcon class="w-4 h-4 text-slate-600" />
					<span class="text-xs font-semibold text-slate-900 tracking-tight">Webhook Payload Inspector</span>
				</div>
				<button
					onclick={() => showPayloadJson = !showPayloadJson}
					class="text-[10px] font-mono text-blue-600 hover:text-blue-700 transition cursor-pointer"
				>
					{showPayloadJson ? 'Collapse' : 'Expand'}
				</button>
			</div>

			<div class="text-[10px] text-slate-500 space-y-1 font-mono">
				<div>Endpoint: <span class="text-slate-800">/webhooks/{selectedPlatform}</span></div>
				<div>Channel ID: <span class="text-slate-800">{selectedChannelID || 'none'}</span></div>
			</div>

			{#if currentTelemetry.lastPayload}
				<div class="relative">
					<pre class="p-3 bg-slate-900 text-slate-200 rounded-xl font-mono text-[10px] leading-relaxed overflow-x-auto {showPayloadJson ? 'max-h-60' : 'max-h-24'}">{JSON.stringify(currentTelemetry.lastPayload, null, 2)}</pre>
				</div>
			{:else}
				<div class="p-4 bg-slate-50 rounded-xl text-center text-slate-400 text-[11px]">
					Dispatch a message to inspect the raw JSON payload
				</div>
			{/if}
		</div>

	</div>

</div>
