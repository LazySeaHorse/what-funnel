<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
	import { InboxState } from '$lib/store';

	const inbox = new InboxState();
	let composeText = $state('');
	let messageContainer: HTMLDivElement | null = $state(null);
	let isAssignDropdownOpen = $state(false);

	onMount(async () => {
		try {
			await inbox.init();
			if (!inbox.currentUser) {
				goto('/login');
			}
		} catch (err) {
			goto('/login');
		}
	});

	// Derived list of displayable messages (attaches reactions, hides reaction bubbles)
	let displayMessages = $derived.by(() => {
		const msgs = inbox.messages;
		const reactionsMap: Record<string, string[]> = {};

		// 1. Gather all reactions
		for (const m of msgs) {
			if (m.content_type === 'reaction') {
				try {
					const contentObj = typeof m.content === 'string' ? JSON.parse(m.content) : m.content;
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
			.filter((m) => m.content_type !== 'reaction')
			.map((m) => {
				const contentObj = typeof m.content === 'string' ? JSON.parse(m.content) : m.content;
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
			.filter((u) => assignedIDs.includes(u.id))
			.map((u) => u.email.split('@')[0])
			.join(', ');
	}
</script>

<div class="inbox-layout">
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
					<a href="/settings/channels" class="nav-btn">Channels</a>
					<a href="/settings/users" class="nav-btn">Users</a>
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
							<span class="convo-time">{formatTime(convo.last_message_at)}</span>
						</div>
						<div class="convo-preview">
							{#if convo.last_message_preview}
								{convo.last_message_preview.content_type === 'text' 
									? JSON.parse(convo.last_message_preview.content).text 
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
							
							<span class="message-time">{formatTime(msg.created_at)}</span>
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
</div>

<style>
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
</style>
