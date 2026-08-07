<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
	import { InboxState } from '$lib/store.svelte';
	import DevTestWidget from '$lib/DevTestWidget.svelte';
	import Icon from '$lib/Icon.svelte';

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

	onMount(() => {
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

		(async () => {
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
			} catch (err) {
				goto('/login');
			}
		})();

		return () => {
			window.removeEventListener('lead-state-changed', handleLeadStateChange as EventListener);
			window.removeEventListener('dev-message-sent', handleDevMessageSent);
		};
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

	// Derived list of displayable messages
	let displayMessages = $derived.by(() => {
		const msgs = inbox.messages;
		const reactionsMap: Record<string, string[]> = {};

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
		return st ? st.color : '#0B6E99';
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

	// Tag styling rotation (Pink, Yellow, Blue)
	function getTagStyleClass(index: number) {
		const styles = ['badge-pink', 'badge-yellow', 'badge-blue'];
		return styles[index % styles.length];
	}
</script>

{#if showSetupBanner}
	<div class="setup-banner" role="alert">
		<div class="setup-banner-inner">
			<Icon name="zap" size={16} color="var(--yellow-primary)" />
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
		>
			<Icon name="x" size={14} color="var(--text-muted)" />
		</button>
	</div>
{/if}

<div class="inbox-layout" class:has-lead-panel={leadTrackingEnabled && inbox.activeConvo} class:has-banner={showSetupBanner}>
	<!-- Left Navigation & Conversations Pane -->
	<div class="sidebar glass-panel">
		<!-- Header / Profile Section -->
		<div class="profile-header">
			<div class="logo-area">
				<div class="logo-box">
					<Icon name="bot" size={18} color="var(--blue-text)" />
				</div>
				<h2 class="logo-text">What Funnel</h2>
			</div>
			{#if inbox.currentUser}
				<div class="user-meta">
					<div class="user-email">{inbox.currentUser.email}</div>
					<div class="badge-blue" style="display: inline-block; margin-top: 4px;">{inbox.currentUser.role}</div>
				</div>
			{/if}
			
			<div class="nav-links">
				{#if inbox.currentUser?.role === 'admin'}
					<a href="/settings/account" class="nav-btn">
						<Icon name="settings" size={13} /> Settings
					</a>
				{/if}
				<button onclick={handleLogout} class="nav-btn logout-btn">
					<Icon name="logout" size={13} color="var(--danger)" /> Logout
				</button>
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
					<div class="convo-avatar large">{getInitial(inbox.activeConvo.contact_name)}</div>
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
							<Icon name="user" size={14} /> Assign Agent
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
								<span class="badge-pink ai-badge">AI Assistant</span>
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
									<span><Icon name="kb" size={14} /> Document</span>
									<a href={msg.parsedContent.media_url} download class="doc-link">Download File</a>
								</div>
							{:else if msg.content_type === 'location'}
								<p class="msg-text">Location: {msg.parsedContent.text || `${msg.parsedContent.latitude}, ${msg.parsedContent.longitude}`}</p>
							{:else if msg.content_type === 'contact'}
								<div class="msg-contact-card">
									<span><Icon name="user" size={14} /> Contact Card</span>
									<strong>{msg.parsedContent.text || 'Unnamed'}</strong>
								</div>
							{/if}

							<!-- Inline Reactions -->
							{#if msg.reactions && msg.reactions.length > 0}
								<div class="reactions-list">
									{#each msg.reactions as rx}
										<span class="reaction-badge">{rx}</span>
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
					<Icon name="send" size={15} />
				</button>
			</form>
		{:else}
			<div class="thread-empty-state">
				<div style="width: 44px; height: 44px; border-radius: 8px; background: var(--blue-bg); border: 1px solid var(--blue-border); display: flex; align-items: center; justify-content: center; margin-bottom: 8px;">
					<Icon name="chat" size={24} color="var(--blue-text)" />
				</div>
				<h2>No Conversation Selected</h2>
				<p>Choose a conversation from the left sidebar to view messages.</p>
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
						{#each inbox.activeConvo.lead.tags || [] as tag, idx}
							<span class={`${getTagStyleClass(idx)} lead-tag`}>
								{tag}
								<button class="remove-tag-btn" onclick={() => removeTag(tag)}>
									<Icon name="x" size={10} />
								</button>
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
						<Icon name="notes" size={13} /> Notes ({notes.length})
					</button>
					<button 
						class="panel-tab-btn" 
						class:active={activePanelTab === 'history'} 
						onclick={() => activePanelTab = 'history'}
					>
						<Icon name="history" size={13} /> History
					</button>
				</div>

				<div class="tab-content-container">
					{#if activePanelTab === 'notes'}
						<!-- Notes timeline -->
						<div class="notes-timeline">
							{#each notes as note}
								<div class="notion-callout note-card">
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
												<Icon name="arrow-right" size={10} />
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
					<p class="no-lead-text">This conversation is not currently tracked as a lead.</p>
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
	/* Setup Banner */
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
		background: var(--yellow-bg);
		border-bottom: 1px solid var(--yellow-border);
	}
	.setup-banner-inner {
		display: flex;
		align-items: center;
		gap: 10px;
		flex: 1;
		min-width: 0;
	}
	.setup-banner-content {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.setup-banner-content strong {
		font-size: 13px;
		font-weight: 600;
		color: var(--yellow-text);
	}
	.setup-banner-links {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 6px;
	}
	.setup-banner-link {
		font-size: 12px;
		color: var(--yellow-text);
		text-decoration: underline;
	}
	.sep {
		color: var(--text-muted);
		font-size: 12px;
	}
	.setup-banner-dismiss {
		background: none;
		border: none;
		cursor: pointer;
		padding: 4px;
		border-radius: 4px;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.inbox-layout {
		display: grid;
		grid-template-columns: 300px 1fr;
		height: 100vh;
		background-color: var(--bg-page);
		padding: 12px;
		gap: 12px;
	}

	.inbox-layout.has-banner {
		padding-top: 48px;
		height: calc(100vh - 36px);
	}

	.sidebar {
		display: flex;
		flex-direction: column;
		height: 100%;
		overflow: hidden;
		background: var(--bg-sidebar);
	}

	.profile-header {
		padding: 16px;
		border-bottom: 1px solid var(--border-color);
	}

	.logo-area {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 10px;
	}

	.logo-box {
		width: 24px;
		height: 24px;
		border-radius: 5px;
		background: var(--blue-bg);
		border: 1px solid var(--blue-border);
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.logo-text {
		font-size: 16px;
		font-weight: 700;
		color: var(--text-primary);
	}

	.user-meta {
		margin-bottom: 10px;
	}

	.user-email {
		font-size: 12.5px;
		color: var(--text-secondary);
	}

	.nav-links {
		display: flex;
		gap: 6px;
		margin-top: 8px;
	}

	.nav-btn {
		font-size: 12px;
		color: var(--text-secondary);
		text-decoration: none;
		background: #FFFFFF;
		border: 1px solid var(--border-color);
		padding: 4px 10px;
		border-radius: 5px;
		cursor: pointer;
		display: inline-flex;
		align-items: center;
		gap: 4px;
		transition: background-color 0.15s;
	}

	.nav-btn:hover {
		background: var(--bg-hover);
		color: var(--text-primary);
	}

	.logout-btn {
		margin-left: auto;
		color: var(--danger);
	}

	.filter-tabs {
		display: flex;
		padding: 8px 12px;
		gap: 4px;
		border-bottom: 1px solid var(--border-color);
	}

	.tab-btn {
		flex: 1;
		background: transparent;
		border: none;
		color: var(--text-secondary);
		padding: 6px;
		font-size: 12.5px;
		font-weight: 500;
		cursor: pointer;
		border-radius: 5px;
		transition: all 0.15s;
	}

	.tab-btn:hover {
		background: var(--bg-hover);
		color: var(--text-primary);
	}

	.tab-btn.active {
		background: var(--blue-bg);
		color: var(--blue-text);
		font-weight: 600;
	}

	.conversation-list {
		flex: 1;
		overflow-y: auto;
		padding: 6px;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.convo-item {
		display: flex;
		align-items: center;
		padding: 10px;
		background: transparent;
		border: 1px solid transparent;
		border-radius: 6px;
		cursor: pointer;
		width: 100%;
		text-align: left;
		gap: 10px;
		transition: all 0.15s;
	}

	.convo-item:hover {
		background: var(--bg-hover);
	}

	.convo-item.active {
		background: var(--blue-bg);
		border-color: var(--blue-border);
	}

	.convo-avatar {
		width: 36px;
		height: 36px;
		background: #E8E8E5;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 14px;
		font-weight: 600;
		color: var(--text-primary);
		position: relative;
		flex-shrink: 0;
	}

	.convo-avatar.large {
		width: 40px;
		height: 40px;
		font-size: 16px;
	}

	.unread-badge {
		position: absolute;
		top: 0;
		right: 0;
		width: 9px;
		height: 9px;
		border-radius: 50%;
		background-color: var(--pink-primary);
		border: 2px solid #FFFFFF;
	}

	.convo-info {
		flex: 1;
		min-width: 0;
	}

	.convo-top {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 4px;
		margin-bottom: 2px;
	}

	.convo-name {
		font-size: 13.5px;
		font-weight: 600;
		color: var(--text-primary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.lead-state-badge {
		font-size: 10px;
		font-weight: 600;
		padding: 1px 5px;
		border-radius: 4px;
		white-space: nowrap;
	}

	.convo-time {
		font-size: 10.5px;
		color: var(--text-muted);
	}

	.convo-preview {
		font-size: 12.5px;
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
		background: #FFFFFF;
	}

	.thread-header {
		padding: 14px 20px;
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
		font-size: 15px;
		font-weight: 600;
		color: var(--text-primary);
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
		margin-top: 6px;
		width: 220px;
		padding: 12px;
		z-index: 10;
		display: flex;
		flex-direction: column;
		gap: 8px;
		background: #FFFFFF;
	}

	.assign-dropdown h4 {
		font-size: 11px;
		color: var(--text-secondary);
		text-transform: uppercase;
		margin-bottom: 2px;
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
		padding: 20px;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.load-more-container {
		display: flex;
		justify-content: center;
		margin-bottom: 12px;
	}

	.load-more-btn {
		font-size: 12px;
		padding: 4px 10px;
	}

	.message-row {
		display: flex;
		justify-content: flex-start;
	}

	.message-row.outbound {
		justify-content: flex-end;
	}

	.message-bubble {
		max-width: 65%;
		padding: 10px 14px;
		border-radius: 8px;
		background: var(--bg-sidebar);
		border: 1px solid var(--border-color);
		position: relative;
	}

	.message-row.outbound .message-bubble {
		background: var(--blue-bg);
		border-color: var(--blue-border);
	}

	.message-row.ai .message-bubble {
		background: var(--pink-bg);
		border-color: var(--pink-border);
	}

	.ai-badge {
		margin-bottom: 4px;
	}

	.msg-text {
		font-size: 13.5px;
		line-height: 1.5;
		color: var(--text-primary);
	}

	.msg-media {
		max-width: 100%;
		max-height: 240px;
		border-radius: 6px;
		margin-top: 4px;
	}

	.msg-caption {
		font-size: 12.5px;
		color: var(--text-secondary);
		margin-top: 4px;
	}

	.msg-doc {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: 13px;
	}

	.doc-link {
		color: var(--blue-text);
		text-decoration: none;
		font-weight: 500;
	}

	.msg-contact-card {
		display: flex;
		flex-direction: column;
		gap: 2px;
		font-size: 13px;
	}

	.reactions-list {
		display: flex;
		gap: 4px;
		margin-top: 6px;
		flex-wrap: wrap;
	}

	.reaction-badge {
		background: #FFFFFF;
		border: 1px solid var(--border-color);
		padding: 2px 6px;
		border-radius: 4px;
		font-size: 11px;
		font-weight: 500;
	}

	.message-time {
		display: block;
		font-size: 10px;
		color: var(--text-muted);
		margin-top: 4px;
		text-align: right;
	}

	.external-origin-indicator {
		font-style: italic;
		font-weight: 500;
		color: var(--blue-text);
	}

	.compose-area {
		padding: 14px 20px;
		border-top: 1px solid var(--border-color);
		display: flex;
		gap: 10px;
	}

	.compose-input {
		flex: 1;
		resize: none;
		height: 40px;
		line-height: 20px;
	}

	.send-btn {
		height: 40px;
	}

	.thread-empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		height: 100%;
		color: var(--text-muted);
		gap: 6px;
	}

	.inbox-layout.has-lead-panel {
		grid-template-columns: 300px 1fr 320px;
	}

	/* State Filter */
	.state-filter-container {
		padding: 8px 12px;
		border-bottom: 1px solid var(--border-color);
	}
	
	.state-filter-select {
		height: 32px;
		font-size: 12.5px;
	}

	/* Lead Panel */
	.lead-panel {
		padding: 18px;
		display: flex;
		flex-direction: column;
		gap: 16px;
		overflow-y: auto;
		background: #FFFFFF;
	}

	.lead-panel-title {
		font-size: 15px;
		font-weight: 700;
		color: var(--text-primary);
		border-bottom: 1px solid var(--border-color);
		padding-bottom: 10px;
	}

	.panel-section {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.section-label {
		font-size: 11px;
		font-weight: 600;
		color: var(--text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.4px;
	}

	.state-select {
		height: 34px;
		font-size: 13px;
	}

	.tags-container {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		margin-bottom: 6px;
	}

	.lead-tag {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 2px 8px;
		border-radius: 4px;
		font-size: 12px;
		font-weight: 500;
	}

	.remove-tag-btn {
		background: none;
		border: none;
		cursor: pointer;
		display: flex;
		align-items: center;
		padding: 0;
	}

	.no-tags-placeholder {
		font-size: 12.5px;
		color: var(--text-muted);
		font-style: italic;
	}

	.tag-input-form {
		display: flex;
		gap: 6px;
	}

	.tag-input {
		flex: 1;
		height: 32px;
		font-size: 12.5px;
	}

	.add-tag-btn {
		height: 32px;
		padding: 0 10px;
	}

	.panel-tabs {
		display: flex;
		border-bottom: 1px solid var(--border-color);
		gap: 4px;
	}

	.panel-tab-btn {
		flex: 1;
		background: transparent;
		border: none;
		color: var(--text-secondary);
		padding: 8px;
		font-size: 12.5px;
		font-weight: 500;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 6px;
		border-bottom: 2px solid transparent;
		transition: all 0.15s;
	}

	.panel-tab-btn.active {
		color: var(--blue-text);
		border-bottom-color: var(--blue-primary);
		font-weight: 600;
	}

	.tab-content-container {
		flex: 1;
		display: flex;
		flex-direction: column;
	}

	.notes-timeline, .history-timeline {
		display: flex;
		flex-direction: column;
		gap: 8px;
		margin-bottom: 12px;
		max-height: 260px;
		overflow-y: auto;
	}

	.note-card {
		flex-direction: column;
		gap: 4px;
	}

	.note-header {
		display: flex;
		justify-content: space-between;
		font-size: 11px;
		font-weight: 600;
	}

	.note-time {
		color: var(--yellow-text);
		opacity: 0.8;
		font-weight: 400;
	}

	.note-body {
		font-size: 13px;
		line-height: 1.4;
	}

	.add-note-form {
		display: flex;
		flex-direction: column;
		gap: 6px;
		margin-top: auto;
	}

	.note-textarea {
		height: 60px;
		font-size: 12.5px;
		resize: none;
	}

	.add-note-btn {
		height: 32px;
		font-size: 12.5px;
	}

	.empty-timeline-state {
		font-size: 12.5px;
		color: var(--text-muted);
		text-align: center;
		padding: 16px 0;
	}

	.history-card {
		display: flex;
		align-items: flex-start;
		gap: 8px;
		padding: 6px 0;
	}

	.history-circle {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		margin-top: 4px;
		flex-shrink: 0;
	}

	.history-content {
		font-size: 12px;
	}

	.history-transition {
		color: var(--text-primary);
		font-weight: 500;
	}

	.history-meta {
		font-size: 10.5px;
		color: var(--text-muted);
	}

	.no-lead-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 32px 16px;
		text-align: center;
		gap: 12px;
	}

	.no-lead-text {
		font-size: 13px;
		color: var(--text-secondary);
	}

	.start-lead-btn {
		width: 100%;
		height: 36px;
	}
</style>
