<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
	import Icon from '$lib/Icon.svelte';

	let loading = $state(true);
	let error = $state('');
	let successMsg = $state('');
	let currentUser = $state<any | null>(null);
	let productMode = $state('full_workspace');

	// Paste compile state
	let rawText = $state('');
	let compiling = $state(false);

	// Concepts & Suggestions state
	let concepts = $state<any[]>([]);
	let suggestions = $state<any[]>([]);

	// Inline editing state for suggestions
	let editingSuggestionId = $state<string | null>(null);
	let editPayload = $state<any>(null);

	onMount(async () => {
		try {
			currentUser = await apiRequest('/auth/me');
			if (currentUser.role !== 'admin') {
				goto('/inbox');
				return;
			}
			const account = await apiRequest('/workspace/account');
			if (account) {
				productMode = account.product_mode || 'full_workspace';
			}
			await Promise.all([loadConcepts(), loadSuggestions()]);
		} catch (err) {
			goto('/login');
		} finally {
			loading = false;
		}
	});

	async function loadConcepts() {
		try {
			const res = await apiRequest('/api/kb/concepts');
			concepts = res.concepts || [];
		} catch (err: any) {
			error = 'Failed to load concepts: ' + err.message;
		}
	}

	async function loadSuggestions() {
		try {
			const res = await apiRequest('/api/kb/suggestions?status_filter=pending');
			suggestions = res.suggestions || [];
		} catch (err: any) {
			error = 'Failed to load suggestions: ' + err.message;
		}
	}

	async function handleCompilePaste(e: Event) {
		e.preventDefault();
		if (!rawText.trim()) return;

		error = '';
		successMsg = '';
		compiling = true;

		try {
			const res = await apiRequest('/api/kb/compile-paste', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({ raw_text: rawText })
			});

			if (res.added_concepts && res.added_concepts.length > 0) {
				successMsg = `Successfully compiled and added ${res.added_concepts.length} concepts directly.`;
				rawText = '';
				await loadConcepts();
			} else if (res.suggestion_ids && res.suggestion_ids.length > 0) {
				successMsg = `${res.suggestion_ids.length} concepts extracted and queued for administrator approval.`;
				rawText = '';
				await loadSuggestions();
			} else {
				successMsg = 'Compilation complete, but no concepts were found.';
			}
		} catch (err: any) {
			error = 'Failed to compile paste: ' + err.message;
		} finally {
			compiling = false;
		}
	}

	async function handleDeleteConcept(id: string) {
		if (!confirm('Are you sure you want to delete this concept?')) return;
		error = '';
		successMsg = '';
		try {
			await apiRequest(`/api/kb/concepts/${id}`, { method: 'DELETE' });
			successMsg = 'Concept deleted successfully.';
			await loadConcepts();
		} catch (err: any) {
			error = 'Failed to delete concept: ' + err.message;
		}
	}

	async function handleApproveSuggestion(sug: any) {
		error = '';
		successMsg = '';
		try {
			await apiRequest(`/api/kb/suggestions/${sug.id}/approve`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ payload: sug.payload })
			});
			successMsg = 'Suggestion approved and concept added.';
			await Promise.all([loadConcepts(), loadSuggestions()]);
		} catch (err: any) {
			error = 'Failed to approve suggestion: ' + err.message;
		}
	}

	async function handleRejectSuggestion(id: string) {
		error = '';
		successMsg = '';
		try {
			await apiRequest(`/api/kb/suggestions/${id}/reject`, { method: 'POST' });
			successMsg = 'Suggestion rejected.';
			await loadSuggestions();
		} catch (err: any) {
			error = 'Failed to reject suggestion: ' + err.message;
		}
	}

	function startEditSuggestion(sug: any) {
		editingSuggestionId = sug.id;
		editPayload = JSON.parse(JSON.stringify(sug.payload));
	}

	function cancelEditSuggestion() {
		editingSuggestionId = null;
		editPayload = null;
	}

	async function saveEditedSuggestion(sug: any) {
		sug.payload = editPayload;
		await handleApproveSuggestion(sug);
		editingSuggestionId = null;
		editPayload = null;
	}
</script>

