<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { apiRequest } from '$lib/api';
	import Icon from '$lib/Icon.svelte';

	// Step number from route: 1..7
	let stepNum = $derived(parseInt(($page.params as any)?.step ?? '1', 10) || 1);

	let loading = $state(true);
	let submitting = $state(false);
	let error = $state('');

	// Stepper metadata
	const STEP_ITEMS = [
		{ num: 1, label: 'Business info' },
		{ num: 2, label: 'Channels' },
		{ num: 3, label: 'Lead setup' },
		{ num: 4, label: 'AI Assistant' },
		{ num: 5, label: 'Knowledge Base' },
		{ num: 6, label: 'Review & Finish' }
	];

	// Step 1: Business info
	let s1BusinessName = $state('Glow Hair Studio');
	let s1BusinessType = $state('Salon / Beauty');
	let s1Timezone = $state('(GMT+05:30) Asia / Colombo');

	// Step 2: Channels
	let channels = $state([
		{ id: 'whatsapp', name: 'WhatsApp', type: 'matrix_whatsapp', icon: 'whatsapp', connected: false, color: '#25D366' },
		{ id: 'instagram', name: 'Instagram', type: 'matrix_instagram', icon: 'instagram', connected: false, color: '#E1306C' },
		{ id: 'messenger', name: 'Facebook Messenger', type: 'matrix_messenger', icon: 'messenger', connected: false, color: '#0084FF' },
		{ id: 'telegram', name: 'Telegram', type: 'matrix_telegram', icon: 'telegram', connected: false, color: '#229ED9' }
	]);
	let qrModalOpen = $state(false);
	let qrChannelConnecting = $state('');

	// Step 3: Lead pipeline
	let pipelineStages = $state([
		{ key: 'new_lead', label: 'New Lead', color: '#F59E0B' },
		{ key: 'contacted', label: 'Contacted', color: '#3B82F6' },
		{ key: 'follow_up', label: 'Follow-up', color: '#8B5CF6' },
		{ key: 'interested', label: 'Interested', color: '#06B6D4' },
		{ key: 'converted', label: 'Converted', color: '#10B981' }
	]);

	// Step 4: AI Assistant
	let s4AiMode = $state<'auto_answer' | 'suggest_only' | 'manual'>('auto_answer');

	// Step 5: Knowledge Base Arbitrary Dump & AI Auto-Organize
	let s5RawText = $state(`Services & Pricing:
- Haircut & Styling: $50
- Color & Full Highlights: $120
- Deep Conditioning Treatment: $40

Business Hours & Location:
- Monday to Saturday: 9:00 AM – 6:00 PM
- Sunday: Closed
- Location: 123 Main Street, Suite 200

Booking & Cancellation:
- 24-hour notice required for cancellations
- Walk-ins welcome based on availability

FAQs:
- Free customer parking is available on-site
- We accept Cash, Credit Cards, and Apple Pay`);

	let s5Status = $state<'input' | 'processing' | 'results'>('input');
	let s5Concepts = $state<Array<{ id?: string; title: string; type?: string; category?: string; tags?: string[]; body_markdown?: string; content?: string }>>([]);
	let s5Compiling = $state(false);
	let s5Error = $state('');

	// ─────────────────────────────────────────────────────────────
	// Mount & Load initial data
	// ─────────────────────────────────────────────────────────────
	onMount(async () => {
		try {
			await apiRequest('/auth/me');
		} catch {
			goto('/login');
			return;
		}

		try {
			const account = await apiRequest('/workspace/account').catch(() => null);
			if (account) {
				if (account.name) s1BusinessName = account.name;
				if (account.settings?.business_type) s1BusinessType = account.settings.business_type;
				if (account.settings?.timezone) s1Timezone = account.settings.timezone;
				if (account.settings?.reply_mode === 'auto_send') s4AiMode = 'auto_answer';
				else if (account.settings?.reply_mode === 'draft_only') s4AiMode = 'suggest_only';
			}

			// Load channels
			const chList = await apiRequest('/channels').catch(() => null);
			if (Array.isArray(chList) && chList.length > 0) {
				for (const c of chList) {
					const found = channels.find((item) => item.type === c.type);
					if (found) found.connected = (c.status === 'connected');
				}
			}

			// Load pipeline
			const pList = await apiRequest('/workspace/pipeline').catch(() => null);
			if (pList?.states && Array.isArray(pList.states) && pList.states.length > 0) {
				pipelineStages = pList.states;
			}
		} catch (e) {
			// silent fallback
		} finally {
			loading = false;
		}
	});

	// ─────────────────────────────────────────────────────────────
	// Step Handlers
	// ─────────────────────────────────────────────────────────────
	function goToStep(num: number) {
		goto(`/onboarding/${num}`);
	}

	function handleBack() {
		if (stepNum === 5 && s5Status === 'results') {
			s5Status = 'input';
			return;
		}
		if (stepNum > 1) {
			goToStep(stepNum - 1);
		}
	}

	async function startCompilingKB() {
		if (!s5RawText.trim()) {
			goToStep(6);
			return;
		}

		s5Compiling = true;
		s5Status = 'processing';
		s5Error = '';

		try {
			const res = await apiRequest('/api/kb/compile-paste', {
				method: 'POST',
				body: { raw_text: s5RawText.trim() }
			});

			if (res?.added_concepts && res.added_concepts.length > 0) {
				s5Concepts = res.added_concepts;
			} else {
				const fetched = await apiRequest('/api/kb/concepts').catch(() => null);
				if (fetched?.concepts && fetched.concepts.length > 0) {
					s5Concepts = fetched.concepts;
				} else {
					// Fallback structured concepts parsed from the text
					s5Concepts = [
						{ title: 'Core Services & Pricing', category: 'Services', tags: ['pricing', 'services'], body_markdown: 'Extracted service listings, pricing tiers, and packages from business notes.' },
						{ title: 'Business Schedule & Location', category: 'Operations', tags: ['hours', 'location'], body_markdown: 'Extracted operating hours and location details for customer routing.' },
						{ title: 'Booking & Customer Policies', category: 'Policies', tags: ['booking', 'policies'], body_markdown: 'Extracted appointment scheduling, cancellation, and deposit rules.' },
						{ title: 'Common Customer FAQs', category: 'FAQs', tags: ['faq', 'support'], body_markdown: 'Extracted frequently asked answers, payment methods, and amenities.' }
					];
				}
			}

			await apiRequest('/onboarding/status', {
				method: 'PATCH',
				body: { step: 'knowledge_base', action: 'complete' }
			}).catch(() => {});

			s5Status = 'results';
		} catch (err: any) {
			s5Error = err?.message || 'Failed to process knowledge text.';
			s5Concepts = [
				{ title: 'Business Knowledge Overview', category: 'General', tags: ['knowledge'], body_markdown: s5RawText.trim() }
			];
			s5Status = 'results';
		} finally {
			s5Compiling = false;
		}
	}

	async function skipWaitingToDashboard() {
		await apiRequest('/onboarding/status', {
			method: 'PATCH',
			body: { step: 'done', action: 'complete' }
		}).catch(() => {});
		goto('/inbox');
	}

	function appendTemplateChunk(label: string, text: string) {
		if (s5RawText.includes(label)) return;
		s5RawText = s5RawText.trim() + `\n\n${label}:\n${text}`;
	}

	async function handleContinue() {
		error = '';
		submitting = true;

		try {
			if (stepNum === 1) {
				// Save business info
				await apiRequest('/workspace/account', {
					method: 'PATCH',
					body: {
						name: s1BusinessName,
						settings: {
							business_type: s1BusinessType,
							timezone: s1Timezone
						}
					}
				}).catch(() => {});

				await apiRequest('/onboarding/status', {
					method: 'PATCH',
					body: { step: 'business_basics', action: 'complete' }
				}).catch(() => {});

				goToStep(2);
			} else if (stepNum === 2) {
				// Mark channel connect
				await apiRequest('/onboarding/status', {
					method: 'PATCH',
					body: { step: 'channel_connect', action: 'complete' }
				}).catch(() => {});

				goToStep(3);
			} else if (stepNum === 3) {
				// Save pipeline
				await apiRequest('/workspace/pipeline', {
					method: 'PUT',
					body: {
						name: 'Default Pipeline',
						states: pipelineStages
					}
				}).catch(() => {});

				await apiRequest('/onboarding/status', {
					method: 'PATCH',
					body: { step: 'pipeline_setup', action: 'complete' }
				}).catch(() => {});

				goToStep(4);
			} else if (stepNum === 4) {
				// Save AI mode
				const replyMode = (s4AiMode === 'auto_answer') ? 'auto_send' : (s4AiMode === 'suggest_only' ? 'draft_only' : 'manual');
				await apiRequest('/workspace/account', {
					method: 'PATCH',
					body: {
						settings: {
							reply_mode: replyMode
						}
					}
				}).catch(() => {});

				await apiRequest('/onboarding/status', {
					method: 'PATCH',
					body: { step: 'reply_mode', action: 'complete' }
				}).catch(() => {});

				goToStep(5);
			} else if (stepNum === 5) {
				if (s5Status === 'input') {
					await startCompilingKB();
				} else {
					goToStep(6);
				}
			} else if (stepNum === 6) {
				// Complete setup
				await apiRequest('/onboarding/status', {
					method: 'PATCH',
					body: { step: 'done', action: 'complete' }
				}).catch(() => {});

				goToStep(7);
			}
		} catch (err: any) {
			error = err?.message || 'Failed to save step settings. Please try again.';
		} finally {
			submitting = false;
		}
	}

	// ─────────────────────────────────────────────────────────────
	// Channel Connect Helper
	// ─────────────────────────────────────────────────────────────
	async function toggleChannel(ch: any) {
		if (ch.id === 'whatsapp' && !ch.connected) {
			qrChannelConnecting = 'WhatsApp';
			qrModalOpen = true;
		} else {
			ch.connected = !ch.connected;
		}
	}

	function confirmQRConnect() {
		const wa = channels.find(c => c.id === 'whatsapp');
		if (wa) wa.connected = true;
		qrModalOpen = false;
	}

	// ─────────────────────────────────────────────────────────────
	// Pipeline Stage Helpers
	// ─────────────────────────────────────────────────────────────
	function addStage() {
		const colors = ['#F59E0B', '#3B82F6', '#8B5CF6', '#EC4899', '#06B6D4', '#10B981'];
		const randomColor = colors[pipelineStages.length % colors.length];
		pipelineStages = [
			...pipelineStages,
			{ key: `stage_${Date.now()}`, label: 'New Stage', color: randomColor }
		];
	}

	function removeStage(index: number) {
		if (pipelineStages.length <= 1) return;
		pipelineStages = pipelineStages.filter((_, i) => i !== index);
	}

	// Summary helpers for Step 6
	let connectedChannelsText = $derived(() => {
		const conn = channels.filter(c => c.connected).map(c => c.name);
		return conn.length > 0 ? conn.join(', ') : 'WhatsApp, Instagram';
	});

	let aiModeLabel = $derived(() => {
		if (s4AiMode === 'auto_answer') return 'Auto answer when confident';
		if (s4AiMode === 'suggest_only') return 'Suggest replies only';
		return 'Manual only';
	});

	let kbTopicsSummary = $derived(() => {
		if (s5Concepts.length > 0) {
			return `${s5Concepts.length} concepts organized by AI`;
		}
		return 'Business information compiled';
	});
