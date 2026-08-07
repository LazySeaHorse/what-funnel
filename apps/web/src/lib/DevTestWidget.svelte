<script lang="ts">
	import { onMount } from 'svelte';
	import { apiRequest } from '$lib/api';

	// ─── State ───────────────────────────────────────────────────────────────
	let isOpen = $state(false);
	let isMinimized = $state(false);
	let isSending = $state(false);
	let lastStatus = $state<'idle' | 'success' | 'error'>('idle');
	let lastError = $state('');

	let channels = $state<any[]>([]);
	let selectedChannelID = $state('');

	let messageText = $state('');
	let contentType = $state<'text' | 'image' | 'audio'>('text');
	let mediaURL = $state('');

	// ─── Persistent dummy test contacts (stored in localStorage) ─────────────
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
			avatar: '👩',
			platform: 'whatsapp',
		},
		{
			id: 'c2',
			name: 'Bob Demo',
			externalID: 'test-bob-002',
			avatar: '👨',
			platform: 'instagram',
		},
		{
			id: 'c3',
			name: 'Charlie Mock',
			externalID: 'test-charlie-003',
			avatar: '🧑',
			platform: 'twitter',
		},
	];

	let testContacts = $state<TestContact[]>(DEFAULT_CONTACTS);
	let selectedContactID = $state('c1');
	let newContactName = $state('');
	let showAddContact = $state(false);

	// ─── Platforms ───────────────────────────────────────────────────────────
	const PLATFORMS = [
		{ key: 'whatsapp', label: 'WhatsApp', color: '#25D366', icon: '💬' },
		{ key: 'instagram', label: 'Instagram', color: '#E1306C', icon: '📸' },
		{ key: 'twitter', label: 'Twitter DM', color: '#1DA1F2', icon: '🐦' },
		{ key: 'messenger', label: 'Messenger', color: '#0084FF', icon: '💙' },
		{ key: 'telegram', label: 'Telegram', color: '#26A5E4', icon: '✈️' },
	];

	let selectedPlatform = $state('whatsapp');

	// ─── Preset messages ─────────────────────────────────────────────────────
	const PRESET_MESSAGES: Record<string, string[]> = {
		whatsapp: [
			"Hi! I'm interested in your product.",
			"Can you tell me more about pricing?",
			"I'd like to schedule a demo.",
			"Is this still available?",
		],
		instagram: [
			"Saw your post! 🔥 Can I DM you?",
			"What's the price for this?",
			"Do you ship internationally?",
		],
		twitter: [
			"Hey, I saw your tweet. Can we talk?",
			"Interested in a collaboration?",
			"Quick question about your service",
		],
		messenger: [
			"Hello! I came from your Facebook page.",
			"Is customer support available?",
			"I have a question about my order.",
		],
		telegram: [
			"Hi, I found you on Telegram",
			"Is this the right channel for support?",
			"Interested in your services!",
		],
	};

	function getPresetMessages() {
		return PRESET_MESSAGES[selectedPlatform] || [];
	}

	// ─── Lifecycle ───────────────────────────────────────────────────────────
	onMount(async () => {
		// Restore persisted contacts
		try {
			const stored = localStorage.getItem(STORAGE_KEY);
			if (stored) {
				testContacts = JSON.parse(stored);
			}
		} catch {}

		// Load channels
		await loadChannels();
	});

	async function loadChannels() {
		try {
			const data = await apiRequest('/simulate/channels');
			channels = Array.isArray(data) ? data : [];
			if (channels.length > 0 && !selectedChannelID) {
				selectedChannelID = channels[0].id;
			}
		} catch (err) {
			channels = [];
		}
	}

	// ─── Actions ─────────────────────────────────────────────────────────────
	async function sendMessage() {
		if (!messageText.trim() && contentType === 'text') return;
		if (!selectedChannelID) {
			lastStatus = 'error';
			lastError = 'No channel selected. Create a channel in Settings first.';
			return;
		}

		const contact = testContacts.find((c) => c.id === selectedContactID);
		if (!contact) {
			lastStatus = 'error';
			lastError = 'No test contact selected.';
			return;
		}

		isSending = true;
		lastStatus = 'idle';
		lastError = '';

		try {
			await apiRequest('/simulate-inbound', {
				method: 'POST',
				body: {
					channel_id: selectedChannelID,
					sender_external_id: contact.externalID,
					sender_display_name: `${contact.name} (${PLATFORMS.find((p) => p.key === selectedPlatform)?.label ?? selectedPlatform})`,
					sender_avatar_url: '',
					content_type: contentType,
					text: messageText.trim(),
					media_url: contentType !== 'text' ? mediaURL : '',
				},
			});
			lastStatus = 'success';
			messageText = '';
			mediaURL = '';
			// Auto-reset status after 3s
			setTimeout(() => {
				lastStatus = 'idle';
			}, 3000);
		} catch (err: any) {
			lastStatus = 'error';
			lastError = err.message || 'Unknown error';
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
			avatar: '🧑',
			platform: selectedPlatform,
		};
		testContacts = [...testContacts, contact];
		selectedContactID = contact.id;
		newContactName = '';
		showAddContact = false;
		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(testContacts));
		} catch {}
	}

	function removeContact(id: string) {
		testContacts = testContacts.filter((c) => c.id !== id);
		if (selectedContactID === id) {
			selectedContactID = testContacts[0]?.id ?? '';
		}
		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(testContacts));
		} catch {}
	}

	$effect(() => {
		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(testContacts));
		} catch {}
	});

	function getPlatformInfo(key: string) {
		return PLATFORMS.find((p) => p.key === key) ?? PLATFORMS[0];
	}

	const activePlatform = $derived(getPlatformInfo(selectedPlatform));
