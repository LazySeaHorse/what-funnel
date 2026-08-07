<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
	import { InboxState } from '$lib/store.svelte';
	import DevTestWidget from '$lib/DevTestWidget.svelte';

	const inbox = new InboxState();
	let composeText = $state('');
	let messageContainer: HTMLDivElement | null = $state(null);
	let isAssignDropdownOpen = $state(false);

	// Lead Management frontend states
	let leadTrackingEnabled = $state(true);
	let productMode = $state('full_workspace');
	let pipelineStates = $state<any[]>([]);
	let activePanelTab = $state<'notes' | 'history'>('notes');
	let notes = $state<any[]>([]);
	let history = $state<any[]>([]);
	let loadingNotes = $state(false);
	let newNoteText = $state('');
	let tagInput = $state('');

	// Finish-setup banner
	const SKIPPED_STEP_NAMES: Record<string, { label: string; step: number }> = {
		channel_connect: { label: 'Connect a channel', step: 4 },
		kb_setup:        { label: 'Set up your knowledge base', step: 5 },
		reply_mode:      { label: 'Configure reply mode', step: 6 },
		pipeline_setup:  { label: 'Review your pipeline', step: 7 },
		team_invite:     { label: 'Invite your team', step: 8 }
	};
	let showSetupBanner = $state(false);
	let bannerSkippedSteps = $state<Array<{ label: string; step: number }>>([]);

	onMount(async () => {
		try {
			await inbox.init();
			if (!inbox.currentUser) {
				goto('/login');
				return;
			}
			
			// Fetch account details to check lead_tracking_enabled
			const account = await apiRequest('/workspace/account');
			if (account) {
				productMode = account.product_mode || 'full_workspace';
				if (account.settings) {
					try {
						const decoded = atob(account.settings);
						const parsed = JSON.parse(decoded);
						leadTrackingEnabled = parsed.lead_tracking_enabled !== false;
					} catch (e) {
						console.error('Failed to parse account settings', e);
					}
				}
				if (productMode === 'chatbot_only') {
					leadTrackingEnabled = false;
				}
			}
			
			// Fetch pipeline states
			const pipelines = await apiRequest('/workspace/pipelines');
			if (pipelines && pipelines.length > 0) {
				pipelineStates = pipelines[0].states || [];
			}

			// Finish-setup banner: check onboarding status
			try {
				const dismissed = sessionStorage.getItem('setup-banner-dismissed');
				if (!dismissed) {
					const onboarding = await apiRequest('/onboarding/status');
					if (onboarding?.completed_at && onboarding?.skipped_steps?.length > 0) {
						const skipped = (onboarding.skipped_steps as string[])
							.filter((k: string) => k in SKIPPED_STEP_NAMES)
							.map((k: string) => SKIPPED_STEP_NAMES[k]);
						if (skipped.length > 0) {
							bannerSkippedSteps = skipped;
							showSetupBanner = true;
						}
					}
				}
			} catch (_) {}
			
			// Listen to live lead state changes from WebSocket & dev widget simulated sends
			const handleLeadStateChange = (e: CustomEvent) => {
				if (inbox.activeConvo?.lead && e.detail.lead_id === inbox.activeConvo.lead.id) {
					loadLeadDetails(inbox.activeConvo.lead.id);
				}
			};
			const handleDevMessageSent = () => {
				inbox.loadConversations();
			};
			window.addEventListener('lead-state-changed', handleLeadStateChange as EventListener);
			window.addEventListener('dev-message-sent', handleDevMessageSent);
			return () => {
				window.removeEventListener('lead-state-changed', handleLeadStateChange as EventListener);
				window.removeEventListener('dev-message-sent', handleDevMessageSent);
			};
		} catch (err) {
			goto('/login');
		}
	});

	function parseMessageContent(content: any): Record<string, any> {
		if (!content) return {};
		if (typeof content === 'object') return content;
		if (typeof content === 'string') {
			try {
				return JSON.parse(content);
			} catch (e1) {
				try {
					const decoded = atob(content);
					return JSON.parse(decoded);
				} catch (e2) {
					return { text: content };
				}
			}
		}
		return {};
	}

	// Derived list of displayable messages (attaches reactions, hides reaction bubbles)
	let displayMessages = $derived.by(() => {
		const msgs = inbox.messages;
		const reactionsMap: Record<string, string[]> = {};

		// 1. Gather all reactions
		for (const m of msgs) {
			if (m.content_type === 'reaction') {
				try {
					const contentObj = parseMessageContent(m.content);
					const reaction = contentObj.text || contentObj.reaction;
					const targetExtID = contentObj.reply_to_external_id;
					if (targetExtID && reaction) {
						if (!reactionsMap[targetExtID]) {
							reactionsMap[targetExtID] = [];
						}
						reactionsMap[targetExtID].push(reaction);
					}
				} catch (e) {}
			}
		}

		// 2. Map messages and attach reactions
		return msgs
			.filter((m: any) => m.content_type !== 'reaction')
			.map((m: any) => {
				const contentObj = parseMessageContent(m.content);
				return {
					...m,
					parsedContent: contentObj,
					reactions: m.external_message_id ? reactionsMap[m.external_message_id] || [] : []
				};
			});
	});

	async function selectConvo(id: string) {
		await inbox.selectConversation(id);
		await tick();
		scrollToBottom();
	}

	async function handleSend(e?: Event) {
		if (e) e.preventDefault();
		if (!composeText.trim() || !inbox.activeConvoID) return;

		const textToSend = composeText;
		composeText = '';
		await inbox.sendMessage(textToSend);
		await tick();
		scrollToBottom();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			handleSend();
		}
	}

	async function handleLoadMore() {
		if (!inbox.nextCursor) return;
		const oldHeight = messageContainer?.scrollHeight || 0;
		await inbox.loadMessages(false);
		await tick();
		if (messageContainer) {
			messageContainer.scrollTop = messageContainer.scrollHeight - oldHeight;
		}
	}

	function scrollToBottom() {
		if (messageContainer) {
			messageContainer.scrollTop = messageContainer.scrollHeight;
		}
	}

	async function handleLogout() {
		try {
			await apiRequest('/auth/logout', { method: 'POST' });
			goto('/login');
		} catch (err) {
			console.error(err);
		}
	}

	function formatTime(timeStr?: string) {
		if (!timeStr) return '';
		const d = new Date(timeStr);
		return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	}

	function getInitial(name?: string) {
		return name ? name.charAt(0).toUpperCase() : '?';
	}

	function toggleUserAssignment(userID: string) {
		if (!inbox.activeConvo) return;
		const current = inbox.activeConvo.assigned_user_ids || [];
		let updated: string[];
		if (current.includes(userID)) {
			updated = current.filter((id: string) => id !== userID);
		} else {
			updated = [...current, userID];
		}
		inbox.assignConversation(updated);
	}

	function getAssignedNames(assignedIDs?: string[]) {
		if (!assignedIDs || assignedIDs.length === 0) return 'Unassigned';
		return inbox.users
			.filter((u: any) => assignedIDs.includes(u.id))
			.map((u: any) => u.email.split('@')[0])
			.join(', ');
	}

	async function loadLeadDetails(leadId: string) {
		loadingNotes = true;
		try {
			notes = await apiRequest(`/leads/${leadId}/notes`);
			history = await apiRequest(`/leads/${leadId}/history`);
		} catch (err) {
			console.error('Failed to load lead details', err);
		} finally {
			loadingNotes = false;
		}
	}

	$effect(() => {
		const lead = inbox.activeConvo?.lead;
		if (lead && lead.id) {
			loadLeadDetails(lead.id);
		} else {
			notes = [];
			history = [];
		}
	});

	async function createLead() {
		if (!inbox.activeConvoID) return;
		try {
			const lead = await apiRequest(`/conversations/${inbox.activeConvoID}/lead`, {
				method: 'POST'
			});
			if (inbox.activeConvo) {
				inbox.activeConvo.lead = lead;
			}
			await inbox.loadConversations();
		} catch (err) {
			console.error(err);
		}
	}

	async function updateLeadState(stateKey: string) {
		if (!inbox.activeConvo?.lead) return;
		try {
			const lead = await apiRequest(`/leads/${inbox.activeConvo.lead.id}/state`, {
				method: 'PATCH',
				body: { state_key: stateKey }
			});
			inbox.activeConvo.lead.current_state_key = lead.current_state_key;
			await inbox.loadConversations();
		} catch (err) {
			console.error(err);
		}
	}

	async function addTag(e: Event) {
		e.preventDefault();
		if (!tagInput.trim() || !inbox.activeConvo?.lead) return;
		const tag = tagInput.trim();
		const currentTags = inbox.activeConvo.lead.tags || [];
		if (currentTags.includes(tag)) {
			tagInput = '';
			return;
		}
		const newTags = [...currentTags, tag];
		try {
			const lead = await apiRequest(`/leads/${inbox.activeConvo.lead.id}/tags`, {
				method: 'PATCH',
				body: { tags: newTags }
			});
			inbox.activeConvo.lead.tags = lead.tags;
			tagInput = '';
		} catch (err) {
			console.error(err);
		}
	}

	async function removeTag(tag: string) {
		if (!inbox.activeConvo?.lead) return;
		const currentTags = inbox.activeConvo.lead.tags || [];
		const newTags = currentTags.filter((t: string) => t !== tag);
		try {
			const lead = await apiRequest(`/leads/${inbox.activeConvo.lead.id}/tags`, {
				method: 'PATCH',
				body: { tags: newTags }
			});
			inbox.activeConvo.lead.tags = lead.tags;
		} catch (err) {
			console.error(err);
		}
	}

	async function addNote(e: Event) {
		e.preventDefault();
		if (!newNoteText.trim() || !inbox.activeConvo?.lead) return;
		const body = newNoteText.trim();
		try {
			await apiRequest(`/leads/${inbox.activeConvo.lead.id}/notes`, {
				method: 'POST',
				body: { body }
			});
			newNoteText = '';
			await loadLeadDetails(inbox.activeConvo.lead.id);
		} catch (err) {
			console.error(err);
		}
	}

	function getStateColor(stateKey: string) {
		const st = pipelineStates.find(s => s.key === stateKey);
		return st ? st.color : '#6366f1';
	}

	function getStateLabel(stateKey: string) {
		const st = pipelineStates.find(s => s.key === stateKey);
		return st ? st.label : stateKey;
	}

	function formatDate(dateStr?: string) {
		if (!dateStr) return '';
		const d = new Date(dateStr);
		return d.toLocaleDateString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
	}
