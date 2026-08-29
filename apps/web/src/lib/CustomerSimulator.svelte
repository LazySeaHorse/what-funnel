<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { apiRequest } from '$lib/api';

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
	let pollInterval = $state<any>(null);
	let chatScrollContainer: HTMLDivElement | null = $state(null);

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

	// ─── Platforms ───────────────────────────────────────────────────────────
	const PLATFORMS = [
		{ key: 'whatsapp', label: 'WhatsApp', icon: 'WA', bg: 'bg-[#25D366] text-white', border: 'border-[#25D366]' },
		{ key: 'instagram', label: 'Instagram', icon: 'IG', bg: 'bg-gradient-to-tr from-amber-500 via-rose-500 to-purple-600 text-white', border: 'border-rose-400' },
		{ key: 'messenger', label: 'Messenger', icon: 'MS', bg: 'bg-gradient-to-tr from-[#00C6FF] to-[#0078FF] text-white', border: 'border-blue-400' },
		{ key: 'telegram', label: 'Telegram', icon: 'TG', bg: 'bg-sky-500 text-white', border: 'border-sky-400' },
	];

	let selectedPlatform = $state('whatsapp');

	// ─── Preset messages ─────────────────────────────────────────────────────
	const PRESET_MESSAGES: Record<string, string[]> = {
		whatsapp: [
			"Hi! Do you have any weekend slots available?",
			"Can I see the pricing for the premium package?",
			"Where are you located?",
			"What is your cancellation policy?"
		],
		instagram: [
			"Saw your post! Are you accepting new clients?",
			"What's the price for hair coloring?",
			"Do you offer home concierge service?"
		],
		messenger: [
			"Hello! Is a deposit required?",
			"Can I book for a bridal party of 5?",
			"What hours are you open on Sunday?"
		],
		telegram: [
			"Hi! Interested in your services",
			"Can I get a custom quote?",
			"Is customer support available?"
		]
	};

	function getPresets() {
		return PRESET_MESSAGES[selectedPlatform] || PRESET_MESSAGES['whatsapp'];
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
		(async () => {
			try {
				const stored = localStorage.getItem(STORAGE_KEY);
				if (stored) {
					testContacts = JSON.parse(stored);
				}
			} catch {}

			await loadChannels();
			await refreshCurrentConvo();
		})();

		pollInterval = setInterval(() => {
			refreshCurrentConvo(false);
		}, 2500);

		const handleSent = () => {
			refreshCurrentConvo(false);
		};
		window.addEventListener('dev-message-sent', handleSent);

		return () => {
			if (pollInterval) clearInterval(pollInterval);
			window.removeEventListener('dev-message-sent', handleSent);
		};
	});

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

	async function createMockChannel() {
		await ensureChannelForPlatform(selectedPlatform);
	}

	async function refreshCurrentConvo(showLoading = true) {
		const contact = testContacts.find((c) => c.id === selectedContactID);
		if (!contact) return;

		if (showLoading && !convoMessages.length) isConvoLoading = true;

		try {
			const convos = await apiRequest('/conversations?filter=all');
			if (Array.isArray(convos)) {
				const match = convos.find((c: any) => 
					(c.contact_name && (c.contact_name.startsWith(contact.name) || c.contact_name.includes(contact.name))) ||
					(c.contact?.display_name && (c.contact.display_name.startsWith(contact.name) || c.contact.display_name.includes(contact.name))) ||
					(c.contact?.external_identity && c.contact.external_identity === contact.externalID) ||
					(c.external_identity && c.external_identity === contact.externalID) ||
					(c.display_name && (c.display_name.startsWith(contact.name) || c.display_name.includes(contact.name))) ||
					(c.contact_display_name && (c.contact_display_name.startsWith(contact.name) || c.contact_display_name.includes(contact.name)))
				);

				if (match) {
					activeConvoID = match.id;
					const msgRes = await apiRequest(`/conversations/${match.id}/messages?limit=30`);
					if (msgRes && msgRes.messages) {
						convoMessages = msgRes.messages.reverse();
						await tick();
						if (chatScrollContainer) {
							chatScrollContainer.scrollTop = chatScrollContainer.scrollHeight;
						}
					}
				} else {
					convoMessages = [];
				}
			}
		} catch (err) {
			// Ignore polling error
		} finally {
			if (showLoading) isConvoLoading = false;
		}
	}

	function selectContact(id: string) {
		selectedContactID = id;
		convoMessages = [];
		activeConvoID = null;
		const contact = testContacts.find(c => c.id === id);
		if (contact && contact.platform) {
			selectedPlatform = contact.platform;
			ensureChannelForPlatform(contact.platform);
		}
		refreshCurrentConvo(true);
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
			// Instagram or Messenger
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

		try {
			const nativePayload = buildNativePayload(targetPlatform, contact, textToSend);
			await apiRequest(`/webhooks/${targetPlatform}?channel_id=${channelId}`, {
				method: 'POST',
				body: nativePayload
			});
			lastStatus = 'success';
			
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

<div class="flex flex-col h-full space-y-4 text-xs">
	
	<!-- Header / Persona selector -->
	<div class="space-y-2">
		<div class="flex items-center justify-between px-3 py-2 border-b border-slate-100 bg-slate-50 text-xs">
			<span class="font-medium text-slate-700">Simulate Customer</span>
			<button
				onclick={() => showAddContact = !showAddContact}
				class="text-[11px] font-medium text-blue-600 hover:text-blue-700 transition"
			>
				{showAddContact ? 'Cancel' : '+ New Persona'}
			</button>
		</div>

		{#if showAddContact}
			<div class="p-2.5 bg-blue-50/50 rounded-xl border border-blue-100 space-y-2">
				<input
					type="text"
					bind:value={newContactName}
					placeholder="Customer name (e.g. John Doe)..."
					class="w-full p-2 text-xs bg-white rounded-lg border border-slate-200 focus:outline-none"
				/>
				<button
					onclick={addContact}
					class="w-full py-1.5 bg-blue-600 text-white rounded-lg font-medium text-xs hover:bg-blue-700 transition"
				>
					Save Customer Persona
				</button>
			</div>
		{/if}

		<!-- Customer persona pills -->
		<div class="flex items-center gap-1.5 overflow-x-auto pb-1">
			{#each testContacts as contact}
				{@const isSelected = selectedContactID === contact.id}
				<button
					onclick={() => selectContact(contact.id)}
					class="shrink-0 px-2.5 py-1 rounded-xl font-medium flex items-center gap-1.5 border transition {isSelected ? 'bg-blue-50 border-blue-300 text-blue-700' : 'bg-slate-50 border-slate-200/80 text-slate-600 hover:bg-slate-100'}"
				>
					<span class="w-4 h-4 rounded-full bg-blue-600 text-white flex items-center justify-center text-[9px] font-medium">
						{contact.avatar}
					</span>
					<span>{contact.name}</span>
				</button>
			{/each}
		</div>
	</div>

	<!-- Platform Switcher -->
	<div class="space-y-1.5">
		<span class="text-[11px] font-medium text-slate-400 uppercase tracking-wider">Channel Platform</span>
		<div class="grid grid-cols-4 gap-1.5">
			{#each PLATFORMS as p}
				{@const isSelected = selectedPlatform === p.key}
				<button
					onclick={() => selectPlatform(p.key)}
					class="py-1.5 px-2 rounded-xl text-center border font-medium text-[11px] transition flex flex-col items-center gap-0.5 {isSelected ? 'bg-blue-600 text-white border-blue-600' : 'bg-white border-slate-200 text-slate-600 hover:bg-slate-50'}"
				>
					<span>{p.icon}</span>
					<span class="text-[10px]">{p.label}</span>
				</button>
			{/each}
		</div>
	</div>

	<!-- Quick Preset Prompts -->
	<div class="space-y-1.5">
		<span class="text-[11px] font-medium text-slate-400 uppercase tracking-wider">Quick Inbound Prompts</span>
		<div class="flex flex-wrap gap-1.5">
			{#each getPresets() as preset}
				<button
					onclick={() => sendMessage(preset)}
					disabled={isSending}
					class="text-left px-2.5 py-1 bg-slate-50 hover:bg-blue-50 hover:text-blue-600 hover:border-blue-200 border border-slate-200/80 rounded-lg text-[11px] text-slate-600 transition leading-snug disabled:opacity-50"
				>
					{preset}
				</button>
			{/each}
		</div>
	</div>

	<!-- Customer POV Simulated Chat Window -->
	<div class="flex-1 flex flex-col min-h-[320px] bg-slate-50 rounded-2xl border border-slate-200/80 overflow-hidden">
		<div class="px-4 py-2.5 bg-white border-b border-slate-100 flex items-center justify-between shrink-0">
			<div class="flex items-center gap-2">
				<span class="text-xs font-medium text-slate-700">Customer Phone View</span>
				<span class="px-2 py-0.5 rounded-full bg-blue-50 text-blue-600 text-[10px] font-medium">
					{testContacts.find(c => c.id === selectedContactID)?.name || 'Customer'}
				</span>
			</div>
			<span class="text-[10px] text-emerald-600 font-medium flex items-center gap-1">
				<span class="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse"></span>
				Live Realtime Thread
			</span>
		</div>

		<!-- Thread messages -->
		<div bind:this={chatScrollContainer} class="flex-1 p-4 overflow-y-auto space-y-3">
			{#if isConvoLoading}
				<div class="text-center py-10 text-slate-400 text-xs">Loading thread...</div>
			{:else if convoMessages.length === 0}
				<div class="text-center py-12 text-slate-400 text-xs leading-relaxed">
					No messages yet in this simulated thread.<br/>Type below or pick a prompt to simulate an incoming customer inquiry.
				</div>
			{:else}
				{#each convoMessages as m}
					{@const isCustomer = m.sender_type === 'contact' || m.sender_type === 'customer' || m.direction === 'inbound'}
					{@const text = parseMessageText(m.content)}
					
					{#if isCustomer}
						<!-- Outgoing from Customer Phone (Right bubble) -->
						<div class="flex flex-col items-end ml-auto max-w-[80%]">
							<div class="sim-bubble p-3 rounded-2xl rounded-tr-sm bg-blue-600 text-white text-xs leading-relaxed">
								{text}
							</div>
							<span class="text-[10px] text-slate-400 mt-1 mr-1">{formatTime(m.created_at)}</span>
						</div>
					{:else}
						<!-- Incoming from Business / AI (Left bubble) -->
						<div class="flex flex-col items-start max-w-[80%]">
							<div class="sim-bubble p-3 rounded-2xl rounded-tl-sm bg-white text-slate-800 border border-slate-200/70 text-xs leading-relaxed">
								{#if m.sender_type === 'ai'}
									<div class="flex items-center gap-1 text-[10px] font-medium text-purple-600 mb-1">
										<span>✨ AI Auto-reply</span>
									</div>
								{/if}
								{text}
							</div>
							<span class="text-[10px] text-slate-400 mt-1 ml-1">{formatTime(m.created_at)}</span>
						</div>
					{/if}
				{/each}
			{/if}
		</div>
	</div>

	<!-- Custom message input from customer POV -->
	<div class="space-y-2 shrink-0">
		<div class="relative flex items-center">
			<input
				type="text"
				bind:value={messageText}
				onkeydown={handleKeydown}
				placeholder="Send message as customer..."
				class="w-full pl-3 pr-10 py-2.5 bg-white text-xs text-slate-800 placeholder-slate-400 rounded-xl border border-slate-200/90 focus:outline-none focus:border-blue-500"
			/>
			<button
				onclick={() => sendMessage()}
				disabled={isSending || !messageText.trim()}
				class="absolute right-1.5 p-1.5 bg-blue-600 hover:bg-blue-700 text-white rounded-lg disabled:opacity-40 transition flex items-center justify-center"
				title="Send as customer"
			>
				<svg class="w-3.5 h-3.5 rotate-45 -mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
					<path stroke-linecap="round" stroke-linejoin="round" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
				</svg>
			</button>
		</div>

		{#if lastStatus === 'success'}
			<div class="text-[11px] text-emerald-600 font-medium flex items-center gap-1">
				<span>✓ Inbound message sent to What Funnel</span>
			</div>
		{:else if lastStatus === 'error'}
			<div class="text-[11px] text-rose-600 font-medium">
				{lastError}
			</div>
		{/if}
	</div>

</div>