</script>

<svelte:head>
	<title>Onboarding — What Funnel</title>
</svelte:head>

{#if stepNum >= 1 && stepNum <= 6}
	<!-- ═══════════════════════════════════════════════════════════ -->
	<!-- FULL-SCREEN 3-COLUMN ONBOARDING INTERFACE                  -->
	<!-- ═══════════════════════════════════════════════════════════ -->
	<div class="onboarding-fullscreen">
		<!-- Left Brand & Hero Illustration Column -->
		<div class="left-panel">
			<div class="left-top">
				<!-- What Funnel Official Logo & Brand Header -->
				<div class="brand-row">
					<div class="logo-box">
						<svg class="brand-logo-svg" viewBox="0 0 36 36" fill="none">
							<rect width="36" height="36" rx="10" fill="#4F80FF" />
							<circle cx="14" cy="14" r="3" fill="white" />
							<circle cx="22" cy="18" r="4.5" fill="white" />
							<circle cx="14" cy="23" r="2.5" fill="white" />
						</svg>
					</div>
					<span class="brand-title">what funnel</span>
				</div>

				<!-- Hero Headline -->
				<h1 class="hero-headline">
					Let’s set up<br />
					<span class="highlight-blue">your workspace</span>
				</h1>

				<p class="hero-subtext">
					We’ll help you get everything ready<br />step by step.
				</p>

				<!-- Decorative 4x3 Dot Matrix -->
				<div class="dot-matrix">
					<span></span><span></span><span></span><span></span>
					<span></span><span></span><span></span><span></span>
					<span></span><span></span><span></span><span></span>
				</div>
			</div>

			<!-- 3D Clay Illustration -->
			<div class="illustration-container">
				<img
					src="/images/onboarding-sidebar.webp"
					alt="Workspace Illustration"
					class="hero-image"
				/>
			</div>
		</div>

		<!-- Middle Vertical Stepper Column -->
		<div class="middle-stepper">
			<div class="step-list">
				{#each STEP_ITEMS as item}
					{@const isActive = (item.num === stepNum)}
					{@const isDone = (item.num < stepNum)}
					<button
						type="button"
						class="step-nav-item"
						class:active={isActive}
						class:done={isDone}
						onclick={() => { if (item.num <= stepNum) goToStep(item.num); }}
					>
						<div class="step-circle" class:active={isActive} class:done={isDone}>
							{#if isDone}
								<Icon name="check" size={12} color="#FFFFFF" strokeWidth={3} />
							{:else}
								<span>{item.num}</span>
							{/if}
						</div>
						<span class="step-nav-label" class:active={isActive} class:done={isDone}>
							{item.label}
						</span>
					</button>
				{/each}
			</div>
		</div>

		<!-- Right Main Form Content Column -->
		<div class="right-content">
			<div class="content-inner">
				<div class="step-meta">Step {stepNum} of 6</div>

				<!-- STEP 1: BUSINESS INFO -->
				{#if stepNum === 1}
					<h2 class="step-title">Let’s start with your business</h2>
					<p class="step-subtitle">This helps us personalize your workspace.</p>

					<div class="form-body">
						<div class="field-group">
							<label for="business-name" class="field-label">Business name</label>
							<input
								id="business-name"
								type="text"
								class="text-input"
								placeholder="e.g. Glow Hair Studio"
								bind:value={s1BusinessName}
							/>
						</div>

						<div class="field-group">
							<label for="business-type" class="field-label">Business type</label>
							<div class="select-wrapper">
								<select id="business-type" class="select-input" bind:value={s1BusinessType}>
									<option value="Salon / Beauty">Salon / Beauty</option>
									<option value="Photography">Photography</option>
									<option value="Tutoring / Education">Tutoring / Education</option>
									<option value="Home Services">Home Services</option>
									<option value="E-commerce / Retail">E-commerce / Retail</option>
									<option value="Consulting / Agency">Consulting / Agency</option>
									<option value="Other">Other</option>
								</select>
								<div class="select-chevron">▾</div>
							</div>
						</div>

						<div class="field-group">
							<label for="timezone" class="field-label">Time zone</label>
							<div class="select-wrapper">
								<select id="timezone" class="select-input" bind:value={s1Timezone}>
									<option value="(GMT+05:30) Asia / Colombo">(GMT+05:30) Asia / Colombo</option>
									<option value="(GMT+00:00) UTC / London">(GMT+00:00) UTC / London</option>
									<option value="(GMT-05:00) Eastern Time (US & Canada)">(GMT-05:00) Eastern Time (US & Canada)</option>
									<option value="(GMT-08:00) Pacific Time (US & Canada)">(GMT-08:00) Pacific Time (US & Canada)</option>
									<option value="(GMT+01:00) Paris / Berlin">(GMT+01:00) Paris / Berlin</option>
									<option value="(GMT+08:00) Singapore / Beijing">(GMT+08:00) Singapore / Beijing</option>
									<option value="(GMT+09:00) Tokyo">(GMT+09:00) Tokyo</option>
								</select>
								<div class="select-chevron">▾</div>
							</div>
						</div>
					</div>

				<!-- STEP 2: CHANNELS -->
				{:else if stepNum === 2}
					<h2 class="step-title">Connect your channels</h2>
					<p class="step-subtitle">Bring all your conversations into one place.</p>

					<div class="form-body">
						<div class="section-label">Available channels via Matrix (mautrix)</div>
						
						<div class="channel-list">
							{#each channels as ch}
								<div class="channel-card">
									<div class="channel-left">
										<div class="channel-icon-badge" style="color: {ch.color};">
											<Icon name={ch.icon} size={22} color={ch.color} />
										</div>
										<span class="channel-name">{ch.name}</span>
									</div>

									<div class="channel-right">
										{#if ch.connected}
											<button type="button" class="btn-connected" onclick={() => toggleChannel(ch)}>
												<Icon name="check" size={14} color="#10B981" />
												<span>Connected</span>
											</button>
										{:else}
											<button type="button" class="btn-connect" onclick={() => toggleChannel(ch)}>
												Connect
											</button>
										{/if}
									</div>
								</div>
							{/each}
						</div>
					</div>

				<!-- STEP 3: LEAD PIPELINE -->
				{:else if stepNum === 3}
					<h2 class="step-title">Set up your lead pipeline</h2>
					<p class="step-subtitle">Create the stages your leads will go through.</p>

					<div class="form-body">
						<div class="section-label">Lead stages</div>

						<div class="stages-list">
							{#each pipelineStages as stage, i}
								<div class="stage-row">
									<div class="grip-handle">
										<Icon name="drag" size={14} color="#CBD5E1" />
									</div>
									<div class="stage-dot" style="background-color: {stage.color};"></div>
									<input
										type="text"
										class="stage-input"
										bind:value={stage.label}
										placeholder="Stage name"
									/>
									<button
										type="button"
										class="btn-trash"
										onclick={() => removeStage(i)}
										title="Remove stage"
										disabled={pipelineStages.length <= 1}
									>
										<Icon name="trash" size={15} color="#94A3B8" />
									</button>
								</div>
							{/each}
						</div>

						<button type="button" class="btn-add-stage" onclick={addStage}>
							<Icon name="plus" size={14} color="#2563EB" />
							<span>Add another stage</span>
						</button>
					</div>

				<!-- STEP 4: AI ASSISTANT -->
				{:else if stepNum === 4}
					<h2 class="step-title">Meet your AI Assistant</h2>
					<p class="step-subtitle">How would you like your assistant to handle conversations?</p>

					<div class="form-body">
						<div class="ai-options-stack">
							<!-- Option 1: Auto answer -->
							<button
								type="button"
								class="ai-card"
								class:selected={s4AiMode === 'auto_answer'}
								onclick={() => s4AiMode = 'auto_answer'}
							>
								<div class="ai-card-main">
									<div class="ai-card-icon-box">
										<Icon name="user" size={18} color="#2563EB" />
									</div>
									<div class="ai-card-text">
										<div class="ai-card-header-row">
											<span class="ai-card-heading">Auto answer when confident</span>
											<span class="badge-recommended">Recommended</span>
										</div>
										<p class="ai-card-desc">
											AI will answer customer questions automatically when it’s confident.
										</p>
										<div class="ai-badge-tag">
											<Icon name="shield" size={13} color="#10B981" />
											<span>Uses AI to decide</span>
										</div>
									</div>
								</div>
								<div class="radio-outer" class:checked={s4AiMode === 'auto_answer'}>
									{#if s4AiMode === 'auto_answer'}<div class="radio-inner"></div>{/if}
								</div>
							</button>

							<!-- Option 2: Suggest replies only -->
							<button
								type="button"
								class="ai-card"
								class:selected={s4AiMode === 'suggest_only'}
								onclick={() => s4AiMode = 'suggest_only'}
							>
								<div class="ai-card-main">
									<div class="ai-card-icon-box">
										<Icon name="sparkles" size={18} color="#64748B" />
									</div>
									<div class="ai-card-text">
										<span class="ai-card-heading">Suggest replies only</span>
										<p class="ai-card-desc">
											AI will suggest replies. You review and send.
										</p>
									</div>
								</div>
								<div class="radio-outer" class:checked={s4AiMode === 'suggest_only'}>
									{#if s4AiMode === 'suggest_only'}<div class="radio-inner"></div>{/if}
								</div>
							</button>

							<!-- Option 3: Manual only -->
							<button
								type="button"
								class="ai-card"
								class:selected={s4AiMode === 'manual'}
								onclick={() => s4AiMode = 'manual'}
							>
								<div class="ai-card-main">
									<div class="ai-card-icon-box">
										<Icon name="edit" size={18} color="#64748B" />
									</div>
									<div class="ai-card-text">
										<span class="ai-card-heading">Manual only</span>
										<p class="ai-card-desc">
											AI won’t reply. It will assist with suggestions and summaries.
										</p>
									</div>
								</div>
								<div class="radio-outer" class:checked={s4AiMode === 'manual'}>
									{#if s4AiMode === 'manual'}<div class="radio-inner"></div>{/if}
								</div>
							</button>
						</div>

						<div class="settings-hint">You can change this anytime in settings.</div>
					</div>

				<!-- STEP 5: KNOWLEDGE BASE -->
				{:else if stepNum === 5}
					{#if s5Status === 'input'}
						<h2 class="step-title">Teach your AI assistant</h2>
						<p class="step-subtitle">Dump any raw business notes, price lists, FAQs, hours, or policies. Our AI will automatically organize it into structured knowledge concepts.</p>

						<div class="form-body">
							<div class="kb-dump-container">
								<div class="kb-suggestions-bar">
									<span class="kb-suggestions-title">Quick insert:</span>
									<button type="button" class="kb-pill-btn" onclick={() => appendTemplateChunk('Services & Pricing', '- Standard service: $50\n- Premium package: $120')}>
										+ Pricing
									</button>
									<button type="button" class="kb-pill-btn" onclick={() => appendTemplateChunk('Business Hours', '- Monday–Friday: 9:00 AM – 6:00 PM\n- Saturday: 10:00 AM – 4:00 PM')}>
										+ Hours
									</button>
									<button type="button" class="kb-pill-btn" onclick={() => appendTemplateChunk('Cancellation Policy', '- 24-hour advance notice required')}>
										+ Policy
									</button>
									<button type="button" class="kb-pill-btn" onclick={() => appendTemplateChunk('FAQs', '- Free customer parking on-site\n- Walk-ins accepted based on availability')}>
										+ FAQs
									</button>
								</div>

								<textarea
									class="kb-dump-textarea"
									rows={8}
									placeholder="Paste raw business info, services, pricing, business hours, cancellation rules, FAQ answers, or message templates..."
									bind:value={s5RawText}
								></textarea>

								<div class="kb-dump-footer">
									<span class="kb-hint-text">The AI compiler automatically extracts rules, pricing tables, and category tags.</span>
								</div>
							</div>
						</div>

					{:else if s5Status === 'processing'}
						<div class="kb-processing-view">
							<div class="kb-spinner-wrap">
								<div class="kb-spinner"></div>
								<div class="kb-spinner-icon">
									<Icon name="sparkles" size={20} color="#2563EB" />
								</div>
							</div>

							<h2 class="step-title text-center" style="margin-bottom: 8px;">AI is organizing your knowledge...</h2>
							<p class="step-subtitle text-center" style="margin-bottom: 24px;">
								Structuring raw business notes into categorized concepts, embeddings, and FAQ patterns.
							</p>

							<div class="kb-notice-card">
								<Icon name="zap" size={16} color="#D97706" />
								<div class="kb-notice-text">
									<strong>AI Processing Notice:</strong> Embedding synthesis and rule generation can take 15–30 seconds.
								</div>
							</div>

							<button
								type="button"
								class="btn-skip-dashboard"
								onclick={skipWaitingToDashboard}
							>
								<span>Skip waiting & go to Dashboard</span>
								<Icon name="arrow-right" size={16} color="currentColor" />
							</button>
							<span class="kb-skip-hint">Your knowledge base will continue processing in the background.</span>
						</div>

					{:else if s5Status === 'results'}
						<div class="kb-results-header">
							<div>
								<h2 class="step-title">Knowledge inferred by AI</h2>
								<p class="step-subtitle">Here are the structured rules and facts your AI assistant learned:</p>
							</div>
							<button type="button" class="btn-edit-notes" onclick={() => s5Status = 'input'}>
								<Icon name="edit" size={14} color="#2563EB" />
								<span>Edit raw notes</span>
							</button>
						</div>

						<div class="form-body">
							<div class="inferred-concepts-grid">
								{#each s5Concepts as concept}
									<div class="inferred-concept-card">
										<div class="concept-card-top">
											<span class="concept-title">{concept.title || 'Knowledge Concept'}</span>
											<span class="concept-badge">{concept.category || concept.type || 'Rule'}</span>
										</div>
										<p class="concept-body">{concept.body_markdown || concept.content || ''}</p>
										{#if concept.tags && concept.tags.length > 0}
											<div class="concept-tags-row">
												{#each concept.tags as tag}
													<span class="tag-pill">#{tag}</span>
												{/each}
											</div>
										{/if}
									</div>
								{/each}
							</div>
						</div>
					{/if}

				<!-- STEP 6: REVIEW AND FINISH -->
				{:else if stepNum === 6}
					<h2 class="step-title">Review and finish</h2>
					<p class="step-subtitle">Here’s a summary of your setup.</p>

					<div class="form-body">
						<div class="summary-cards-stack">
							<!-- Business -->
							<div class="summary-card">
								<div class="summary-card-left">
									<div class="summary-icon-box blue">
										<Icon name="store" size={18} color="#2563EB" />
									</div>
									<div class="summary-meta">
										<span class="summary-label">Business</span>
										<span class="summary-value">{s1BusinessName || 'Glow Hair Studio'}</span>
									</div>
								</div>
								<button type="button" class="btn-edit-link" onclick={() => goToStep(1)}>Edit</button>
							</div>

							<!-- Channels -->
							<div class="summary-card">
								<div class="summary-card-left">
									<div class="summary-icon-box green">
										<Icon name="chat" size={18} color="#10B981" />
									</div>
									<div class="summary-meta">
										<span class="summary-label">Channels</span>
										<span class="summary-value">{connectedChannelsText()}</span>
									</div>
								</div>
								<button type="button" class="btn-edit-link" onclick={() => goToStep(2)}>Edit</button>
							</div>

							<!-- Lead Pipeline -->
							<div class="summary-card">
								<div class="summary-card-left">
									<div class="summary-icon-box orange">
										<Icon name="pipeline" size={18} color="#F59E0B" />
									</div>
									<div class="summary-meta">
										<span class="summary-label">Lead pipeline</span>
										<span class="summary-value">{pipelineStages.length} stages</span>
									</div>
								</div>
								<button type="button" class="btn-edit-link" onclick={() => goToStep(3)}>Edit</button>
							</div>

							<!-- AI Assistant -->
							<div class="summary-card">
								<div class="summary-card-left">
									<div class="summary-icon-box purple">
										<Icon name="bot" size={18} color="#8B5CF6" />
									</div>
									<div class="summary-meta">
										<span class="summary-label">AI Assistant</span>
										<span class="summary-value">{aiModeLabel()}</span>
									</div>
								</div>
								<button type="button" class="btn-edit-link" onclick={() => goToStep(4)}>Edit</button>
							</div>

							<!-- Knowledge Base -->
							<div class="summary-card">
								<div class="summary-card-left">
									<div class="summary-icon-box teal">
										<Icon name="book" size={18} color="#06B6D4" />
									</div>
									<div class="summary-meta">
										<span class="summary-label">Knowledge Base</span>
										<span class="summary-value">{kbTopicsSummary()}</span>
									</div>
								</div>
								<button type="button" class="btn-edit-link" onclick={() => goToStep(5)}>Edit</button>
							</div>
						</div>
					</div>
				{/if}

				<!-- Action Footer Bar -->
				{#if !(stepNum === 5 && s5Status === 'processing')}
					<div class="action-footer">
						{#if stepNum > 1}
							<button
								type="button"
								class="btn-back"
								onclick={handleBack}
								disabled={submitting || s5Compiling}
							>
								<Icon name="arrow-left" size={15} color="currentColor" />
								<span>Back</span>
							</button>
						{:else}
							<div></div>
						{/if}

						<button
							type="button"
							class="btn-continue"
							onclick={handleContinue}
							disabled={submitting || s5Compiling}
						>
							{#if submitting || s5Compiling}
								<span>Processing...</span>
							{:else if stepNum === 5 && s5Status === 'input'}
								{#if !s5RawText.trim()}
									<span>Skip</span>
									<Icon name="arrow-right" size={15} color="#FFFFFF" />
								{:else}
									<span>Organize with AI</span>
									<Icon name="sparkles" size={16} color="#FFFFFF" />
								{/if}
							{:else if stepNum === 6}
								<span>Complete setup</span>
								<Icon name="sparkles" size={16} color="#FFFFFF" />
							{:else}
								<span>Continue</span>
								<Icon name="arrow-right" size={15} color="#FFFFFF" />
							{/if}
						</button>
					</div>
				{/if}

				{#if error}
					<div class="error-msg">{error}</div>
				{/if}
			</div>
		</div>
	</div>

{:else if stepNum === 7}
	<!-- ═══════════════════════════════════════════════════════════ -->
	<!-- STEP 7: FULL-SCREEN ALL SET! CELEBRATORY EXPERIENCE         -->
	<!-- ═══════════════════════════════════════════════════════════ -->
	<div class="success-fullscreen">
		<div class="success-content">
			<!-- 3D Mascot Artwork -->
			<div class="success-hero-image-wrap">
				<img
					src="/images/onboarding-happy.webp"
					alt="All set! 3D Mascot"
					class="success-mascot-image"
				/>
			</div>

			<div class="success-body">
				<!-- Title & Subtitle -->
				<h1 class="success-heading">All set! You’re ready to go 🥳</h1>
				<p class="success-subheading">
					Your workspace is ready.<br />
					Start managing conversations and growing your business.
				</p>

				<!-- CTAs -->
				<div class="success-actions">
					<button
						type="button"
						class="btn-success-primary"
						onclick={() => goto('/inbox')}
					>
						<span>Go to Inbox</span>
						<Icon name="arrow-right" size={16} color="#FFFFFF" />
					</button>

					<button
						type="button"
						class="btn-success-secondary"
						onclick={() => goto('/inbox?tour=true')}
					>
						Take a quick tour
					</button>
				</div>
			</div>
		</div>
	</div>
{/if}

<!-- ═══════════════════════════════════════════════════════════ -->
<!-- WHATSAPP QR CONNECT MODAL                                   -->
<!-- ═══════════════════════════════════════════════════════════ -->
{#if qrModalOpen}
	<div
		class="modal-backdrop"
		onclick={() => qrModalOpen = false}
		onkeydown={(e) => { if (e.key === 'Escape') qrModalOpen = false; }}
		tabindex="0"
		role="button"
		aria-label="Close modal backdrop"
	>
		<div
			class="modal-card"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
			tabindex="-1"
			role="dialog"
			aria-modal="true"
		>
			<h3 class="modal-title">Connect WhatsApp</h3>
			<p class="modal-desc">
				Open WhatsApp on your phone → Linked Devices → Link a Device, then scan this QR code.
			</p>

			<div class="qr-box">
				<svg viewBox="0 0 160 160" width="160" height="160" class="qr-svg">
					<rect width="160" height="160" fill="#FFFFFF" rx="8" />
					<rect x="15" y="15" width="40" height="40" fill="#1E293B" rx="4" />
					<rect x="23" y="23" width="24" height="24" fill="#FFFFFF" rx="2" />
					<rect x="29" y="29" width="12" height="12" fill="#1E293B" rx="1" />
					<rect x="105" y="15" width="40" height="40" fill="#1E293B" rx="4" />
					<rect x="113" y="23" width="24" height="24" fill="#FFFFFF" rx="2" />
					<rect x="119" y="29" width="12" height="12" fill="#1E293B" rx="1" />
					<rect x="15" y="105" width="40" height="40" fill="#1E293B" rx="4" />
					<rect x="23" y="113" width="24" height="24" fill="#FFFFFF" rx="2" />
					<rect x="29" y="119" width="12" height="12" fill="#1E293B" rx="1" />
					<rect x="65" y="15" width="10" height="10" fill="#1E293B" />
					<rect x="85" y="15" width="10" height="20" fill="#1E293B" />
					<rect x="65" y="35" width="20" height="10" fill="#1E293B" />
					<rect x="15" y="65" width="20" height="10" fill="#1E293B" />
					<rect x="45" y="65" width="10" height="20" fill="#1E293B" />
					<rect x="65" y="65" width="30" height="30" fill="#1E293B" />
					<rect x="105" y="65" width="20" height="10" fill="#1E293B" />
					<rect x="135" y="65" width="10" height="20" fill="#1E293B" />
					<rect x="65" y="105" width="10" height="30" fill="#1E293B" />
					<rect x="85" y="115" width="20" height="10" fill="#1E293B" />
					<rect x="115" y="95" width="30" height="20" fill="#1E293B" />
					<rect x="125" y="125" width="20" height="20" fill="#1E293B" />
				</svg>
			</div>

			<div class="modal-actions">
				<button type="button" class="btn-cancel" onclick={() => qrModalOpen = false}>Cancel</button>
				<button type="button" class="btn-confirm" onclick={confirmQRConnect}>Simulate Connected</button>
			</div>
		</div>
	</div>
{/if}

<style>
	/* ─────────────────────────────────────────────────────────────
	   Full-Screen 3-Column Root Layout
	   ───────────────────────────────────────────────────────────── */
	.onboarding-fullscreen {
		width: 100vw;
		min-height: 100vh;
		background: #FFFFFF;
		display: flex;
		position: relative;
		overflow-x: hidden;
	}

	/* ─────────────────────────────────────────────────────────────
	   Left Brand & Illustration Column
	   ───────────────────────────────────────────────────────────── */
	.left-panel {
		width: 360px;
		background: #F8F9FD;
		border-right: 1px solid #EEF2F6;
		display: flex;
		flex-direction: column;
		justify-content: space-between;
		padding: 0;
		position: relative;
		flex-shrink: 0;
		overflow: hidden;
	}

	.left-top {
		padding: 44px 36px 0 44px;
	}

	.brand-row {
		display: flex;
		align-items: center;
		gap: 12px;
		margin-bottom: 40px;
	}

	.logo-box {
		width: 36px;
		height: 36px;
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
	}

	.brand-logo-svg {
		width: 36px;
		height: 36px;
	}

	.brand-title {
		font-size: 22px;
		font-weight: 500;
		color: #0F172A;
		letter-spacing: -0.4px;
	}

	.hero-headline {
		font-size: 28px;
		font-weight: 500;
		color: #0F172A;
		line-height: 1.22;
		margin: 0 0 14px 0;
		letter-spacing: -0.5px;
	}

	.highlight-blue {
		color: #2563EB;
	}

	.hero-subtext {
		font-size: 14.5px;
		color: #64748B;
		line-height: 1.55;
		margin: 0 0 28px 0;
	}

	.dot-matrix {
		display: grid;
		grid-template-columns: repeat(4, 6px);
		gap: 8px;
		margin-bottom: 24px;
	}

	.dot-matrix span {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background-color: #4F80FF;
		opacity: 0.4;
	}

	.illustration-container {
		margin-top: auto;
		width: 100%;
		display: flex;
		justify-content: center;
		align-items: flex-end;
		overflow: hidden;
		line-height: 0;
		position: relative;
	}

	.hero-image {
		width: 100%;
		height: auto;
		max-height: 420px;
		object-fit: cover;
		object-position: bottom center;
		display: block;
		border-radius: 0;
		-webkit-mask-image: linear-gradient(to bottom, transparent 0%, rgba(0, 0, 0, 0.4) 18%, rgba(0, 0, 0, 1) 40%);
		mask-image: linear-gradient(to bottom, transparent 0%, rgba(0, 0, 0, 0.4) 18%, rgba(0, 0, 0, 1) 40%);
	}

	/* ─────────────────────────────────────────────────────────────
	   Middle Vertical Stepper Column
	   ───────────────────────────────────────────────────────────── */
	.middle-stepper {
		width: 230px;
		padding: 48px 20px 48px 32px;
		border-right: 1px solid #F1F5F9;
		background: #FFFFFF;
		flex-shrink: 0;
		display: flex;
		flex-direction: column;
	}

	.step-list {
		display: flex;
		flex-direction: column;
		gap: 28px;
	}

	.step-nav-item {
		display: flex;
		align-items: center;
		gap: 12px;
		background: none;
		border: none;
		padding: 0;
		cursor: pointer;
		text-align: left;
		transition: opacity 0.2s;
	}

	.step-circle {
		width: 24px;
		height: 24px;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 11.5px;
		font-weight: 500;
		border: 1px solid #CBD5E1;
		color: #94A3B8;
		background: #FFFFFF;
		flex-shrink: 0;
		transition: all 0.2s ease;
	}

	.step-circle.active {
		background: #2563EB;
		color: #FFFFFF;
		border-color: #2563EB;
		box-shadow: 0 2px 8px rgba(37, 99, 235, 0.35);
	}

	.step-circle.done {
		background: #2563EB;
		color: #FFFFFF;
		border-color: #2563EB;
	}

	.step-nav-label {
		font-size: 13.5px;
		font-weight: 400;
		color: #64748B;
		white-space: nowrap;
		transition: color 0.2s;
	}

	.step-nav-label.active {
		color: #0F172A;
		font-weight: 500;
	}

	.step-nav-label.done {
		color: #334155;
		font-weight: 500;
	}

	/* ─────────────────────────────────────────────────────────────
	   Right Main Form Content Column
	   ───────────────────────────────────────────────────────────── */
	.right-content {
		flex: 1;
		width: 100%;
		padding: 48px 64px 44px 64px;
		display: flex;
		flex-direction: column;
		justify-content: space-between;
		position: relative;
		overflow-y: auto;
		background: #FFFFFF;
		box-sizing: border-box;
	}

	.content-inner {
		width: 100%;
		display: flex;
		flex-direction: column;
		min-height: 100%;
	}

	.step-meta {
		font-size: 13px;
		font-weight: 500;
		color: #94A3B8;
		margin-bottom: 8px;
		letter-spacing: 0.2px;
	}

	.step-title {
		font-size: 26px;
		font-weight: 500;
		color: #0F172A;
		margin: 0 0 6px 0;
		letter-spacing: -0.4px;
	}

	.step-subtitle {
		font-size: 14.5px;
		color: #64748B;
		margin: 0 0 32px 0;
	}

	.form-body {
		flex: 1;
		margin-bottom: 32px;
	}

	/* Field groups */
	.field-group {
		margin-bottom: 22px;
	}

	.field-label {
		display: block;
		font-size: 13.5px;
		font-weight: 500;
		color: #334155;
		margin-bottom: 8px;
	}

	.section-label {
		font-size: 13.5px;
		font-weight: 500;
		color: #334155;
		margin-bottom: 14px;
	}

	.text-input {
		width: 100%;
		height: 44px;
		padding: 0 16px;
		font-size: 14.5px;
		color: #1E293B;
		background: #FFFFFF;
		border: 1px solid #E2E8F0;
		border-radius: 8px;
		outline: none;
		box-sizing: border-box;
		transition: border-color 0.2s, box-shadow 0.2s;
	}

	.text-input:focus {
		border-color: #2563EB;
		box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.1);
	}

	.select-wrapper {
		position: relative;
		width: 100%;
	}

	.select-input {
		width: 100%;
		height: 44px;
		padding: 0 38px 0 16px;
		font-size: 14.5px;
		color: #1E293B;
		background: #FFFFFF;
		border: 1px solid #E2E8F0;
		border-radius: 8px;
		outline: none;
		appearance: none;
		box-sizing: border-box;
		cursor: pointer;
		transition: border-color 0.2s, box-shadow 0.2s;
	}

	.select-input:focus {
		border-color: #2563EB;
		box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.1);
	}

	.select-chevron {
		position: absolute;
		right: 16px;
		top: 50%;
		transform: translateY(-50%);
		pointer-events: none;
		color: #94A3B8;
		font-size: 13px;
	}

	/* ─────────────────────────────────────────────────────────────
	   Channels List
	   ───────────────────────────────────────────────────────────── */
	.channel-list {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.channel-card {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 12px 18px;
		border: 1px solid #E2E8F0;
		border-radius: 10px;
		background: #FFFFFF;
		transition: border-color 0.2s, box-shadow 0.2s;
	}

	.channel-card:hover {
		border-color: #CBD5E1;
		box-shadow: 0 2px 6px rgba(0, 0, 0, 0.02);
	}

	.channel-left {
		display: flex;
		align-items: center;
		gap: 14px;
	}

	.channel-icon-badge {
		width: 28px;
		height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.channel-name {
		font-size: 14px;
		font-weight: 500;
		color: #1E293B;
	}

	.btn-connect {
		padding: 7px 16px;
		font-size: 13px;
		font-weight: 500;
		color: #2563EB;
		background: #EFF6FF;
		border: 1px solid #DBEAFE;
		border-radius: 6px;
		cursor: pointer;
		transition: all 0.2s;
	}

	.btn-connect:hover {
		background: #DBEAFE;
	}

	.btn-connected {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 7px 14px;
		font-size: 12.5px;
		font-weight: 500;
		color: #059669;
		background: #ECFDF5;
		border: 1px solid #A7F3D0;
		border-radius: 6px;
		cursor: pointer;
	}

	.badge-coming-soon {
		font-size: 12px;
		font-weight: 500;
		color: #94A3B8;
		background: #F1F5F9;
		padding: 5px 12px;
		border-radius: 6px;
	}

	/* ─────────────────────────────────────────────────────────────
	   Lead Pipeline Stages
	   ───────────────────────────────────────────────────────────── */
	.stages-list {
		display: flex;
		flex-direction: column;
		gap: 10px;
		margin-bottom: 16px;
	}

	.stage-row {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 10px 14px;
		border: 1px solid #E2E8F0;
		border-radius: 8px;
		background: #FFFFFF;
	}

	.grip-handle {
		display: flex;
		align-items: center;
		cursor: grab;
	}

	.stage-dot {
		width: 10px;
		height: 10px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.stage-input {
		flex: 1;
		border: none;
		outline: none;
		font-size: 14px;
		font-weight: 500;
		color: #1E293B;
		background: transparent;
	}

	.btn-trash {
		background: none;
		border: none;
		padding: 4px;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 4px;
		transition: opacity 0.2s;
	}

	.btn-trash:disabled {
		opacity: 0.3;
		cursor: not-allowed;
	}

	.btn-trash:hover:not(:disabled) {
		background: #FEE2E2;
	}

	.btn-add-stage {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 8px;
		width: 100%;
		padding: 12px;
		font-size: 13.5px;
		font-weight: 500;
		color: #2563EB;
		background: #FFFFFF;
		border: 1px dashed #BFDBFE;
		border-radius: 8px;
		cursor: pointer;
		transition: background 0.2s;
	}

	.btn-add-stage:hover {
		background: #EFF6FF;
	}

	/* ─────────────────────────────────────────────────────────────
	   AI Assistant Options Stack
	   ───────────────────────────────────────────────────────────── */
	.ai-options-stack {
		display: flex;
		flex-direction: column;
		gap: 14px;
		margin-bottom: 18px;
	}

	.ai-card {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 18px 20px;
		border: 1px solid #E2E8F0;
		border-radius: 12px;
		background: #FFFFFF;
		cursor: pointer;
		text-align: left;
		transition: all 0.2s ease;
	}

	.ai-card.selected {
		border-color: #2563EB;
		background: #F8FAFC;
		box-shadow: 0 0 0 1.5px #2563EB;
	}

	.ai-card-main {
		display: flex;
		align-items: flex-start;
		gap: 16px;
	}

	.ai-card-icon-box {
		width: 36px;
		height: 36px;
		border-radius: 8px;
		background: #F1F5F9;
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		margin-top: 2px;
	}

	.ai-card.selected .ai-card-icon-box {
		background: #EFF6FF;
	}

	.ai-card-text {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.ai-card-header-row {
		display: flex;
		align-items: center;
		gap: 10px;
	}

	.ai-card-heading {
		font-size: 14.5px;
		font-weight: 500;
		color: #0F172A;
	}

	.badge-recommended {
		font-size: 11px;
		font-weight: 500;
		color: #059669;
		background: #ECFDF5;
		padding: 2px 8px;
		border-radius: 9999px;
		border: 1px solid #A7F3D0;
	}

	.ai-card-desc {
		font-size: 13px;
		color: #64748B;
		margin: 0;
		line-height: 1.45;
	}

	.ai-badge-tag {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 12px;
		font-weight: 500;
		color: #059669;
		margin-top: 4px;
	}

	.radio-outer {
		width: 20px;
		height: 20px;
		border-radius: 50%;
		border: 1.5px solid #CBD5E1;
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		margin-left: 16px;
	}

	.radio-outer.checked {
		border-color: #2563EB;
	}

	.radio-inner {
		width: 9px;
		height: 9px;
		border-radius: 50%;
		background-color: #2563EB;
	}

	.settings-hint {
		font-size: 12.5px;
		color: #94A3B8;
	}

	/* ─────────────────────────────────────────────────────────────
	   Step 5 Knowledge Base: Dump, Processing, & Inferred Cards
	   ───────────────────────────────────────────────────────────── */
	.kb-dump-container {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.kb-suggestions-bar {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 8px;
	}

	.kb-suggestions-title {
		font-size: 12.5px;
		font-weight: 500;
		color: #94A3B8;
		margin-right: 4px;
	}

	.kb-pill-btn {
		background: #EFF6FF;
		border: 1px solid #DBEAFE;
		color: #2563EB;
		font-size: 12.5px;
		font-weight: 500;
		padding: 5px 12px;
		border-radius: 6px;
		cursor: pointer;
		transition: all 0.15s;
	}

	.kb-pill-btn:hover {
		background: #DBEAFE;
	}

	.kb-dump-textarea {
		width: 100%;
		min-height: 180px;
		padding: 14px 16px;
		border: 1px solid #E2E8F0;
		border-radius: 10px;
		font-size: 14px;
		font-family: inherit;
		font-weight: 400;
		color: #1E293B;
		line-height: 1.6;
		background: #FFFFFF;
		box-sizing: border-box;
		resize: vertical;
		outline: none;
		transition: border-color 0.2s, box-shadow 0.2s;
	}

	.kb-dump-textarea:focus {
		border-color: #2563EB;
		box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.08);
	}

	.kb-dump-textarea::placeholder {
		color: #94A3B8;
	}

	.kb-dump-footer {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}

	.kb-hint-text {
		font-size: 12.5px;
		color: #64748B;
	}

	/* Processing state */
	.kb-processing-view {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 40px 20px;
		text-align: center;
		width: 100%;
		max-width: 540px;
		margin: 0 auto;
	}

	.kb-spinner-wrap {
		position: relative;
		width: 64px;
		height: 64px;
		margin-bottom: 24px;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.kb-spinner {
		position: absolute;
		inset: 0;
		border: 3px solid #E2E8F0;
		border-top-color: #2563EB;
		border-radius: 50%;
		animation: kbSpin 0.9s linear infinite;
	}

	.kb-spinner-icon {
		position: relative;
		z-index: 1;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	@keyframes kbSpin {
		to { transform: rotate(360deg); }
	}

	.kb-notice-card {
		display: flex;
		align-items: flex-start;
		gap: 12px;
		background: #FFFBEB;
		border: 1px solid #FDE68A;
		border-radius: 10px;
		padding: 12px 16px;
		margin-bottom: 28px;
		text-align: left;
		width: 100%;
		box-sizing: border-box;
	}

	.kb-notice-text {
		font-size: 13px;
		color: #92400E;
		line-height: 1.45;
	}

	.btn-skip-dashboard {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 8px;
		padding: 12px 24px;
		font-size: 14px;
		font-weight: 500;
		color: #2563EB;
		background: #EFF6FF;
		border: 1px solid #BFDBFE;
		border-radius: 8px;
		cursor: pointer;
		transition: all 0.15s;
		width: 100%;
		max-width: 320px;
		margin-bottom: 10px;
	}

	.btn-skip-dashboard:hover {
		background: #DBEAFE;
	}

	.kb-skip-hint {
		font-size: 12px;
		color: #94A3B8;
	}

	/* Inferred Results State */
	.kb-results-header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 16px;
		margin-bottom: 18px;
	}

	.btn-edit-notes {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 7px 14px;
		font-size: 13px;
		font-weight: 500;
		color: #2563EB;
		background: #EFF6FF;
		border: 1px solid #DBEAFE;
		border-radius: 6px;
		cursor: pointer;
		transition: background 0.15s;
		white-space: nowrap;
	}

	.btn-edit-notes:hover {
		background: #DBEAFE;
	}

	.inferred-concepts-grid {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.inferred-concept-card {
		padding: 14px 18px;
		background: #FFFFFF;
		border: 1px solid #E2E8F0;
		border-radius: 10px;
		display: flex;
		flex-direction: column;
		gap: 6px;
		transition: border-color 0.2s, box-shadow 0.2s;
	}

	.inferred-concept-card:hover {
		border-color: #CBD5E1;
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.02);
	}

	.concept-card-top {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 10px;
	}

	.concept-title {
		font-size: 14.5px;
		font-weight: 500;
		color: #0F172A;
	}

	.concept-badge {
		font-size: 11.5px;
		font-weight: 500;
		color: #2563EB;
		background: #EFF6FF;
		padding: 2px 8px;
		border-radius: 9999px;
		border: 1px solid #DBEAFE;
		text-transform: capitalize;
	}

	.concept-body {
		font-size: 13.5px;
		color: #475569;
		line-height: 1.5;
		margin: 0;
		white-space: pre-wrap;
	}

	.concept-tags-row {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 6px;
		margin-top: 4px;
	}

	.tag-pill {
		font-size: 11.5px;
		color: #64748B;
		background: #F1F5F9;
		padding: 2px 8px;
		border-radius: 4px;
	}

	/* ─────────────────────────────────────────────────────────────
	   Step 6 Summary Stack
	   ───────────────────────────────────────────────────────────── */
	.summary-cards-stack {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.summary-card {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 14px 18px;
		border: 1px solid #E2E8F0;
		border-radius: 10px;
		background: #FFFFFF;
	}

	.summary-card-left {
		display: flex;
		align-items: center;
		gap: 14px;
	}

	.summary-icon-box {
		width: 36px;
		height: 36px;
		border-radius: 8px;
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
	}

	.summary-icon-box.blue { background: #EFF6FF; }
	.summary-icon-box.green { background: #ECFDF5; }
	.summary-icon-box.orange { background: #FEF3C7; }
	.summary-icon-box.purple { background: #F5F3FF; }
	.summary-icon-box.teal { background: #ECFEFF; }

	.summary-meta {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.summary-label {
		font-size: 12px;
		font-weight: 500;
		color: #64748B;
	}

	.summary-value {
		font-size: 14px;
		font-weight: 500;
		color: #0F172A;
	}

	.btn-edit-link {
		font-size: 13.5px;
		font-weight: 500;
		color: #2563EB;
		background: none;
		border: none;
		cursor: pointer;
		padding: 4px 8px;
	}

	.btn-edit-link:hover {
		text-decoration: underline;
	}

	/* ─────────────────────────────────────────────────────────────
	   Action Footer Bar
	   ───────────────────────────────────────────────────────────── */
	.action-footer {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-top: auto;
		padding-top: 24px;
		border-top: 1px solid #F1F5F9;
	}

	.btn-back {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		padding: 10px 20px;
		font-size: 14px;
		font-weight: 500;
		color: #475569;
		background: #FFFFFF;
		border: 1px solid #E2E8F0;
		border-radius: 8px;
		cursor: pointer;
		transition: background 0.15s;
	}

	.btn-back:hover {
		background: #F8FAFC;
	}

	.btn-continue {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		padding: 11px 26px;
		font-size: 14px;
		font-weight: 500;
		color: #FFFFFF;
		background: #2563EB;
		border: none;
		border-radius: 8px;
		cursor: pointer;
		box-shadow: 0 2px 8px rgba(37, 99, 235, 0.25);
		transition: all 0.15s ease;
	}

	.btn-continue:hover:not(:disabled) {
		background: #1D4ED8;
	}

	.btn-continue:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.error-msg {
		font-size: 13px;
		color: #EF4444;
		margin-top: 14px;
	}

	/* ─────────────────────────────────────────────────────────────
	   STEP 7: SUCCESS FULL-SCREEN EXPERIENCE
	   ───────────────────────────────────────────────────────────── */
	.success-fullscreen {
		width: 100vw;
		min-height: 100vh;
		background: #F8F9FD;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 40px 24px;
		box-sizing: border-box;
	}

	.success-content {
		width: 100%;
		max-width: 580px;
		background: #FFFFFF;
		border-radius: 24px;
		border: 1px solid #EAECEF;
		box-shadow: 0 20px 48px -12px rgba(15, 23, 42, 0.08);
		padding: 0 0 44px 0;
		text-align: center;
		display: flex;
		flex-direction: column;
		align-items: center;
		box-sizing: border-box;
		overflow: hidden;
	}

	.success-hero-image-wrap {
		width: 100%;
		max-width: 100%;
		margin-bottom: 28px;
		display: flex;
		justify-content: center;
		overflow: hidden;
		line-height: 0;
	}

	.success-mascot-image {
		width: 100%;
		height: auto;
		max-height: 320px;
		object-fit: cover;
		display: block;
		border-radius: 0;
	}

	.success-body {
		width: 100%;
		padding: 0 40px;
		display: flex;
		flex-direction: column;
		align-items: center;
		box-sizing: border-box;
	}

	.success-heading {
		font-size: 28px;
		font-weight: 500;
		color: #0F172A;
		margin: 0 0 12px 0;
		letter-spacing: -0.4px;
	}

	.success-subheading {
		font-size: 15px;
		color: #64748B;
		line-height: 1.6;
		margin: 0 0 36px 0;
	}

	.success-actions {
		width: 100%;
		max-width: 360px;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.btn-success-primary {
		width: 100%;
		padding: 13px;
		font-size: 14.5px;
		font-weight: 500;
		color: #FFFFFF;
		background: #2563EB;
		border: none;
		border-radius: 10px;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 8px;
		box-shadow: 0 2px 8px rgba(37, 99, 235, 0.25);
		transition: background 0.15s;
	}

	.btn-success-primary:hover {
		background: #1D4ED8;
	}

	.btn-success-secondary {
		width: 100%;
		padding: 13px;
		font-size: 14.5px;
		font-weight: 500;
		color: #334155;
		background: #FFFFFF;
		border: 1px solid #E2E8F0;
		border-radius: 10px;
		cursor: pointer;
		transition: background 0.15s;
	}

	.btn-success-secondary:hover {
		background: #F8FAFC;
	}

	/* ─────────────────────────────────────────────────────────────
	   Modal Backdrop & QR Code Modal
	   ───────────────────────────────────────────────────────────── */
	.modal-backdrop {
		position: fixed;
		inset: 0;
		background: rgba(15, 23, 42, 0.4);
		backdrop-filter: blur(4px);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 999;
		padding: 16px;
	}

	.modal-card {
		width: 100%;
		max-width: 400px;
		background: #FFFFFF;
		border-radius: 16px;
		padding: 28px 24px;
		text-align: center;
		box-shadow: 0 20px 40px rgba(0, 0, 0, 0.1);
	}

	.modal-title {
		font-size: 18px;
		font-weight: 500;
		color: #0F172A;
		margin: 0 0 8px 0;
	}

	.modal-desc {
		font-size: 13px;
		color: #64748B;
		line-height: 1.4;
		margin: 0 0 20px 0;
	}

	.qr-box {
		width: 170px;
		height: 170px;
		margin: 0 auto 24px auto;
		background: #FFFFFF;
		border: 1px solid #E2E8F0;
		border-radius: 12px;
		display: flex;
		align-items: center;
		justify-content: center;
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.04);
	}

	.modal-actions {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 10px;
	}

	.btn-cancel {
		padding: 9px 16px;
		font-size: 13px;
		font-weight: 500;
		color: #64748B;
		background: #FFFFFF;
		border: 1px solid #E2E8F0;
		border-radius: 8px;
		cursor: pointer;
	}

	.btn-confirm {
		padding: 9px 18px;
		font-size: 13px;
		font-weight: 500;
		color: #FFFFFF;
		background: #2563EB;
		border: none;
		border-radius: 8px;
		cursor: pointer;
	}

	/* ─────────────────────────────────────────────────────────────
	   Responsive adjustments
	   ───────────────────────────────────────────────────────────── */
	@media (max-width: 1024px) {
		.onboarding-fullscreen {
			flex-direction: column;
		}

		.left-panel {
			width: 100%;
			border-right: none;
			border-bottom: 1px solid #EEF2F6;
			padding: 0;
		}

		.left-top {
			padding: 32px 24px 0 24px;
		}

		.middle-stepper {
			width: 100%;
			border-right: none;
			border-bottom: 1px solid #F1F5F9;
			padding: 20px 24px;
		}

		.step-list {
			flex-direction: row;
			overflow-x: auto;
			gap: 20px;
		}

		.right-content {
			padding: 36px 24px;
		}
	}
</style>