<div class="settings-container">
	<div class="settings-sidebar glass-panel">
		<h2 class="sidebar-title">Settings</h2>
		<nav class="sidebar-nav">
			<a href="/inbox" class="nav-item back-item">
				<Icon name="arrow-left" size={14} /> Back to Inbox
			</a>
			<a href="/settings/account" class="nav-item">
				<Icon name="settings" size={14} /> Account Settings
			</a>
			<a href="/settings/channels" class="nav-item">
				<Icon name="channels" size={14} /> Channels
			</a>
			<a href="/settings/users" class="nav-item">
				<Icon name="users" size={14} /> Workspace Users
			</a>
			{#if productMode !== 'chatbot_only'}
				<a href="/settings/pipeline" class="nav-item">
					<Icon name="pipeline" size={14} /> Lead Pipeline
				</a>
			{/if}
			<a href="/settings/knowledge-base" class="nav-item active">
				<Icon name="kb" size={14} /> Knowledge Base
			</a>
		</nav>
	</div>

	<div class="settings-content glass-panel">
		<div class="content-header">
			<div>
				<h1>Knowledge Base</h1>
				<p class="subtitle">Train your AI assistant by compiling business documents and FAQs</p>
			</div>
		</div>

		{#if error}
			<div class="banner error">{error}</div>
		{/if}

		{#if successMsg}
			<div class="banner success">{successMsg}</div>
		{/if}

		{#if loading}
			<div class="loading-state">Loading Knowledge Base...</div>
		{:else}
			<div class="kb-sections-container">
				<!-- Paste & Compile Section -->
				<div class="settings-card glass-panel">
					<h3>Compile Raw Text / FAQs</h3>
					<p class="card-desc">Paste unstructured business information, policies, or FAQs below. The AI compiler will structure it into structured knowledge concepts.</p>
					
					<form onsubmit={handleCompilePaste} class="compile-form">
						<textarea 
							class="input-field raw-text-area" 
							placeholder="Paste your business details, pricing, FAQs, services, hours, policies..."
							bind:value={rawText}
							required
							disabled={compiling}
						></textarea>
						<button 
							type="submit" 
							class="btn-primary compile-btn"
							disabled={compiling || !rawText.trim()}
						>
							{#if compiling}
								Compiling...
							{:else}
								<Icon name="sparkles" size={15} /> Compile & Structure Knowledge
							{/if}
						</button>
					</form>
				</div>

				<!-- Pending Approval Suggestions -->
				{#if suggestions.length > 0}
					<div class="settings-card glass-panel">
						<div class="section-header-row">
							<h3>Pending Concepts for Approval</h3>
							<span class="badge-yellow">{suggestions.length} pending</span>
						</div>
						<p class="card-desc">These concepts were compiled and are waiting for your review before going live.</p>

						<div class="suggestions-list">
							{#each suggestions as sug}
								<div class="suggestion-card glass-panel">
									{#if editingSuggestionId === sug.id}
										<div class="edit-form">
											<div class="form-group">
												<label>Title</label>
												<input type="text" class="input-field" bind:value={editPayload.title} />
											</div>
											<div class="form-group">
												<label>Category</label>
												<input type="text" class="input-field" bind:value={editPayload.category} />
											</div>
											<div class="form-group">
												<label>Content</label>
												<textarea class="input-field" rows="3" bind:value={editPayload.content}></textarea>
											</div>
											<div class="edit-actions">
												<button class="btn-secondary" onclick={cancelEditSuggestion}>Cancel</button>
												<button class="btn-primary" onclick={() => saveEditedSuggestion(sug)}>Approve & Save</button>
											</div>
										</div>
									{:else}
										<div class="sug-header">
											<span class="sug-title">{sug.payload?.title || sug.payload?.name || 'Untitled Concept'}</span>
											<span class="badge-blue sug-cat">{sug.payload?.category || 'General'}</span>
										</div>
										<p class="sug-content">{sug.payload?.content || sug.payload?.description || JSON.stringify(sug.payload)}</p>
										<div class="sug-actions">
											<button class="btn-secondary edit-btn" onclick={() => startEditSuggestion(sug)}>
												<Icon name="edit" size={13} /> Edit
											</button>
											<button class="btn-secondary reject-btn" onclick={() => handleRejectSuggestion(sug.id)}>
												Reject
											</button>
											<button class="btn-primary approve-btn" onclick={() => handleApproveSuggestion(sug)}>
												<Icon name="check" size={13} /> Approve
											</button>
										</div>
									{/if}
								</div>
							{/each}
						</div>
					</div>
				{/if}

				<!-- Active Live Concepts -->
				<div class="settings-card glass-panel">
					<div class="section-header-row">
						<h3>Active Knowledge Concepts</h3>
						<span class="badge-blue">{concepts.length} active</span>
					</div>
					<p class="card-desc">Concepts currently active and used by your AI assistant to answer customer messages.</p>

					<div class="concepts-grid">
						{#each concepts as concept}
							<div class="concept-item glass-panel">
								<div class="concept-header">
									<span class="concept-title">{concept.title || concept.name || 'Concept'}</span>
									<button class="delete-btn" onclick={() => handleDeleteConcept(concept.id)} title="Delete Concept">
										<Icon name="trash" size={14} color="var(--danger)" />
									</button>
								</div>
								<div class="concept-meta">
									<span class="badge-blue">{concept.category || 'General'}</span>
								</div>
								<p class="concept-content">{concept.content || concept.description || ''}</p>
							</div>
						{:else}
							<div class="empty-state">
								<div style="width: 40px; height: 40px; border-radius: 8px; background: var(--blue-bg); border: 1px solid var(--blue-border); display: flex; align-items: center; justify-content: center; margin: 0 auto 8px;">
									<Icon name="kb" size={20} color="var(--blue-text)" />
								</div>
								<h4>No Active Concepts</h4>
								<p>Paste raw text above to compile concepts into your knowledge base.</p>
							</div>
						{/each}
					</div>
				</div>
			</div>
		{/if}
	</div>
</div>

<style>
	.settings-container {
		display: flex;
		gap: 20px;
		max-width: 1100px;
		margin: 24px auto;
		padding: 0 16px;
		height: calc(100vh - 48px);
	}

	.settings-sidebar {
		width: 240px;
		padding: 20px;
		display: flex;
		flex-direction: column;
		gap: 16px;
		background: var(--bg-sidebar);
		height: 100%;
	}

	.sidebar-title {
		font-size: 16px;
		font-weight: 700;
		color: var(--text-primary);
	}

	.sidebar-nav {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.nav-item {
		padding: 8px 12px;
		border-radius: 6px;
		color: var(--text-secondary);
		text-decoration: none;
		font-size: 13px;
		font-weight: 500;
		display: flex;
		align-items: center;
		gap: 8px;
		transition: all 0.15s;
	}

	.nav-item:hover {
		background: var(--bg-hover);
		color: var(--text-primary);
	}

	.nav-item.active {
		background: var(--blue-bg);
		color: var(--blue-text);
		font-weight: 600;
	}

	.back-item {
		margin-bottom: 8px;
		color: var(--text-muted);
	}

	.settings-content {
		flex: 1;
		padding: 28px;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: 20px;
		background: #FFFFFF;
		height: 100%;
	}

	.content-header h1 {
		font-size: 20px;
		font-weight: 700;
		margin-bottom: 2px;
	}

	.subtitle {
		font-size: 13.5px;
		color: var(--text-secondary);
	}

	.banner.error {
		padding: 10px 14px;
		background: var(--danger-bg);
		border: 1px solid rgba(235, 87, 87, 0.3);
		border-radius: 6px;
		color: var(--danger);
		font-size: 13px;
	}

	.banner.success {
		padding: 10px 14px;
		background: var(--success-bg);
		border: 1px solid rgba(46, 125, 50, 0.3);
		border-radius: 6px;
		color: var(--success);
		font-size: 13px;
	}

	.loading-state {
		text-align: center;
		padding: 40px;
		color: var(--text-secondary);
		font-size: 13.5px;
	}

	.kb-sections-container {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.settings-card {
		padding: 20px;
		display: flex;
		flex-direction: column;
		gap: 6px;
		background: #FFFFFF;
	}

	.settings-card h3 {
		font-size: 15px;
		font-weight: 600;
		color: var(--text-primary);
	}

	.card-desc {
		font-size: 13px;
		color: var(--text-secondary);
		margin-bottom: 10px;
	}

	.section-header-row {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.compile-form {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.raw-text-area {
		height: 120px;
		resize: vertical;
		font-size: 13px;
		line-height: 1.45;
	}

	.compile-btn {
		align-self: flex-start;
	}

	.suggestions-list {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.suggestion-card {
		padding: 14px;
		display: flex;
		flex-direction: column;
		gap: 8px;
		background: var(--bg-hover);
	}

	.sug-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.sug-title {
		font-size: 14px;
		font-weight: 600;
		color: var(--text-primary);
	}

	.sug-content {
		font-size: 13px;
		color: var(--text-secondary);
		line-height: 1.45;
	}

	.sug-actions {
		display: flex;
		justify-content: flex-end;
		gap: 6px;
	}

	.concepts-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
		gap: 12px;
	}

	.concept-item {
		padding: 14px;
		display: flex;
		flex-direction: column;
		gap: 6px;
		background: #FFFFFF;
	}

	.concept-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
	}

	.concept-title {
		font-size: 13.5px;
		font-weight: 600;
		color: var(--text-primary);
	}

	.delete-btn {
		background: none;
		border: none;
		cursor: pointer;
		padding: 2px;
	}

	.concept-content {
		font-size: 12.5px;
		color: var(--text-secondary);
		line-height: 1.4;
	}

	.empty-state {
		grid-column: 1 / -1;
		text-align: center;
		padding: 32px;
		color: var(--text-secondary);
	}

	.empty-state h4 {
		font-size: 15px;
		font-weight: 600;
		margin-bottom: 2px;
	}

	.empty-state p {
		font-size: 13px;
	}
</style>
