<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';

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
			await apiRequest(`/api/kb/concepts/${id}`, {
				method: 'DELETE'
			});
			successMsg = 'Concept deleted successfully.';
			await loadConcepts();
		} catch (err: any) {
			error = 'Failed to delete concept: ' + err.message;
		}
	}

	function parsePayload(sugg: any) {
		return typeof sugg.proposed_payload === 'string'
			? JSON.parse(sugg.proposed_payload)
			: sugg.proposed_payload;
	}

	function startEditSuggestion(sugg: any) {
		editingSuggestionId = sugg.id;
		const rawPayload = parsePayload(sugg);
		editPayload = JSON.parse(jsonStringifySafe(rawPayload));
	}

	function jsonStringifySafe(obj: any) {
		return JSON.stringify(obj);
	}

	function cancelEditSuggestion() {
		editingSuggestionId = null;
		editPayload = null;
	}

	async function handleApproveSuggestion(sugg: any) {
		error = '';
		successMsg = '';

		let payloadToSend: any = null;
		if (editingSuggestionId === sugg.id) {
			payloadToSend = editPayload;
		}

		try {
			await apiRequest(`/api/kb/suggestions/${sugg.id}/approve`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({
					reviewed_by: currentUser.id,
					edited_payload: payloadToSend
				})
			});
			successMsg = 'Suggestion approved successfully.';
			editingSuggestionId = null;
			editPayload = null;
			await Promise.all([loadConcepts(), loadSuggestions()]);
		} catch (err: any) {
			error = 'Failed to approve suggestion: ' + err.message;
		}
	}

	async function handleRejectSuggestion(id: string) {
		if (!confirm('Are you sure you want to reject this suggestion?')) return;
		error = '';
		successMsg = '';
		try {
			await apiRequest(`/api/kb/suggestions/${id}/reject`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({
					reviewed_by: currentUser.id
				})
			});
			successMsg = 'Suggestion rejected.';
			await loadSuggestions();
		} catch (err: any) {
			error = 'Failed to reject suggestion: ' + err.message;
		}
	}
</script>