</script>

<!-- ─── Floating Launcher Button ─────────────────────────────────────────── -->
{#if !isOpen}
	<button
		class="dev-launcher"
		onclick={() => (isOpen = true)}
		title="Open Dev Test Widget"
		aria-label="Open Dev Test Widget"
	>
		<span class="dev-launcher-icon">🧪</span>
		<span class="dev-launcher-label">Dev</span>
	</button>
{/if}

<!-- ─── Main Widget Panel ──────────────────────────────────────────────────── -->
{#if isOpen}
	<div class="dev-widget" class:minimized={isMinimized}>
		<!-- Header -->
		<div class="dev-header">
			<div class="dev-header-left">
				<span class="dev-header-icon">🧪</span>
				<span class="dev-header-title">Message Simulator</span>
				<span class="dev-badge">DEV</span>
			</div>
			<div class="dev-header-actions">
				<button
					class="dev-icon-btn"
					onclick={() => (isMinimized = !isMinimized)}
					title={isMinimized ? 'Expand' : 'Minimize'}
					aria-label={isMinimized ? 'Expand' : 'Minimize'}
				>
					{isMinimized ? '⬆' : '⬇'}
				</button>
				<button
					class="dev-icon-btn dev-close-btn"
					onclick={() => (isOpen = false)}
					title="Close"
					aria-label="Close widget"
				>
					✕
				</button>
			</div>
		</div>

		{#if !isMinimized}
			<div class="dev-body">
				<!-- Platform Picker -->
				<div class="dev-section">
					<div class="dev-label">Platform</div>
					<div class="platform-grid">
						{#each PLATFORMS as plat}
							<button
								class="platform-btn"
								class:active={selectedPlatform === plat.key}
								onclick={() => (selectedPlatform = plat.key)}
								style="--plat-color: {plat.color}"
								title={plat.label}
							>
								<span class="platform-icon">{plat.icon}</span>
								<span class="platform-name">{plat.label}</span>
							</button>
						{/each}
					</div>
				</div>

				<!-- Channel Selector -->
				<div class="dev-section">
					<div class="dev-label-row">
						<div class="dev-label">Target Channel</div>
						<button class="dev-refresh-btn" onclick={loadChannels} title="Refresh channels">↻</button>
					</div>
					{#if channels.length === 0}
						<div class="dev-empty-note">
							No channels found. Create one in
							<a href="/settings/account" class="dev-link">Settings → Channels</a>.
						</div>
					{:else}
						<select class="dev-select" bind:value={selectedChannelID}>
							{#each channels as ch}
								<option value={ch.id}>{ch.type} — {ch.id.slice(0, 8)}…</option>
							{/each}
						</select>
					{/if}
				</div>

				<!-- Test Contact Selector -->
				<div class="dev-section">
					<div class="dev-label-row">
						<div class="dev-label">Test Contact</div>
						<button
							class="dev-add-btn"
							onclick={() => (showAddContact = !showAddContact)}
							title="Add contact"
						>+ Add</button>
					</div>
					<div class="contact-list">
						{#each testContacts as contact}
							<button
								class="contact-chip"
								class:active={selectedContactID === contact.id}
								onclick={() => (selectedContactID = contact.id)}
							>
								<span class="contact-avatar">{contact.avatar}</span>
								<span class="contact-name">{contact.name}</span>
								{#if testContacts.length > 1}
									<span
										class="contact-remove"
										role="button"
										tabindex="0"
										onclick={(e) => { e.stopPropagation(); removeContact(contact.id); }}
										onkeydown={(e) => e.key === 'Enter' && removeContact(contact.id)}
									>✕</span>
								{/if}
							</button>
						{/each}
					</div>

					{#if showAddContact}
						<div class="add-contact-form">
							<input
								class="dev-input"
								type="text"
								placeholder="Contact name…"
								bind:value={newContactName}
								onkeydown={(e) => e.key === 'Enter' && addContact()}
							/>
							<button class="dev-btn-secondary" onclick={addContact}>Add</button>
						</div>
					{/if}
				</div>

				<!-- Content Type -->
				<div class="dev-section">
					<div class="dev-label">Message Type</div>
					<div class="type-tabs">
						{#each [{ k: 'text', l: '💬 Text' }, { k: 'image', l: '🖼 Image' }, { k: 'audio', l: '🎙 Audio' }] as t}
							<button
								class="type-tab"
								class:active={contentType === t.k}
								onclick={() => (contentType = t.k as 'text' | 'image' | 'audio')}
							>
								{t.l}
							</button>
						{/each}
					</div>
				</div>

				<!-- Preset Messages (text only) -->
				{#if contentType === 'text'}
					<div class="dev-section">
						<div class="dev-label">Quick Presets</div>
						<div class="presets-list">
							{#each getPresetMessages() as preset}
								<button
									class="preset-chip"
									onclick={() => (messageText = preset)}
								>{preset}</button>
							{/each}
						</div>
					</div>
				{/if}

				<!-- Message Compose -->
				<div class="dev-section">
					<div class="dev-label">{contentType === 'text' ? 'Message Text' : 'Caption (optional)'}</div>
					<textarea
						class="dev-textarea"
						placeholder={contentType === 'text'
							? 'Type a message from the test contact…'
							: 'Optional caption…'}
						bind:value={messageText}
						onkeydown={handleKeydown}
						rows="3"
					></textarea>

					{#if contentType !== 'text'}
						<input
							class="dev-input dev-input-mt"
							type="url"
							placeholder="Media URL (https://…)"
							bind:value={mediaURL}
						/>
					{/if}
				</div>

				<!-- Status Feedback -->
				{#if lastStatus === 'success'}
					<div class="dev-feedback dev-feedback--success">
						✓ Message injected via {activePlatform.icon} {activePlatform.label}
					</div>
				{:else if lastStatus === 'error'}
					<div class="dev-feedback dev-feedback--error">
						✗ {lastError}
					</div>
				{/if}

				<!-- Send Button -->
				<button
					class="dev-send-btn"
					style="--plat-color: {activePlatform.color}"
					disabled={isSending || (contentType === 'text' && !messageText.trim())}
					onclick={sendMessage}
				>
					{#if isSending}
						<span class="dev-spinner"></span> Sending…
					{:else}
						{activePlatform.icon} Send as {activePlatform.label}
					{/if}
				</button>
			</div>
		{/if}
	</div>
{/if}

<style>
	/* ─── Launcher ─────────────────────────────────────────────────────────── */
	.dev-launcher {
		position: fixed;
		bottom: 24px;
		right: 24px;
		z-index: 9000;
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 10px 16px;
		background: rgba(15, 15, 25, 0.92);
		border: 1px solid rgba(139, 92, 246, 0.4);
		border-radius: 12px;
		color: #c4b5fd;
		font-size: 13px;
		font-weight: 600;
		cursor: pointer;
		backdrop-filter: blur(12px);
		box-shadow: 0 4px 20px rgba(0, 0, 0, 0.4), 0 0 0 1px rgba(139, 92, 246, 0.15) inset;
		transition: all 0.2s;
	}
	.dev-launcher:hover {
		background: rgba(20, 15, 35, 0.96);
		border-color: rgba(139, 92, 246, 0.7);
		box-shadow: 0 6px 28px rgba(139, 92, 246, 0.25), 0 0 0 1px rgba(139, 92, 246, 0.2) inset;
		transform: translateY(-1px);
	}
	.dev-launcher-icon { font-size: 16px; }
	.dev-launcher-label { letter-spacing: 0.5px; }

	/* ─── Widget Panel ─────────────────────────────────────────────────────── */
	.dev-widget {
		position: fixed;
		bottom: 24px;
		right: 24px;
		z-index: 9001;
		width: 360px;
		max-height: 90vh;
		display: flex;
		flex-direction: column;
		background: rgba(12, 10, 22, 0.97);
		border: 1px solid rgba(139, 92, 246, 0.3);
		border-radius: 16px;
		backdrop-filter: blur(20px);
		box-shadow:
			0 24px 60px rgba(0, 0, 0, 0.6),
			0 0 0 1px rgba(139, 92, 246, 0.1) inset;
		overflow: hidden;
		animation: widget-in 0.22s cubic-bezier(0.16, 1, 0.3, 1);
	}
	.dev-widget.minimized {
		max-height: none;
	}

	@keyframes widget-in {
		from { opacity: 0; transform: translateY(16px) scale(0.97); }
		to   { opacity: 1; transform: translateY(0) scale(1); }
	}

	/* ─── Header ─────────────────────────────────────────────────────────── */
	.dev-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 12px 14px;
		border-bottom: 1px solid rgba(139, 92, 246, 0.15);
		background: rgba(139, 92, 246, 0.06);
		flex-shrink: 0;
	}
	.dev-header-left {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.dev-header-icon { font-size: 16px; }
	.dev-header-title {
		font-size: 13px;
		font-weight: 700;
		color: #e9d5ff;
		letter-spacing: 0.3px;
	}
	.dev-badge {
		font-size: 9px;
		font-weight: 800;
		letter-spacing: 1px;
		background: rgba(139, 92, 246, 0.25);
		color: #a78bfa;
		border: 1px solid rgba(139, 92, 246, 0.35);
		padding: 1px 5px;
		border-radius: 4px;
	}
	.dev-header-actions {
		display: flex;
		align-items: center;
		gap: 4px;
	}
	.dev-icon-btn {
		background: transparent;
		border: none;
		color: rgba(196, 181, 253, 0.5);
		font-size: 13px;
		cursor: pointer;
		padding: 4px 6px;
		border-radius: 6px;
		transition: all 0.15s;
	}
	.dev-icon-btn:hover {
		background: rgba(139, 92, 246, 0.12);
		color: #c4b5fd;
	}
	.dev-close-btn:hover {
		background: rgba(239, 68, 68, 0.12);
		color: #f87171;
	}

	/* ─── Body ─────────────────────────────────────────────────────────────── */
	.dev-body {
		flex: 1;
		overflow-y: auto;
		padding: 14px;
		display: flex;
		flex-direction: column;
		gap: 14px;
		scrollbar-width: thin;
		scrollbar-color: rgba(139, 92, 246, 0.2) transparent;
	}

	.dev-section {
		display: flex;
		flex-direction: column;
		gap: 7px;
	}
	.dev-label {
		font-size: 10px;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.8px;
		color: rgba(167, 139, 250, 0.7);
	}
	.dev-label-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.dev-refresh-btn, .dev-add-btn {
		background: transparent;
		border: 1px solid rgba(139, 92, 246, 0.25);
		color: #a78bfa;
		font-size: 11px;
		font-weight: 600;
		padding: 3px 8px;
		border-radius: 5px;
		cursor: pointer;
		transition: all 0.15s;
	}
	.dev-refresh-btn:hover, .dev-add-btn:hover {
		background: rgba(139, 92, 246, 0.12);
		border-color: rgba(139, 92, 246, 0.5);
	}

	/* ─── Platform Grid ────────────────────────────────────────────────────── */
	.platform-grid {
		display: grid;
		grid-template-columns: repeat(5, 1fr);
		gap: 5px;
	}
	.platform-btn {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 3px;
		padding: 7px 4px;
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid rgba(255, 255, 255, 0.06);
		border-radius: 8px;
		cursor: pointer;
		transition: all 0.15s;
	}
	.platform-btn:hover {
		background: rgba(255, 255, 255, 0.06);
		border-color: rgba(255, 255, 255, 0.12);
	}
	.platform-btn.active {
		background: color-mix(in srgb, var(--plat-color) 15%, transparent);
		border-color: color-mix(in srgb, var(--plat-color) 50%, transparent);
		box-shadow: 0 0 10px color-mix(in srgb, var(--plat-color) 20%, transparent);
	}
	.platform-icon { font-size: 16px; }
	.platform-name {
		font-size: 8px;
		color: rgba(255, 255, 255, 0.5);
		font-weight: 500;
		text-align: center;
		line-height: 1.2;
	}
	.platform-btn.active .platform-name {
		color: rgba(255, 255, 255, 0.85);
	}

	/* ─── Selects & Inputs ─────────────────────────────────────────────────── */
	.dev-select {
		background: rgba(255, 255, 255, 0.04);
		border: 1px solid rgba(139, 92, 246, 0.2);
		border-radius: 8px;
		color: #e9d5ff;
		font-size: 12px;
		padding: 7px 10px;
		width: 100%;
		outline: none;
		transition: border-color 0.15s;
		appearance: auto;
	}
	.dev-select:focus {
		border-color: rgba(139, 92, 246, 0.5);
	}
	.dev-input {
		background: rgba(255, 255, 255, 0.04);
		border: 1px solid rgba(139, 92, 246, 0.2);
		border-radius: 8px;
		color: #e9d5ff;
		font-size: 12px;
		padding: 7px 10px;
		width: 100%;
		outline: none;
		transition: border-color 0.15s;
	}
	.dev-input:focus {
		border-color: rgba(139, 92, 246, 0.5);
	}
	.dev-input-mt { margin-top: 6px; }
	.dev-textarea {
		background: rgba(255, 255, 255, 0.04);
		border: 1px solid rgba(139, 92, 246, 0.2);
		border-radius: 8px;
		color: #e9d5ff;
		font-size: 12px;
		padding: 8px 10px;
		width: 100%;
		outline: none;
		resize: none;
		font-family: inherit;
		line-height: 1.5;
		transition: border-color 0.15s;
	}
	.dev-textarea:focus {
		border-color: rgba(139, 92, 246, 0.5);
	}

	/* ─── Contact Chips ────────────────────────────────────────────────────── */
	.contact-list {
		display: flex;
		flex-wrap: wrap;
		gap: 5px;
	}
	.contact-chip {
		display: flex;
		align-items: center;
		gap: 5px;
		padding: 5px 9px;
		background: rgba(255, 255, 255, 0.04);
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: 20px;
		cursor: pointer;
		font-size: 12px;
		color: rgba(255, 255, 255, 0.65);
		transition: all 0.15s;
	}
	.contact-chip:hover {
		background: rgba(139, 92, 246, 0.1);
		border-color: rgba(139, 92, 246, 0.3);
		color: #c4b5fd;
	}
	.contact-chip.active {
		background: rgba(139, 92, 246, 0.18);
		border-color: rgba(139, 92, 246, 0.5);
		color: #e9d5ff;
	}
	.contact-avatar { font-size: 14px; }
	.contact-name { font-weight: 500; }
	.contact-remove {
		color: rgba(255, 255, 255, 0.3);
		font-size: 10px;
		margin-left: 2px;
		transition: color 0.15s;
		cursor: pointer;
	}
	.contact-chip:hover .contact-remove { color: #f87171; }

	.add-contact-form {
		display: flex;
		gap: 6px;
		margin-top: 4px;
	}
	.dev-btn-secondary {
		background: rgba(139, 92, 246, 0.12);
		border: 1px solid rgba(139, 92, 246, 0.3);
		border-radius: 7px;
		color: #a78bfa;
		font-size: 12px;
		font-weight: 600;
		padding: 6px 12px;
		cursor: pointer;
		white-space: nowrap;
		transition: all 0.15s;
		flex-shrink: 0;
	}
	.dev-btn-secondary:hover {
		background: rgba(139, 92, 246, 0.22);
	}

	/* ─── Type Tabs ────────────────────────────────────────────────────────── */
	.type-tabs {
		display: flex;
		gap: 5px;
	}
	.type-tab {
		flex: 1;
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid rgba(255, 255, 255, 0.07);
		border-radius: 7px;
		color: rgba(255, 255, 255, 0.45);
		font-size: 11px;
		font-weight: 500;
		padding: 6px;
		cursor: pointer;
		transition: all 0.15s;
	}
	.type-tab:hover {
		background: rgba(139, 92, 246, 0.08);
		color: rgba(255, 255, 255, 0.7);
	}
	.type-tab.active {
		background: rgba(139, 92, 246, 0.16);
		border-color: rgba(139, 92, 246, 0.4);
		color: #c4b5fd;
	}

	/* ─── Preset chips ─────────────────────────────────────────────────────── */
	.presets-list {
		display: flex;
		flex-wrap: wrap;
		gap: 5px;
	}
	.preset-chip {
		background: rgba(255, 255, 255, 0.04);
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: 6px;
		color: rgba(255, 255, 255, 0.55);
		font-size: 11px;
		padding: 4px 8px;
		cursor: pointer;
		text-align: left;
		line-height: 1.4;
		transition: all 0.15s;
	}
	.preset-chip:hover {
		background: rgba(139, 92, 246, 0.1);
		border-color: rgba(139, 92, 246, 0.3);
		color: #c4b5fd;
	}

	/* ─── Empty & Link ─────────────────────────────────────────────────────── */
	.dev-empty-note {
		font-size: 11px;
		color: rgba(255, 255, 255, 0.35);
		padding: 8px;
		background: rgba(255, 255, 255, 0.02);
		border-radius: 7px;
		border: 1px dashed rgba(255, 255, 255, 0.08);
		line-height: 1.5;
	}
	.dev-link {
		color: #a78bfa;
		text-decoration: underline;
	}

	/* ─── Feedback ─────────────────────────────────────────────────────────── */
	.dev-feedback {
		font-size: 11px;
		font-weight: 500;
		padding: 8px 10px;
		border-radius: 8px;
		animation: feedback-in 0.2s ease;
	}
	@keyframes feedback-in {
		from { opacity: 0; transform: translateY(4px); }
		to { opacity: 1; transform: translateY(0); }
	}
	.dev-feedback--success {
		background: rgba(34, 197, 94, 0.1);
		border: 1px solid rgba(34, 197, 94, 0.25);
		color: #86efac;
	}
	.dev-feedback--error {
		background: rgba(239, 68, 68, 0.1);
		border: 1px solid rgba(239, 68, 68, 0.25);
		color: #fca5a5;
	}

	/* ─── Send Button ──────────────────────────────────────────────────────── */
	.dev-send-btn {
		width: 100%;
		padding: 11px;
		background: color-mix(in srgb, var(--plat-color) 25%, rgba(0, 0, 0, 0.2));
		border: 1px solid color-mix(in srgb, var(--plat-color) 50%, transparent);
		border-radius: 10px;
		color: #fff;
		font-size: 13px;
		font-weight: 700;
		cursor: pointer;
		transition: all 0.2s;
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 8px;
	}
	.dev-send-btn:hover:not(:disabled) {
		background: color-mix(in srgb, var(--plat-color) 40%, rgba(0, 0, 0, 0.2));
		box-shadow: 0 4px 18px color-mix(in srgb, var(--plat-color) 30%, transparent);
		transform: translateY(-1px);
	}
	.dev-send-btn:disabled {
		opacity: 0.45;
		cursor: not-allowed;
		transform: none;
	}

	/* ─── Spinner ──────────────────────────────────────────────────────────── */
	.dev-spinner {
		width: 12px;
		height: 12px;
		border: 2px solid rgba(255, 255, 255, 0.3);
		border-top-color: #fff;
		border-radius: 50%;
		animation: spin 0.6s linear infinite;
		flex-shrink: 0;
	}
	@keyframes spin {
		to { transform: rotate(360deg); }
	}
</style>