</script>

{#if showSetupBanner}
	<div class="setup-banner" role="alert">
		<div class="setup-banner-inner">
			<span class="setup-banner-icon">⚡</span>
			<div class="setup-banner-content">
				<strong>Finish setting up your workspace</strong>
				<div class="setup-banner-links">
					{#each bannerSkippedSteps as item, i}
						<a href="/onboarding/{item.step}" class="setup-banner-link">{item.label}</a>
						{#if i < bannerSkippedSteps.length - 1}<span class="sep">·</span>{/if}
					{/each}
				</div>
			</div>
		</div>
		<button
			class="setup-banner-dismiss"
			onclick={() => { showSetupBanner = false; sessionStorage.setItem('setup-banner-dismissed', '1'); }}
			aria-label="Dismiss banner"
		>✕</button>
	</div>
{/if}

<div class="inbox-layout" class:has-lead-panel={leadTrackingEnabled && inbox.activeConvo}>
	<!-- Left Navigation & Conversations Pane -->
	<div class="sidebar glass-panel">
		<!-- Header / Profile Section -->
		<div class="profile-header">
			<div class="logo-area">
				<span class="logo-dot"></span>
				<h2 class="logo-text">What Funnel</h2>
			</div>
			{#if inbox.currentUser}
				<div class="user-meta">
					<div class="user-email">{inbox.currentUser.email}</div>
					<div class="user-badge">{inbox.currentUser.role}</div>
				</div>
			{/if}
			
			<div class="nav-links">
				{#if inbox.currentUser?.role === 'admin'}
					<a href="/settings/account" class="nav-btn">Settings</a>
				{/if}
				<button onclick={handleLogout} class="nav-btn logout-btn">Logout</button>
			</div>
		</div>

		<!-- Filter Tabs -->
		<div class="filter-tabs">
			{#if inbox.currentUser?.role === 'admin'}
				<button
					class="tab-btn"
					class:active={inbox.filter === 'all'}
					onclick={() => {
						inbox.filter = 'all';
						inbox.loadConversations();
					}}
				>
					All
				</button>
			{/if}
			<button
				class="tab-btn"
				class:active={inbox.filter === 'mine'}
				onclick={() => {
					inbox.filter = 'mine';
					inbox.loadConversations();
				}}
			>
				Mine
			</button>
			<button
				class="tab-btn"
				class:active={inbox.filter === 'unassigned'}
				onclick={() => {
					inbox.filter = 'unassigned';
					inbox.loadConversations();
				}}
			>
				Unassigned
			</button>
		</div>

		{#if leadTrackingEnabled && pipelineStates.length > 0}
			<div class="state-filter-container">
				<select 
					class="input-field state-filter-select"
					bind:value={inbox.stateFilter}
					onchange={() => inbox.loadConversations()}
				>
					<option value="">All Lead Stages</option>
					{#each pipelineStates as st}
						<option value={st.key}>{st.label}</option>
					{/each}
				</select>
			</div>
		{/if}

		<!-- Conversation List -->
		<div class="conversation-list">
			{#each inbox.conversations as convo}
				<button
					class="convo-item"
					class:active={inbox.activeConvoID === convo.id}
					onclick={() => selectConvo(convo.id)}
				>
					<div class="convo-avatar">
						{getInitial(convo.contact_name)}
						{#if convo.unread}
							<span class="unread-badge"></span>
						{/if}
					</div>
					<div class="convo-info">
						<div class="convo-top">
							<span class="convo-name">{convo.contact_name || 'Unknown Contact'}</span>
							{#if leadTrackingEnabled && convo.lead}
								<span 
									class="lead-state-badge" 
									style="border: 1px solid {getStateColor(convo.lead.current_state_key)}; color: {getStateColor(convo.lead.current_state_key)}"
								>
									{getStateLabel(convo.lead.current_state_key)}
								</span>
							{/if}
							<span class="convo-time">{formatTime(convo.last_message_at)}</span>
						</div>
						<div class="convo-preview">
							{#if convo.last_message_preview}
								{convo.last_message_preview.content_type === 'text' 
									? (parseMessageContent(convo.last_message_preview.content).text || '')
									: `[${convo.last_message_preview.content_type}]`}
							{:else}
								No messages yet
							{/if}
						</div>
					</div>
				</button>
			{:else}
				<div class="empty-list">No conversations found</div>
			{/each}
		</div>
	</div>

	<!-- Right Thread Pane -->
	<div class="thread-pane glass-panel">
		{#if inbox.activeConvo}
			<!-- Thread Header -->
			<div class="thread-header">
				<div class="thread-contact-info">
					<div class="convo-avatar">{getInitial(inbox.activeConvo.contact_name)}</div>
					<div>
						<h3 class="contact-title">{inbox.activeConvo.contact_name || 'Unknown Contact'}</h3>
						<div class="contact-subtitle">
							Assigned: {getAssignedNames(inbox.activeConvo.assigned_user_ids)}
						</div>
					</div>
				</div>

				<!-- Assignment Multi-select (Admin only) -->
				{#if inbox.currentUser?.role === 'admin'}
					<div class="assignment-control">
						<button 
							class="btn-secondary" 
							onclick={() => isAssignDropdownOpen = !isAssignDropdownOpen}
						>
							Assign Agent ⚙️
						</button>
						{#if isAssignDropdownOpen}
							<div class="assign-dropdown glass-panel">
								<h4>Assign to:</h4>
								{#each inbox.users as user}
									<label class="assign-user-row">
										<input 
											type="checkbox" 
											checked={inbox.activeConvo.assigned_user_ids?.includes(user.id)}
											onchange={() => toggleUserAssignment(user.id)}
										/>
										<span>{user.email}</span>
									</label>
								{/each}
							</div>
						{/if}
					</div>
				{/if}
			</div>

			<!-- Message Stream -->
			<div class="message-stream" bind:this={messageContainer}>
				{#if inbox.nextCursor}
					<div class="load-more-container">
						<button class="btn-secondary load-more-btn" onclick={handleLoadMore}>
							Load Older Messages
						</button>
					</div>
				{/if}

				{#each displayMessages as msg}
					<div 
						class="message-row" 
						class:outbound={msg.direction === 'outbound'}
						class:ai={msg.sender_type === 'ai'}
					>
						<div class="message-bubble">
							{#if msg.sender_type === 'ai'}
								<span class="ai-badge">AI Assistant</span>
							{/if}
							
							<!-- Content Renderers -->
							{#if msg.content_type === 'text'}
								<p class="msg-text">{msg.parsedContent.text}</p>
							{:else if msg.content_type === 'image'}
								<a href={msg.parsedContent.media_url} target="_blank" rel="noreferrer">
									<img src={msg.parsedContent.media_url} alt="Uploaded Media" class="msg-media image" />
								</a>
								{#if msg.parsedContent.text}
									<p class="msg-caption">{msg.parsedContent.text}</p>
								{/if}
							{:else if msg.content_type === 'video'}
								<video src={msg.parsedContent.media_url} controls class="msg-media video">
									<track kind="captions" />
								</video>
								{#if msg.parsedContent.text}
									<p class="msg-caption">{msg.parsedContent.text}</p>
								{/if}
							{:else if msg.content_type === 'audio'}
								<audio src={msg.parsedContent.media_url} controls class="msg-media audio"></audio>
							{:else if msg.content_type === 'document'}
								<div class="msg-doc">
									<span>📄 Document</span>
									<a href={msg.parsedContent.media_url} download class="doc-link">Download File</a>
								</div>
							{:else if msg.content_type === 'location'}
								<p class="msg-text">📍 Location: {msg.parsedContent.text || `${msg.parsedContent.latitude}, ${msg.parsedContent.longitude}`}</p>
							{:else if msg.content_type === 'contact'}
								<div class="msg-contact-card">
									<span>👤 Contact Card</span>
									<strong>{msg.parsedContent.text || 'Unnamed'}</strong>
								</div>
							{/if}

							<!-- Inline Reactions -->
							{#if msg.reactions && msg.reactions.length > 0}
								<div class="reactions-list">
									{#each msg.reactions as rx}
										<span class="reaction-emoji">{rx}</span>
									{/each}
								</div>
							{/if}
							<span class="message-time">
								{#if msg.parsedContent.external_origin}
									<span class="external-origin-indicator">Sent from phone • </span>
								{/if}
								{formatTime(msg.created_at)}
							</span>
						</div>
					</div>
				{/each}
			</div>

			<!-- Compose Area -->
			<form class="compose-area" onsubmit={handleSend}>
				<textarea
					class="input-field compose-input"
					placeholder="Type your reply..."
					bind:value={composeText}
					onkeydown={handleKeydown}
					rows="1"
				></textarea>
				<button type="submit" class="btn-primary send-btn" disabled={!composeText.trim()}>
					Send
				</button>
			</form>
		{:else}
			<div class="thread-empty-state">
				<h2>No Conversation Selected</h2>
				<p>Choose a conversation from the left sidebar to start messaging.</p>
			</div>
		{/if}
	</div>

	{#if leadTrackingEnabled && inbox.activeConvo}
		<div class="lead-panel glass-panel">
			<h3 class="lead-panel-title">Lead Profile</h3>
			
			{#if inbox.activeConvo.lead}
				<!-- State Selector -->
				<div class="panel-section">
					<label class="section-label">Pipeline Stage</label>
					<select 
						class="input-field state-select" 
						value={inbox.activeConvo.lead.current_state_key}
						onchange={(e) => updateLeadState((e.target as HTMLSelectElement).value)}
					>
						{#each pipelineStates as st}
							<option value={st.key}>{st.label}</option>
						{/each}
					</select>
				</div>
				
				<!-- Tag Editor -->
				<div class="panel-section">
					<label class="section-label">Tags</label>
					<div class="tags-container">
						{#each inbox.activeConvo.lead.tags || [] as tag}
							<span class="lead-tag">
								{tag}
								<button class="remove-tag-btn" onclick={() => removeTag(tag)}>&times;</button>
							</span>
						{:else}
							<span class="no-tags-placeholder">No tags assigned</span>
						{/each}
					</div>
					<form onsubmit={addTag} class="tag-input-form">
						<input 
							type="text" 
							class="input-field tag-input" 
							placeholder="Add tag..."
							bind:value={tagInput}
						/>
						<button type="submit" class="btn-secondary add-tag-btn">+</button>
					</form>
				</div>

				<!-- Tabs for Notes & History -->
				<div class="panel-tabs">
					<button 
						class="panel-tab-btn" 
						class:active={activePanelTab === 'notes'} 
						onclick={() => activePanelTab = 'notes'}
					>
						Notes ({notes.length})
					</button>
					<button 
						class="panel-tab-btn" 
						class:active={activePanelTab === 'history'} 
						onclick={() => activePanelTab = 'history'}
					>
						History
					</button>
				</div>

				<div class="tab-content-container">
					{#if activePanelTab === 'notes'}
						<!-- Notes timeline -->
						<div class="notes-timeline">
							{#each notes as note}
								<div class="note-card glass-panel">
									<div class="note-header">
										<span class="note-author">{note.author_email.split('@')[0]}</span>
										<span class="note-time">{formatDate(note.created_at)}</span>
									</div>
									<p class="note-body">{note.body}</p>
								</div>
							{:else}
								<div class="empty-timeline-state">No notes yet. Add one below.</div>
							{/each}
						</div>
						<form onsubmit={addNote} class="add-note-form">
							<textarea 
								class="input-field note-textarea" 
								placeholder="Write a note..." 
								bind:value={newNoteText}
								required
							></textarea>
							<button type="submit" class="btn-primary add-note-btn">Save Note</button>
						</form>
					{:else}
						<!-- History timeline -->
						<div class="history-timeline">
							{#each history as hist}
								<div class="history-card">
									<div class="history-circle" style="background-color: {getStateColor(hist.to_state)}"></div>
									<div class="history-content">
										<div class="history-transition">
											{#if hist.from_state}
												<span class="history-state">{getStateLabel(hist.from_state)}</span> 
												➔ 
											{/if}
											<span class="history-state active-state">{getStateLabel(hist.to_state)}</span>
										</div>
										<div class="history-meta">
											By {hist.actor_email.split('@')[0]} • {formatDate(hist.created_at)}
										</div>
									</div>
								</div>
							{:else}
								<div class="empty-timeline-state">No history recorded yet.</div>
							{/each}
						</div>
					{/if}
				</div>

			{:else}
				<div class="no-lead-state">
					<p class="no-lead-text">This conversation is not currently being tracked as a sales lead.</p>
					<button class="btn-primary start-lead-btn" onclick={createLead}>
						Start Tracking Lead
					</button>
				</div>
			{/if}
		</div>
	{/if}
</div>

<!-- Dev Test Widget (admin only) -->
{#if inbox.currentUser?.role === 'admin'}
	<DevTestWidget />
{/if}

<style>
	/* ─── Setup Banner ─── */
	.setup-banner {
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		z-index: 200;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		padding: 10px 20px;
		background: rgba(245, 158, 11, 0.12);
		border-bottom: 1px solid rgba(245, 158, 11, 0.25);
		backdrop-filter: blur(8px);
		-webkit-backdrop-filter: blur(8px);
		animation: slide-down 0.3s ease both;
	}
	@keyframes slide-down {
		from { transform: translateY(-100%); opacity: 0; }
		to   { transform: translateY(0); opacity: 1; }
	}
	.setup-banner-inner {
		display: flex;
		align-items: center;
		gap: 10px;
		flex: 1;
		min-width: 0;
	}
	.setup-banner-icon {
		font-size: 16px;
		flex-shrink: 0;
	}
	.setup-banner-content {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.setup-banner-content strong {
		font-size: 13px;
		font-weight: 600;
		color: #fde68a;
	}
	.setup-banner-links {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 6px;
	}
	.setup-banner-link {
		font-size: 12px;
		color: var(--warning);
		text-decoration: underline;
		transition: opacity 0.15s;
	}
	.setup-banner-link:hover { opacity: 0.75; }
	.sep {
		color: var(--text-muted);
		font-size: 12px;
	}
	.setup-banner-dismiss {
		background: none;
		border: none;
		color: var(--text-muted);
		font-size: 16px;
		cursor: pointer;
		padding: 4px 8px;
		border-radius: 4px;
		transition: color 0.15s, background 0.15s;
		flex-shrink: 0;
	}
	.setup-banner-dismiss:hover {
		color: var(--text-primary);
		background: rgba(255,255,255,0.06);
	}

	.inbox-layout {
		display: grid;
		grid-template-columns: 320px 1fr;
		height: 100vh;
		background-color: var(--bg-dark);
		padding: 16px;
		gap: 16px;
	}

	.sidebar {
		display: flex;
		flex-direction: column;
		height: 100%;
		overflow: hidden;
	}

	.profile-header {
		padding: 16px;
		border-bottom: 1px solid var(--border-color);
	}

	.logo-area {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 12px;
	}

	.logo-dot {
		width: 10px;
		height: 10px;
		border-radius: 50%;
		background: var(--accent-gradient);
	}

	.logo-text {
		font-size: 18px;
		font-weight: 700;
		background: var(--accent-gradient);
		-webkit-background-clip: text;
		-webkit-text-fill-color: transparent;
	}

	.user-meta {
		margin-bottom: 12px;
	}

	.user-email {
		font-size: 13px;
		color: var(--text-secondary);
	}

	.user-badge {
		display: inline-block;
		font-size: 11px;
		font-weight: 600;
		text-transform: uppercase;
		background: rgba(99, 102, 241, 0.15);
		color: #818cf8;
		padding: 2px 6px;
		border-radius: 4px;
		margin-top: 4px;
	}

	.nav-links {
		display: flex;
		gap: 8px;
		margin-top: 8px;
	}

	.nav-btn {
		font-size: 12px;
		color: var(--text-secondary);
		text-decoration: none;
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid var(--border-color);
		padding: 6px 12px;
		border-radius: 6px;
		cursor: pointer;
		transition: background-color 0.2s;
	}

	.nav-btn:hover {
		background: rgba(255, 255, 255, 0.08);
		color: var(--text-primary);
	}

	.logout-btn {
		margin-left: auto;
		border-color: rgba(239, 68, 68, 0.2);
		color: #f87171;
	}

	.logout-btn:hover {
		background: rgba(239, 68, 68, 0.1);
		color: #f87171;
	}

	.filter-tabs {
		display: flex;
		padding: 8px 16px;
		gap: 6px;
		border-bottom: 1px solid var(--border-color);
	}

	.tab-btn {
		flex: 1;
		background: transparent;
		border: none;
		color: var(--text-secondary);
		padding: 8px;
		font-size: 13px;
		font-weight: 500;
		cursor: pointer;
		border-radius: 6px;
		transition: background-color 0.2s, color 0.2s;
	}

	.tab-btn:hover {
		background: rgba(255, 255, 255, 0.03);
		color: var(--text-primary);
	}

	.tab-btn.active {
		background: rgba(99, 102, 241, 0.15);
		color: #818cf8;
	}

	.conversation-list {
		flex: 1;
		overflow-y: auto;
		padding: 8px;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.convo-item {
		display: flex;
		align-items: center;
		padding: 12px;
		background: transparent;
		border: none;
		border-radius: 8px;
		cursor: pointer;
		width: 100%;
		text-align: left;
		gap: 12px;
		transition: background-color 0.2s;
	}

	.convo-item:hover {
		background: rgba(255, 255, 255, 0.03);
	}

	.convo-item.active {
		background: rgba(255, 255, 255, 0.05);
		border: 1px solid rgba(255, 255, 255, 0.05);
	}

	.convo-avatar {
		width: 40px;
		height: 40px;
		background: linear-gradient(135deg, #1f2937, #374151);
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 15px;
		font-weight: 600;
		color: var(--text-primary);
		position: relative;
		flex-shrink: 0;
	}

	.unread-badge {
		position: absolute;
		top: 0;
		right: 0;
		width: 10px;
		height: 10px;
		border-radius: 50%;
		background-color: #3b82f6;
		border: 2px solid var(--bg-dark);
	}

	.convo-info {
		flex: 1;
		min-width: 0;
	}

	.convo-top {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		margin-bottom: 4px;
	}

	.convo-name {
		font-size: 14px;
		font-weight: 500;
		color: var(--text-primary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.convo-time {
		font-size: 11px;
		color: var(--text-muted);
	}

	.convo-preview {
		font-size: 13px;
		color: var(--text-secondary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.empty-list {
		text-align: center;
		padding: 24px;
		color: var(--text-muted);
		font-size: 13px;
	}

	.thread-pane {
		display: flex;
		flex-direction: column;
		height: 100%;
		overflow: hidden;
	}

	.thread-header {
		padding: 16px 24px;
		border-bottom: 1px solid var(--border-color);
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.thread-contact-info {
		display: flex;
		align-items: center;
		gap: 12px;
	}

	.contact-title {
		font-size: 16px;
		font-weight: 600;
	}

	.contact-subtitle {
		font-size: 12px;
		color: var(--text-secondary);
	}

	.assignment-control {
		position: relative;
	}

	.assign-dropdown {
		position: absolute;
		top: 100%;
		right: 0;
		margin-top: 8px;
		width: 220px;
		padding: 12px;
		z-index: 10;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.assign-dropdown h4 {
		font-size: 12px;
		color: var(--text-secondary);
		text-transform: uppercase;
		margin-bottom: 4px;
	}

	.assign-user-row {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 13px;
		cursor: pointer;
	}

	.message-stream {
		flex: 1;
		overflow-y: auto;
		padding: 24px;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.load-more-container {
		display: flex;
		justify-content: center;
		margin-bottom: 16px;
	}

	.load-more-btn {
		font-size: 12px;
		padding: 6px 12px;
	}

	.message-row {
		display: flex;
		justify-content: flex-start;
	}

	.message-row.outbound {
		justify-content: flex-end;
	}

	.message-bubble {
		max-width: 60%;
		padding: 12px 16px;
		border-radius: 12px;
		background: rgba(255, 255, 255, 0.04);
		border: 1px solid var(--border-color);
		position: relative;
	}

	.message-row.outbound .message-bubble {
		background: rgba(99, 102, 241, 0.08);
		border-color: rgba(99, 102, 241, 0.2);
	}

	.message-row.ai .message-bubble {
		background: rgba(168, 85, 247, 0.08);
		border-color: rgba(168, 85, 247, 0.2);
	}

	.ai-badge {
		display: inline-block;
		font-size: 10px;
		font-weight: 600;
		color: #c084fc;
		background: rgba(168, 85, 247, 0.15);
		padding: 2px 6px;
		border-radius: 4px;
		margin-bottom: 4px;
		text-transform: uppercase;
	}

	.msg-text {
		font-size: 14px;
		line-height: 1.5;
	}

	.msg-media {
		max-width: 100%;
		max-height: 250px;
		border-radius: 8px;
		margin-top: 4px;
	}

	.msg-caption {
		font-size: 13px;
		color: var(--text-secondary);
		margin-top: 4px;
	}

	.msg-doc {
		display: flex;
		flex-direction: column;
		gap: 6px;
		font-size: 13px;
	}

	.doc-link {
		color: #6366f1;
		text-decoration: none;
		font-weight: 500;
	}

	.msg-contact-card {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: 13px;
	}

	.reactions-list {
		display: flex;
		gap: 4px;
		margin-top: 6px;
		flex-wrap: wrap;
	}

	.reaction-emoji {
		background: rgba(255, 255, 255, 0.05);
		border: 1px solid var(--border-color);
		padding: 2px 6px;
		border-radius: 10px;
		font-size: 12px;
	}

	.message-time {
		display: block;
		font-size: 10px;
		color: var(--text-muted);
		margin-top: 6px;
		text-align: right;
	}

	.external-origin-indicator {
		font-style: italic;
		font-weight: 500;
		color: var(--primary-color, #6366f1);
		opacity: 0.95;
	}

	.compose-area {
		padding: 16px 24px;
		border-top: 1px solid var(--border-color);
		display: flex;
		gap: 12px;
	}

	.compose-input {
		flex: 1;
		resize: none;
		height: 42px;
		line-height: 20px;
	}

	.send-btn {
		height: 42px;
	}

	.thread-empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		height: 100%;
		color: var(--text-muted);
		gap: 8px;
	}

	.inbox-layout.has-lead-panel {
		grid-template-columns: 320px 1fr 340px;
	}

	/* State Filter Styling */
	.state-filter-container {
		padding: 8px 16px;
		border-bottom: 1px solid var(--border-color);
	}
	
	.state-filter-select {
		height: 34px;
		font-size: 13px;
		padding: 4px 10px;
		background: rgba(255, 255, 255, 0.02);
		border-radius: 6px;
	}

	/* Lead State Badge in Conversation List */
	.lead-state-badge {
		font-size: 10px;
		font-weight: 600;
		padding: 2px 6px;
		border-radius: 4px;
		text-transform: uppercase;
		letter-spacing: 0.5px;
		margin-left: 8px;
		display: inline-block;
		white-space: nowrap;
	}

	/* Lead Panel Sidebar Styling */
	.lead-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
		overflow: hidden;
		padding: 20px;
		animation: slideInRight 0.2s cubic-bezier(0.16, 1, 0.3, 1);
	}

	@keyframes slideInRight {
		from { transform: translateX(20px); opacity: 0; }
		to { transform: translateX(0); opacity: 1; }
	}

	.lead-panel-title {
		font-size: 16px;
		font-weight: 700;
		margin-bottom: 20px;
		background: var(--accent-gradient);
		-webkit-background-clip: text;
		-webkit-text-fill-color: transparent;
		letter-spacing: 0.5px;
	}

	.panel-section {
		display: flex;
		flex-direction: column;
		gap: 8px;
		margin-bottom: 20px;
	}

	.section-label {
		font-size: 12px;
		font-weight: 600;
		color: var(--text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	.state-select {
		background: rgba(255, 255, 255, 0.03);
	}

	/* Tags Editor */
	.tags-container {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		min-height: 32px;
		padding: 6px;
		background: rgba(0, 0, 0, 0.15);
		border: 1px solid var(--border-color);
		border-radius: 8px;
		align-items: center;
	}

	.no-tags-placeholder {
		font-size: 12px;
		color: var(--text-muted);
		padding-left: 4px;
	}

	.lead-tag {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		background: rgba(var(--primary-rgb), 0.12);
		border: 1px solid rgba(var(--primary-rgb), 0.25);
		color: var(--text-primary);
		padding: 2px 8px;
		border-radius: 6px;
		font-size: 12px;
		font-weight: 500;
	}

	.remove-tag-btn {
		background: transparent;
		border: none;
		color: var(--text-secondary);
		cursor: pointer;
		font-size: 14px;
		font-weight: 700;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		padding: 0;
		width: 14px;
		height: 14px;
		border-radius: 50%;
		transition: background-color 0.2s, color 0.2s;
	}

	.remove-tag-btn:hover {
		background: rgba(255, 255, 255, 0.1);
		color: var(--danger);
	}

	.tag-input-form {
		display: flex;
		gap: 8px;
		margin-top: 8px;
	}

	.tag-input {
		height: 34px;
		font-size: 13px;
	}

	.add-tag-btn {
		height: 34px;
		width: 34px;
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		padding: 0;
		font-size: 16px;
	}

	/* Panel Tabs */
	.panel-tabs {
		display: flex;
		border-bottom: 1px solid var(--border-color);
		margin-bottom: 16px;
		gap: 4px;
	}

	.panel-tab-btn {
		flex: 1;
		background: transparent;
		border: none;
		border-bottom: 2px solid transparent;
		color: var(--text-secondary);
		font-size: 13px;
		font-weight: 600;
		padding: 8px 0;
		cursor: pointer;
		transition: color 0.2s, border-color 0.2s;
	}

	.panel-tab-btn:hover {
		color: var(--text-primary);
	}

	.panel-tab-btn.active {
		color: #fff;
		border-bottom-color: rgb(var(--primary-rgb));
	}

	/* Tab Content Layout */
	.tab-content-container {
		flex: 1;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	/* Notes Timeline */
	.notes-timeline {
		flex: 1;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: 10px;
		padding-right: 4px;
		margin-bottom: 14px;
	}

	.note-card {
		padding: 12px 14px;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.note-header {
		display: flex;
		justify-content: space-between;
		font-size: 11px;
	}

	.note-author {
		font-weight: 600;
		color: var(--text-primary);
	}

	.note-time {
		color: var(--text-muted);
	}

	.note-body {
		font-size: 13px;
		color: var(--text-secondary);
		line-height: 1.4;
		white-space: pre-wrap;
	}

	.add-note-form {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.note-textarea {
		height: 60px;
		font-size: 13px;
		resize: none;
		padding: 8px 12px;
	}

	.add-note-btn {
		align-self: flex-end;
		padding: 6px 12px;
		font-size: 12px;
		height: 32px;
	}

	/* History Timeline */
	.history-timeline {
		flex: 1;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: 16px;
		padding: 4px 4px 4px 8px;
	}

	.history-card {
		display: flex;
		gap: 12px;
		position: relative;
	}

	.history-card:not(:last-child)::after {
		content: "";
		position: absolute;
		left: 5px;
		top: 14px;
		bottom: -22px;
		width: 1px;
		background: var(--border-color);
	}

	.history-circle {
		width: 12px;
		height: 12px;
		border-radius: 50%;
		margin-top: 3px;
		flex-shrink: 0;
		box-shadow: 0 0 8px currentColor;
		z-index: 1;
	}

	.history-content {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.history-transition {
		font-size: 13px;
		font-weight: 500;
		color: var(--text-secondary);
	}

	.history-state {
		font-weight: 600;
	}

	.history-state.active-state {
		color: var(--text-primary);
	}

	.history-meta {
		font-size: 11px;
		color: var(--text-muted);
	}

	.empty-timeline-state {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100px;
		font-size: 13px;
		color: var(--text-muted);
		text-align: center;
	}

	/* No Lead State */
	.no-lead-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		flex: 1;
		gap: 14px;
		text-align: center;
		padding: 24px;
	}

	.no-lead-text {
		font-size: 13px;
		color: var(--text-muted);
		line-height: 1.5;
	}

	.start-lead-btn {
		width: 100%;
		max-width: 200px;
	}
</style>