<div class="settings-container">
	<div class="settings-sidebar">
		<div class="sidebar-header">
			<h2>Settings</h2>
		</div>
		<nav class="sidebar-nav">
			<a href="/inbox" class="nav-item">← Back to Inbox</a>
			<a href="/settings/account" class="nav-item">Account Settings</a>
			<a href="/settings/channels" class="nav-item">Channels</a>
			<a href="/settings/users" class="nav-item">Workspace Users</a>
			{#if productMode !== 'chatbot_only'}
				<a href="/settings/pipeline" class="nav-item">Lead Pipeline</a>
			{/if}
			<a href="/settings/knowledge-base" class="nav-item active">Knowledge Base</a>
		</nav>
	</div>

	<div class="settings-content glass-panel">
		<div class="content-header">
			<div>
				<h1>AI Knowledge Base Settings</h1>
				<p class="subtitle">Import raw documentation and manage automatic answer extraction</p>
			</div>
		</div>

		{#if error}
			<div class="banner error">{error}</div>
		{/if}

		{#if successMsg}
			<div class="banner success">{successMsg}</div>
		{/if}

		{#if loading}
			<div class="loading-state">Loading knowledge base configurations...</div>
		{:else}
			<!-- Paste Compiler Section -->
			<div class="settings-section glass-panel">
				<h3>Compile New Raw Content</h3>
				<p class="section-desc">Paste FAQs, policy drafts, hours, or general documentation. The AI compiler will automatically structure it into Knowledge Base concepts.</p>
				<form onsubmit={handleCompilePaste} class="paste-form">
					<textarea
						class="input-field textarea-field"
						bind:value={rawText}
						placeholder="Example: We are open Monday to Friday 9am to 6pm. The price for our premium plan is $49/mo. For custom support, email support@example.com."
						rows={6}
						required
					></textarea>
					<div class="action-bar">
						<button type="submit" class="btn-primary" disabled={compiling || !rawText.trim()}>
							{compiling ? 'Compiling & Embedding...' : 'Compile Content'}
						</button>
					</div>
				</form>
			</div>

			<!-- Suggestions Queue Section -->
			<div class="settings-section">
				<h3>Approval Queue ({suggestions.length})</h3>
				<p class="section-desc">Suggestions extracted automatically from dormant conversation mining or large content pastes. Review before publishing.</p>
				
				{#if suggestions.length === 0}
					<div class="empty-state glass-panel">No pending suggestions needing approval.</div>
				{:else}
					<div class="suggestions-list">
						{#each suggestions as sugg}
							{@const payload = parsePayload(sugg)}
							<div class="suggestion-card glass-panel">
								<div class="card-header">
									<div class="card-title-group">
										<span class="badge type-badge">{sugg.type}</span>
										<span class="confidence-badge">Confidence: {Math.round(sugg.confidence * 100)}%</span>
									</div>
									<div class="card-actions">
										{#if editingSuggestionId === sugg.id}
											<button onclick={() => handleApproveSuggestion(sugg)} class="btn-success">Save & Approve</button>
											<button onclick={cancelEditSuggestion} class="btn-secondary">Cancel</button>
										{:else}
											<button onclick={() => startEditSuggestion(sugg)} class="btn-secondary">Edit</button>
											<button onclick={() => handleApproveSuggestion(sugg)} class="btn-success">Approve</button>
											<button onclick={() => handleRejectSuggestion(sugg.id)} class="btn-danger">Reject</button>
										{/if}
									</div>
								</div>

								<div class="card-body">
									{#if editingSuggestionId === sugg.id}
										<!-- Dynamic editor depending on type -->
										{#if sugg.type === 'new_kb_concept'}
											<div class="form-group">
												<label for="title-input">Title</label>
												<input id="title-input" class="input-field" bind:value={editPayload.title} />
											</div>
											<div class="form-group">
												<label for="type-input">Type</label>
												<input id="type-input" class="input-field" bind:value={editPayload.type} />
											</div>
											<div class="form-group">
												<label for="body-input">Body (Markdown)</label>
												<textarea id="body-input" class="input-field textarea-field" rows={4} bind:value={editPayload.body_markdown}></textarea>
											</div>
										{:else if sugg.type === 'new_pattern'}
											<div class="form-group">
												<label for="q-input">Canonical Question</label>
												<input id="q-input" class="input-field" bind:value={editPayload.canonical_question} />
											</div>
											<div class="form-group">
												<label for="ans-input">Proposed Answer (Markdown)</label>
												<textarea id="ans-input" class="input-field textarea-field" rows={4} bind:value={editPayload.answer_markdown}></textarea>
											</div>
										{/if}
									{:else}
										<!-- View proposed content -->
										{#if sugg.type === 'new_kb_concept'}
											<h4 class="concept-title">{payload.title} <span class="sub-type">({payload.type})</span></h4>
											<p class="concept-body markdown-content">{payload.body_markdown}</p>
											{#if payload.tags && payload.tags.length > 0}
												<div class="tags-row">
													{#each payload.tags as t}
														<span class="tag-pill">{t}</span>
													{/each}
												</div>
											{/if}
										{:else if sugg.type === 'new_pattern'}
											<h4 class="pattern-question">Q: {payload.canonical_question}</h4>
											<p class="pattern-answer markdown-content">A: {payload.answer_markdown}</p>
											{#if payload.trigger_phrases && payload.trigger_phrases.length > 0}
												<div class="phrases-box">
													<strong>Triggering Phrases:</strong>
													<ul>
														{#each payload.trigger_phrases as phrase}
															<li>"{phrase}"</li>
														{/each}
													</ul>
												</div>
											{/if}
										{/if}
									{/if}
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>

			<!-- Active Concepts Listing Section -->
			<div class="settings-section">
				<h3>Active Knowledge Base Concepts ({concepts.length})</h3>
				<p class="section-desc">These concepts are currently compiled and available to the answering engine.</p>
				
				{#if concepts.length === 0}
					<div class="empty-state glass-panel">No concepts available. Paste some content above to get started.</div>
				{:else}
					<div class="concepts-grid">
						{#each concepts as concept}
							<div class="concept-card glass-panel">
								<div class="card-header">
									<div class="concept-title-group">
										<h4 class="concept-title">{concept.title}</h4>
										<span class="badge type-badge">{concept.type}</span>
									</div>
									<button onclick={() => handleDeleteConcept(concept.id)} class="btn-danger-icon" title="Delete Concept">&times;</button>
								</div>
								<div class="card-body">
									<p class="concept-body markdown-content">{concept.body_markdown}</p>
									{#if concept.tags && concept.tags.length > 0}
										<div class="tags-row">
											{#each concept.tags as t}
												<span class="tag-pill">{t}</span>
											{/each}
										</div>
									{/if}
								</div>
								<div class="card-footer">
									<span class="source-tag">Source: {concept.source}</span>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	</div>
</div>

<style>
	.settings-container {
		display: flex;
		height: 100%;
		gap: 24px;
	}

	.settings-sidebar {
		width: 240px;
		flex-shrink: 0;
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.sidebar-header h2 {
		font-size: 18px;
		font-weight: 700;
	}

	.sidebar-nav {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.nav-item {
		padding: 10px 14px;
		border-radius: 8px;
		color: var(--text-secondary);
		text-decoration: none;
		font-size: 14px;
		font-weight: 500;
		transition: background-color 0.2s, color 0.2s;
	}

	.nav-item:hover {
		background: rgba(255, 255, 255, 0.04);
		color: var(--text-primary);
	}

	.nav-item.active {
		background: rgba(var(--primary-rgb), 0.15);
		color: #fff;
		border: 1px solid rgba(var(--primary-rgb), 0.3);
	}

	.settings-content {
		flex: 1;
		padding: 32px;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: 32px;
	}

	.content-header h1 {
		font-size: 24px;
		font-weight: 700;
		margin-bottom: 4px;
	}

	.subtitle {
		color: var(--text-secondary);
		font-size: 14px;
	}

	.settings-section {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.settings-section h3 {
		font-size: 18px;
		font-weight: 600;
	}

	.section-desc {
		font-size: 13px;
		color: var(--text-secondary);
		margin-top: -8px;
	}

	.loading-state, .empty-state {
		text-align: center;
		padding: 32px;
		color: var(--text-secondary);
		font-size: 14px;
	}

	.banner {
		padding: 12px 16px;
		border-radius: 8px;
		font-size: 14px;
	}

	.banner.error {
		background: rgba(239, 68, 68, 0.15);
		border: 1px solid rgba(239, 68, 68, 0.3);
		color: #fca5a5;
	}

	.banner.success {
		background: rgba(34, 197, 94, 0.15);
		border: 1px solid rgba(34, 197, 94, 0.3);
		color: #86efac;
	}

	.paste-form {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.textarea-field {
		resize: vertical;
		font-family: inherit;
		line-height: 1.5;
	}

	.suggestions-list, .concepts-grid {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.concepts-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
		gap: 20px;
	}

	.suggestion-card, .concept-card {
		padding: 20px;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.card-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: 16px;
	}

	.card-title-group, .concept-title-group {
		display: flex;
		align-items: center;
		gap: 10px;
		flex-wrap: wrap;
	}

	.concept-title {
		font-size: 16px;
		font-weight: 600;
	}

	.sub-type {
		font-size: 12px;
		color: var(--text-secondary);
	}

	.pattern-question {
		font-size: 15px;
		font-weight: 600;
		color: #fff;
	}

	.badge {
		font-size: 11px;
		padding: 2px 8px;
		border-radius: 12px;
		font-weight: 600;
		text-transform: uppercase;
	}

	.type-badge {
		background: rgba(99, 102, 241, 0.15);
		color: #a5b4fc;
		border: 1px solid rgba(99, 102, 241, 0.3);
	}

	.confidence-badge {
		font-size: 12px;
		color: #86efac;
		font-weight: 500;
	}

	.card-actions {
		display: flex;
		gap: 8px;
	}

	.btn-success {
		background: rgba(34, 197, 94, 0.2);
		border: 1px solid rgba(34, 197, 94, 0.4);
		color: #86efac;
		padding: 4px 10px;
		border-radius: 6px;
		font-size: 12px;
		font-weight: 600;
		cursor: pointer;
		transition: background-color 0.2s;
	}

	.btn-success:hover {
		background: rgba(34, 197, 94, 0.35);
	}

	.btn-danger {
		background: rgba(239, 68, 68, 0.2);
		border: 1px solid rgba(239, 68, 68, 0.4);
		color: #fca5a5;
		padding: 4px 10px;
		border-radius: 6px;
		font-size: 12px;
		font-weight: 600;
		cursor: pointer;
		transition: background-color 0.2s;
	}

	.btn-danger:hover {
		background: rgba(239, 68, 68, 0.35);
	}

	.btn-secondary {
		background: rgba(255, 255, 255, 0.05);
		border: 1px solid rgba(255, 255, 255, 0.1);
		color: var(--text-primary);
		padding: 4px 10px;
		border-radius: 6px;
		font-size: 12px;
		font-weight: 600;
		cursor: pointer;
		transition: background-color 0.2s;
	}

	.btn-secondary:hover {
		background: rgba(255, 255, 255, 0.1);
	}

	.form-group {
		display: flex;
		flex-direction: column;
		gap: 6px;
		margin-bottom: 10px;
	}

	.form-group label {
		font-size: 12px;
		color: var(--text-secondary);
		font-weight: 600;
	}

	.markdown-content {
		font-size: 14px;
		color: var(--text-secondary);
		white-space: pre-wrap;
		line-height: 1.6;
	}

	.tags-row {
		display: flex;
		gap: 6px;
		flex-wrap: wrap;
		margin-top: 8px;
	}

	.tag-pill {
		font-size: 11px;
		padding: 2px 6px;
		background: rgba(255, 255, 255, 0.05);
		color: var(--text-secondary);
		border-radius: 4px;
	}

	.phrases-box {
		margin-top: 10px;
		padding: 10px;
		background: rgba(0, 0, 0, 0.2);
		border-radius: 6px;
		font-size: 12px;
	}

	.phrases-box ul {
		margin: 5px 0 0 16px;
		padding: 0;
	}

	.phrases-box li {
		color: var(--text-secondary);
		font-style: italic;
	}

	.card-footer {
		margin-top: auto;
		border-top: 1px solid rgba(255, 255, 255, 0.05);
		padding-top: 8px;
		font-size: 11px;
		color: var(--text-secondary);
	}

	.source-tag {
		text-transform: capitalize;
	}
</style>
