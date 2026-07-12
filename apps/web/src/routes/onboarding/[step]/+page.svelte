<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { apiRequest } from '$lib/api';

	// ──────────────────────────────────────────────
	// Constants
	// ──────────────────────────────────────────────
	const STEP_KEYS = [
		'signup',
		'mode_selected',
		'business_basics',
		'channel_connect',
		'kb_setup',
		'reply_mode',
		'pipeline_setup',
		'team_invite',
		'done'
	];

	const STEP_LABELS = [
		'Create Account',
		'Choose Setup',
		'Your Business',
		'Connect Channel',
		'Knowledge Base',
		'Reply Mode',
		'Lead Pipeline',
		'Invite Team',
		'Done'
	];

	// ──────────────────────────────────────────────
	// Reactive state
	// ──────────────────────────────────────────────
	let stepNum = $derived(parseInt((page.params as any)?.step ?? '1', 10) || 1);

	let loading = $state(true);
	let submitting = $state(false);
	let error = $state('');

	// Onboarding status
	let completedSteps = $state<string[]>([]);
	let skippedSteps = $state<string[]>([]);
	let businessType = $state('');
	let productMode = $state('full_workspace');

	// Templates
	let templates = $state<any[]>([]);

	// Pipeline
	let pipelineStates = $state<any[]>([]);

	// ──────────────────────────────────────────────
	// Step 1 – Signup
	// ──────────────────────────────────────────────
	let s1AccountName = $state('');
	let s1Email = $state('');
	let s1Password = $state('');
	let s1IsLoggedIn = $state(false);

	// ──────────────────────────────────────────────
	// Step 2 – Mode Selection
	// ──────────────────────────────────────────────
	let s2Selected = $state<'chatbot_only' | 'full_workspace' | ''>('');

	// ──────────────────────────────────────────────
	// Step 3 – Business Type
	// ──────────────────────────────────────────────
	const BUSINESS_TYPES = [
		{ key: 'salon', label: 'Salon / Beauty', emoji: '✂️', desc: 'Bookings, appointments, and beauty services' },
		{ key: 'photography', label: 'Photography', emoji: '📷', desc: 'Client enquiries, sessions, and galleries' },
		{ key: 'tutoring', label: 'Tutoring / Education', emoji: '📚', desc: 'Enrolments, schedules, and lesson tracking' },
		{ key: 'home_services', label: 'Home Services', emoji: '🛠️', desc: 'Jobs, quotes, and service scheduling' },
		{ key: 'other', label: 'Other Business', emoji: '💼', desc: 'General business enquiries and support' }
	];
	let s3Selected = $state('');

	// ──────────────────────────────────────────────
	// Step 4 – Channel Connect
	// ──────────────────────────────────────────────
	let s4Phase = $state<'start' | 'waiting-qr' | 'waiting-message' | 'message-received'>('start');
	let s4ChannelID = $state('');
	let s4MessagePreview = $state('');

	// ──────────────────────────────────────────────
	// Step 5 – Knowledge Base
	// ──────────────────────────────────────────────
	let s5Prompts = $state<Array<{ label: string; placeholder: string; value: string }>>([]);
	let s5Phase = $state<'form' | 'review'>('form');
	let s5Concepts = $state<any[]>([]);
	let s5QueuedCount = $state(0);

	// ──────────────────────────────────────────────
	// Step 6 – Reply Mode
	// ──────────────────────────────────────────────
	let s6ReplyMode = $state<'draft' | 'auto'>('draft');

	// ──────────────────────────────────────────────
	// Step 8 – Team Invite
	// ──────────────────────────────────────────────
	let s8Email = $state('');
	let s8Role = $state<'admin' | 'member'>('member');
	let s8Invites = $state<Array<{ email: string; role: string; token: string }>>([]);
	let s8HasInvited = $state(false);

	// ──────────────────────────────────────────────
	// Navigation helpers
	// ──────────────────────────────────────────────
	function advanceToNext(fromStep: number) {
		let next = fromStep + 1;
		// skip step 7 (pipeline) for chatbot_only
		if (next === 7 && productMode === 'chatbot_only') next = 8;
		if (next > 9) next = 9;
		goto(`/onboarding/${next}`);
	}

	async function completeStep(stepKey: string) {
		try {
			await apiRequest('/onboarding/status', {
				method: 'PATCH',
				body: { step: stepKey, action: 'complete' }
			});
		} catch (err) {
			// proceed anyway
		}
		advanceToNext(stepNum);
	}

	async function skipStep(stepKey: string) {
		try {
			await apiRequest('/onboarding/status', {
				method: 'PATCH',
				body: { step: stepKey, action: 'skip' }
			});
		} catch (err) {
			// proceed anyway
		}
		advanceToNext(stepNum);
	}

	// ──────────────────────────────────────────────
	// Mount — auth + load data
	// ──────────────────────────────────────────────
	onMount(async () => {
		// Auth check
		try {
			await apiRequest('/auth/me');
		} catch {
			goto('/login');
			return;
		}

		// Load onboarding status
		try {
			const status = await apiRequest('/onboarding/status');
			if (status) {
				completedSteps = status.completed_steps ?? [];
				skippedSteps = status.skipped_steps ?? [];
				businessType = status.business_type ?? '';
			}
		} catch (_) {}

		// Load account / product mode
		try {
			const account = await apiRequest('/workspace/account');
			if (account) {
				productMode = account.product_mode || 'full_workspace';
			}
		} catch (_) {}

		// Load templates
		try {
			const tmpl = await apiRequest('/onboarding/templates');
			if (Array.isArray(tmpl)) templates = tmpl;
		} catch (_) {}

		// Per-step initialisation
		if (stepNum === 1) {
			// Check if already logged in + signup complete
			try {
				s1IsLoggedIn = true;
				if (completedSteps.includes('signup')) {
					advanceToNext(1);
					return;
				}
				// logged in but signup not marked — mark it now
				await completeStep('signup');
				return;
			} catch {
				s1IsLoggedIn = false;
			}
		}

		if (stepNum === 5 && businessType) {
			const tpl = templates.find((t: any) => t.business_type === businessType);
			if (tpl?.kb_prompts) {
				s5Prompts = tpl.kb_prompts.map((p: any) => ({ label: p.label, placeholder: p.placeholder ?? '', value: '' }));
			}
		}

		if (stepNum === 7) {
			if (productMode === 'chatbot_only') {
				advanceToNext(7);
				return;
			}
			try {
				const pipelines = await apiRequest('/workspace/pipelines');
				if (pipelines && pipelines.length > 0) {
					pipelineStates = pipelines[0].states ?? [];
				}
			} catch (_) {}
		}

		if (stepNum === 9) {
			// Mark onboarding done
			try {
				await apiRequest('/onboarding/status', {
					method: 'PATCH',
					body: { step: 'done', action: 'complete' }
				});
			} catch (_) {}
		}

		loading = false;
	});

	// ──────────────────────────────────────────────
	// Step 1 handlers
	// ──────────────────────────────────────────────
	async function handleS1Signup(e: Event) {
		e.preventDefault();
		submitting = true;
		error = '';
		try {
			await apiRequest('/auth/signup', {
				method: 'POST',
				body: { account_name: s1AccountName, email: s1Email, password: s1Password, product_mode: 'full_workspace' }
			});
			await apiRequest('/auth/login', {
				method: 'POST',
				body: { email: s1Email, password: s1Password }
			});
			await completeStep('signup');
		} catch (err: any) {
			error = err.message || 'Signup failed. Please try again.';
		} finally {
			submitting = false;
		}
	}

	// ──────────────────────────────────────────────
	// Step 2 handlers
	// ──────────────────────────────────────────────
	async function handleS2Select(mode: 'chatbot_only' | 'full_workspace') {
		s2Selected = mode;
		submitting = true;
		error = '';
		try {
			await apiRequest('/workspace/account', {
				method: 'PATCH',
				body: { product_mode: mode }
			});
			productMode = mode;
			await completeStep('mode_selected');
		} catch (err: any) {
			error = err.message || 'Failed to save. Please try again.';
			submitting = false;
		}
	}

	// ──────────────────────────────────────────────
	// Step 3 handlers
	// ──────────────────────────────────────────────
	async function handleS3Select(btype: string) {
		s3Selected = btype;
		submitting = true;
		error = '';
		try {
			await apiRequest('/onboarding/apply-template', {
				method: 'POST',
				body: { business_type: btype }
			});
			businessType = btype;
			await completeStep('business_basics');
		} catch (err: any) {
			error = err.message || 'Failed to apply template. Please try again.';
			submitting = false;
		}
	}

	// ──────────────────────────────────────────────
	// Step 4 handlers
	// ──────────────────────────────────────────────
	async function handleS4CreateChannel() {
		submitting = true;
		error = '';
		try {
			const channel = await apiRequest('/channels', {
				method: 'POST',
				body: { type: 'matrix_whatsapp', name: 'WhatsApp' }
			});
			s4ChannelID = channel?.id ?? '';
			s4Phase = 'waiting-qr';
		} catch (err: any) {
			error = err.message || 'Failed to create channel.';
		} finally {
			submitting = false;
		}
	}

	// Channel status WS event listener
	$effect(() => {
		if (stepNum !== 4) return;
		const handleChannelStatus = (e: CustomEvent) => {
			if (e.detail?.channel_id === s4ChannelID && e.detail?.status === 'connected') {
				s4Phase = 'waiting-message';
			}
		};
		const handleMessage = (e: CustomEvent) => {
			if (s4Phase === 'waiting-message') {
				const preview = e.detail?.message?.content;
				try {
					const parsed = typeof preview === 'string' ? JSON.parse(preview) : preview;
					s4MessagePreview = parsed?.text ?? '[message received]';
				} catch {
					s4MessagePreview = '[message received]';
				}
				s4Phase = 'message-received';
			}
		};
		window.addEventListener('channel-status-changed', handleChannelStatus as EventListener);
		window.addEventListener('message.received', handleMessage as EventListener);
		return () => {
			window.removeEventListener('channel-status-changed', handleChannelStatus as EventListener);
			window.removeEventListener('message.received', handleMessage as EventListener);
		};
	});

	// ──────────────────────────────────────────────
	// Step 5 handlers
	// ──────────────────────────────────────────────
	async function handleS5Submit(e: Event) {
		e.preventDefault();
		submitting = true;
		error = '';
		try {
			const filled = s5Prompts.filter(p => p.value.trim());
			if (filled.length === 0) {
				await skipStep('kb_setup');
				return;
			}
			const raw_text = filled.map(p => `${p.label}\n${p.value.trim()}`).join('\n\n');
			const result = await apiRequest('/api/kb/compile-paste', {
				method: 'POST',
				body: { raw_text }
			});
			s5Concepts = result?.added_concepts ?? [];
			s5QueuedCount = result?.queued_suggestions?.length ?? 0;
			s5Phase = 'review';
		} catch (err: any) {
			error = err.message || 'Failed to compile knowledge base.';
		} finally {
			submitting = false;
		}
	}

	// ──────────────────────────────────────────────
	// Step 6 handlers
	// ──────────────────────────────────────────────
	async function handleS6Continue() {
		submitting = true;
		error = '';
		try {
			await apiRequest('/users/me/reply-mode', {
				method: 'PATCH',
				body: { reply_mode: s6ReplyMode }
			});
			await completeStep('reply_mode');
		} catch (err: any) {
			error = err.message || 'Failed to save reply mode.';
			submitting = false;
		}
	}

	// ──────────────────────────────────────────────
	// Step 8 handlers
	// ──────────────────────────────────────────────
	async function handleS8Invite(e: Event) {
		e.preventDefault();
		if (!s8Email.trim()) return;
		submitting = true;
		error = '';
		try {
			const result = await apiRequest('/workspace/users/invite', {
				method: 'POST',
				body: { email: s8Email, role: s8Role }
			});
			s8Invites = [...s8Invites, { email: s8Email, role: s8Role, token: result?.token ?? '' }];
			s8Email = '';
			s8HasInvited = true;
		} catch (err: any) {
			error = err.message || 'Failed to send invite.';
		} finally {
			submitting = false;
		}
	}

	async function handleS8Done(skip: boolean) {
		if (skip && !s8HasInvited) {
			await skipStep('team_invite');
		} else {
			await completeStep('team_invite');
		}
	}

	// Pipeline state color helper
	function pipelineColor(color: string) {
		return color || '#6366f1';
	}
</script>

{#if loading}
	<div class="step-card glass-panel">
		<div class="loading-state">
			<div class="spinner"></div>
			<p>Loading...</p>
		</div>
	</div>

<!-- ════════════════════════════════════════
     STEP 1 — Sign Up
════════════════════════════════════════ -->
{:else if stepNum === 1}
	<div class="step-card glass-panel fade-in">
		<div class="step-header">
			<h2>Create your account</h2>
			<p>Get started with What Funnel in seconds</p>
		</div>

		{#if s1IsLoggedIn}
			<div class="loading-state">
				<div class="spinner"></div>
				<p style="color: var(--text-secondary); font-size: 14px;">You're already signed in. Continuing...</p>
			</div>
		{:else}
			<form onsubmit={handleS1Signup} class="step-form">
				{#if error}
					<div class="error-banner">{error}</div>
				{/if}

				<div class="field-group">
					<label for="accountName" class="field-label">Business Name</label>
					<input type="text" id="accountName" class="input-field" bind:value={s1AccountName} placeholder="Acme Corp" required disabled={submitting} />
				</div>

				<div class="field-group">
					<label for="email" class="field-label">Email address</label>
					<input type="email" id="email" class="input-field" bind:value={s1Email} placeholder="you@example.com" required disabled={submitting} />
				</div>

				<div class="field-group">
					<label for="password" class="field-label">Password</label>
					<input type="password" id="password" class="input-field" bind:value={s1Password} placeholder="••••••••" required disabled={submitting} minlength={8} />
				</div>

				<button type="submit" class="btn-primary full-width" disabled={submitting}>
					{submitting ? 'Creating account...' : 'Create Account & Continue →'}
				</button>
			</form>

			<div class="step-footer-link">
				Already have an account? <a href="/login">Sign in</a>
			</div>
		{/if}
	</div>

<!-- ════════════════════════════════════════
     STEP 2 — Choose Setup
════════════════════════════════════════ -->
{:else if stepNum === 2}
	<div class="step-card glass-panel fade-in">
		<div class="step-header">
			<h2>How do you want to use What Funnel?</h2>
			<p>Choose the setup that fits your business. You can change this later.</p>
		</div>

		{#if error}
			<div class="error-banner">{error}</div>
		{/if}

		<div class="mode-cards">
			<button
				class="mode-card glass-panel"
				class:selected={s2Selected === 'chatbot_only'}
				onclick={() => handleS2Select('chatbot_only')}
				disabled={submitting}
			>
				<div class="mode-icon">🤖</div>
				<div class="mode-label">Automated replies only</div>
				<div class="mode-desc">Your AI assistant handles common questions automatically. You keep using WhatsApp as normal.</div>
				<div class="mode-select-indicator" class:active={s2Selected === 'chatbot_only'}></div>
			</button>

			<button
				class="mode-card glass-panel"
				class:selected={s2Selected === 'full_workspace'}
				onclick={() => handleS2Select('full_workspace')}
				disabled={submitting}
			>
				<div class="mode-icon">📊</div>
				<div class="mode-label">Full lead workspace</div>
				<div class="mode-desc">Everything in the automated plan, plus a shared inbox, lead tracking, and team collaboration.</div>
				<div class="mode-select-indicator" class:active={s2Selected === 'full_workspace'}></div>
				<div class="recommended-badge">Recommended</div>
			</button>
		</div>

		{#if submitting}
			<div class="submitting-hint">Saving your choice...</div>
		{/if}
	</div>

<!-- ════════════════════════════════════════
     STEP 3 — Business Type
════════════════════════════════════════ -->
{:else if stepNum === 3}
	<div class="step-card glass-panel fade-in">
		<div class="step-header">
			<h2>What type of business are you?</h2>
			<p>We'll set up your bot with the right templates to get you started faster.</p>
		</div>

		{#if error}
			<div class="error-banner">{error}</div>
		{/if}

		<div class="biz-grid">
			{#each BUSINESS_TYPES as biz}
				<button
					class="biz-tile glass-panel"
					class:selected={s3Selected === biz.key}
					onclick={() => handleS3Select(biz.key)}
					disabled={submitting}
				>
					<div class="biz-emoji">{biz.emoji}</div>
					<div class="biz-label">{biz.label}</div>
					<div class="biz-desc">{biz.desc}</div>
				</button>
			{/each}
		</div>

		{#if submitting}
			<div class="submitting-hint">Applying template...</div>
		{/if}
	</div>

<!-- ════════════════════════════════════════
     STEP 4 — Connect Channel
════════════════════════════════════════ -->
{:else if stepNum === 4}
	<div class="step-card glass-panel fade-in">
		<div class="step-header">
			<h2>Connect your WhatsApp number</h2>
			<p>Link your WhatsApp so your bot can receive and reply to messages.</p>
		</div>

		{#if error}
			<div class="error-banner">{error}</div>
		{/if}

		{#if s4Phase === 'start'}
			<div class="channel-start">
				<div class="whatsapp-icon">💬</div>
				<p class="channel-hint">We'll create a WhatsApp connection and show you a QR code to scan.</p>
				<button class="btn-primary full-width" onclick={handleS4CreateChannel} disabled={submitting}>
					{submitting ? 'Creating connection...' : 'Create WhatsApp connection'}
				</button>
				<button class="skip-link" onclick={() => skipStep('channel_connect')}>
					Skip for now, I'll connect later
				</button>
			</div>

		{:else if s4Phase === 'waiting-qr'}
			<div class="qr-area">
				<div class="qr-placeholder">
					<div class="qr-scanner-line"></div>
					<div class="qr-inner-dots">
						{#each Array(16) as _}
							<div class="qr-dot"></div>
						{/each}
					</div>
					<span class="qr-label">QR code will appear here</span>
				</div>
				<div class="qr-instructions">
					<div class="instruction-item"><span class="step-num">1</span> Open WhatsApp on your phone</div>
					<div class="instruction-item"><span class="step-num">2</span> Tap Menu (⋮) → Linked Devices</div>
					<div class="instruction-item"><span class="step-num">3</span> Tap "Link a Device" and scan this QR code</div>
				</div>
				<button class="skip-link" onclick={() => skipStep('channel_connect')}>
					Skip for now, I'll connect later
				</button>
			</div>

		{:else if s4Phase === 'waiting-message'}
			<div class="waiting-message-state">
				<div class="pulse-circle">
					<div class="pulse-inner">✓</div>
				</div>
				<h3>Connected!</h3>
				<p>Now send a WhatsApp message to your number. We'll show you what happens next.</p>
				<p class="waiting-hint">Ask someone to message your number — or message it yourself.</p>
				<button class="skip-link" onclick={() => completeStep('channel_connect')}>
					Skip this — I'll try later
				</button>
			</div>

		{:else if s4Phase === 'message-received'}
			<div class="message-received-state">
				<div class="success-icon">🎉</div>
				<h3>Got it!</h3>
				<p>Here's what just came in →</p>
				<div class="message-preview glass-panel">
					<span class="preview-icon">💬</span>
					<span class="preview-text">"{s4MessagePreview}"</span>
				</div>
				<p style="font-size: 13px; color: var(--text-secondary); margin-top: 12px;">Your bot will respond to messages like this automatically.</p>
				<button class="btn-primary full-width" onclick={() => completeStep('channel_connect')} style="margin-top: 20px;">
					Continue →
				</button>
			</div>
		{/if}
	</div>

<!-- ════════════════════════════════════════
     STEP 5 — Knowledge Base
════════════════════════════════════════ -->
{:else if stepNum === 5}
	<div class="step-card glass-panel fade-in">
		{#if s5Phase === 'form'}
			<div class="step-header">
				<h2>Tell your bot about your business</h2>
				<p>Fill in what you know — your bot will use this to answer customer questions.</p>
			</div>

			{#if error}
				<div class="error-banner">{error}</div>
			{/if}

			<form onsubmit={handleS5Submit} class="step-form">
				{#if s5Prompts.length > 0}
					{#each s5Prompts as prompt, i}
						<div class="field-group">
							<label class="field-label" for={`kb-prompt-${i}`}>{prompt.label}</label>
							<textarea
								id={`kb-prompt-${i}`}
								class="input-field kb-textarea"
								placeholder={prompt.placeholder}
								bind:value={s5Prompts[i].value}
								rows={3}
								disabled={submitting}
							></textarea>
						</div>
					{/each}
				{:else}
					<div class="field-group">
						<label class="field-label" for="kb-general">About your business</label>
						<textarea
							id="kb-general"
							class="input-field kb-textarea"
							placeholder="Describe your services, hours, pricing, location, and anything customers frequently ask about..."
							rows={5}
							disabled={submitting}
						></textarea>
					</div>
				{/if}

				<button type="submit" class="btn-primary full-width" disabled={submitting}>
					{submitting ? 'Setting up your bot...' : 'Set up my bot →'}
				</button>
				<button type="button" class="skip-link" onclick={() => skipStep('kb_setup')}>
					Skip for now
				</button>
			</form>

		{:else if s5Phase === 'review'}
			<div class="step-header">
				<h2>Here's what we picked up</h2>
				<p>Does this look right? You can always edit in Settings later.</p>
			</div>

			{#if s5Concepts.length > 0}
				<div class="concepts-list">
					{#each s5Concepts as concept}
						<div class="concept-chip glass-panel">
							<span class="concept-title">{concept.title ?? concept.name ?? concept}</span>
						</div>
					{/each}
				</div>
			{/if}

			{#if s5QueuedCount > 0}
				<p class="queued-hint">{s5QueuedCount} suggestion{s5QueuedCount !== 1 ? 's' : ''} queued for your review in Settings.</p>
			{/if}

			<div class="review-actions">
				<button class="btn-primary full-width" onclick={() => completeStep('kb_setup')} disabled={submitting}>
					Looks good, continue →
				</button>
				<a href="/settings/knowledge-base" class="secondary-link">Edit in settings</a>
			</div>
		{/if}
	</div>

<!-- ════════════════════════════════════════
     STEP 6 — Reply Mode
════════════════════════════════════════ -->
{:else if stepNum === 6}
	<div class="step-card glass-panel fade-in">
		<div class="step-header">
			<h2>How should your bot send replies?</h2>
			<p>You can change this in Settings at any time.</p>
		</div>

		{#if error}
			<div class="error-banner">{error}</div>
		{/if}

		<div class="reply-mode-cards">
			<button
				class="mode-card glass-panel"
				class:selected={s6ReplyMode === 'draft'}
				onclick={() => s6ReplyMode = 'draft'}
				disabled={submitting}
			>
				<div class="radio-circle" class:checked={s6ReplyMode === 'draft'}></div>
				<div class="mode-body">
					<div class="mode-label">Review before it sends</div>
					<div class="mode-desc">Your bot drafts a reply for you to approve first. You're always in control.</div>
					<span class="recommended-badge inline">Recommended</span>
				</div>
			</button>

			<button
				class="mode-card glass-panel"
				class:selected={s6ReplyMode === 'auto'}
				onclick={() => s6ReplyMode = 'auto'}
				disabled={submitting}
			>
				<div class="radio-circle" class:checked={s6ReplyMode === 'auto'}></div>
				<div class="mode-body">
					<div class="mode-label">Send automatically once confident</div>
					<div class="mode-desc">The bot sends replies instantly when it's confident. You review the log afterwards.</div>
				</div>
			</button>
		</div>

		<button class="btn-primary full-width" onclick={handleS6Continue} disabled={submitting} style="margin-top: 24px;">
			{submitting ? 'Saving...' : 'Continue →'}
		</button>
	</div>

<!-- ════════════════════════════════════════
     STEP 7 — Pipeline Setup (full_workspace only)
════════════════════════════════════════ -->
{:else if stepNum === 7}
	<div class="step-card glass-panel fade-in">
		<div class="step-header">
			<h2>Your lead pipeline is ready</h2>
			<p>We've set up a pipeline based on your business type. Review the stages below.</p>
		</div>

		{#if pipelineStates.length > 0}
			<div class="pipeline-viz">
				{#each pipelineStates as st, i}
					<div class="pipeline-badge" style="border-color: {pipelineColor(st.color)}; color: {pipelineColor(st.color)};">
						{st.label}
					</div>
					{#if i < pipelineStates.length - 1}
						<div class="pipeline-arrow">→</div>
					{/if}
				{/each}
			</div>
		{:else}
			<div class="pipeline-placeholder">
				<p style="color: var(--text-secondary); font-size: 14px;">Pipeline stages will appear here once loaded.</p>
			</div>
		{/if}

		<div class="pipeline-actions">
			<button class="btn-primary full-width" onclick={() => completeStep('pipeline_setup')}>
				Looks good, continue →
			</button>
			<a href="/settings/pipeline" class="secondary-link">Customize pipeline now</a>
			<button class="skip-link" onclick={() => completeStep('pipeline_setup')}>
				Accept and adjust later
			</button>
		</div>
	</div>

<!-- ════════════════════════════════════════
     STEP 8 — Team Invite
════════════════════════════════════════ -->
{:else if stepNum === 8}
	<div class="step-card glass-panel fade-in">
		<div class="step-header">
			<h2>Add your team</h2>
			<p>Invite people now, or skip and do it later from Settings.</p>
		</div>

		{#if error}
			<div class="error-banner">{error}</div>
		{/if}

		<form onsubmit={handleS8Invite} class="invite-form">
			<div class="invite-row">
				<input
					type="email"
					class="input-field"
					bind:value={s8Email}
					placeholder="teammate@example.com"
					disabled={submitting}
					style="flex: 1;"
				/>
				<select class="input-field role-select" bind:value={s8Role} disabled={submitting}>
					<option value="member">Member</option>
					<option value="admin">Admin</option>
				</select>
				<button type="submit" class="btn-secondary invite-btn" disabled={submitting || !s8Email.trim()}>
					{submitting ? '...' : 'Send invite'}
				</button>
			</div>
		</form>

		{#if s8Invites.length > 0}
			<div class="invite-list">
				{#each s8Invites as inv}
					<div class="invite-item glass-panel">
						<span class="invite-email">{inv.email}</span>
						<span class="invite-role-badge">{inv.role}</span>
						<span class="invite-sent">✓ Sent</span>
					</div>
				{/each}
			</div>
		{/if}

		<div class="team-actions">
			<button class="btn-primary full-width" onclick={() => handleS8Done(false)} disabled={submitting} style="margin-top: 24px;">
				{s8HasInvited ? 'Done adding people →' : 'Continue →'}
			</button>
			<button class="skip-link" onclick={() => handleS8Done(true)}>
				Skip for now
			</button>
		</div>
	</div>

<!-- ════════════════════════════════════════
     STEP 9 — Done!
════════════════════════════════════════ -->
{:else if stepNum === 9}
	<div class="step-card glass-panel fade-in done-card">
		<div class="confetti-bg" aria-hidden="true">
			{#each Array(20) as _, i}
				<div class="confetti-dot" style="--delay: {i * 0.15}s; --x: {Math.round(Math.random() * 100)}%; --y: {Math.round(Math.random() * 100)}%;"></div>
			{/each}
		</div>

		<div class="checkmark-wrap">
			<div class="checkmark-circle">
				<svg class="checkmark-svg" viewBox="0 0 52 52" fill="none">
					<circle class="checkmark-bg" cx="26" cy="26" r="25" />
					<path class="checkmark-tick" d="M14 26l8 8 16-16" stroke="#fff" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/>
				</svg>
			</div>
		</div>

		<h2 class="done-headline">You're all set!</h2>
		<p class="done-subtext">
			{#if productMode === 'chatbot_only'}
				Your bot is ready. You can manage it from the inbox anytime.
			{:else}
				Your workspace is ready. Head to the inbox to start managing conversations and leads.
			{/if}
		</p>

		<button class="btn-primary full-width done-cta" onclick={() => goto('/inbox')}>
			{productMode === 'chatbot_only' ? 'Go to Activity →' : 'Go to Inbox →'}
		</button>
	</div>
{/if}

<style>
	/* ─── Animations ─── */
	@keyframes fadeSlideIn {
		from { opacity: 0; transform: translateY(16px); }
		to   { opacity: 1; transform: translateY(0); }
	}
	.fade-in {
		animation: fadeSlideIn 0.4s ease both;
	}

	/* ─── Base card ─── */
	.step-card {
		max-width: 680px;
		margin: 0 auto;
		padding: 40px;
		position: relative;
	}

	.step-header {
		text-align: center;
		margin-bottom: 32px;
	}
	.step-header h2 {
		font-size: 24px;
		font-weight: 700;
		color: var(--text-primary);
		margin-bottom: 8px;
		line-height: 1.3;
	}
	.step-header p {
		font-size: 14px;
		color: var(--text-secondary);
	}

	/* ─── Form ─── */
	.step-form {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}
	.field-group {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.field-label {
		font-size: 12px;
		font-weight: 600;
		color: var(--text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	/* ─── Full-width buttons ─── */
	.full-width {
		width: 100%;
		height: 46px;
		font-size: 15px;
		font-weight: 600;
	}

	/* ─── Error banner ─── */
	.error-banner {
		padding: 12px 16px;
		background: rgba(239, 68, 68, 0.1);
		border: 1px solid rgba(239, 68, 68, 0.3);
		border-radius: 8px;
		color: var(--danger);
		font-size: 13px;
		margin-bottom: 8px;
	}

	/* ─── Skip link ─── */
	.skip-link {
		background: none;
		border: none;
		color: var(--text-muted);
		font-size: 13px;
		cursor: pointer;
		padding: 8px 0;
		text-align: center;
		width: 100%;
		text-decoration: underline;
		transition: color 0.2s;
	}
	.skip-link:hover { color: var(--text-secondary); }

	.step-footer-link {
		margin-top: 20px;
		text-align: center;
		font-size: 13px;
		color: var(--text-secondary);
	}
	.step-footer-link a {
		color: #818cf8;
		text-decoration: none;
		font-weight: 500;
	}

	/* ─── Loading ─── */
	.loading-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 16px;
		padding: 24px 0;
		color: var(--text-secondary);
		font-size: 14px;
	}
	.spinner {
		width: 28px;
		height: 28px;
		border: 2px solid var(--border-color);
		border-top-color: #818cf8;
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}
	@keyframes spin { to { transform: rotate(360deg); } }

	.submitting-hint {
		text-align: center;
		font-size: 13px;
		color: var(--text-muted);
		margin-top: 12px;
	}

	/* ─── Mode cards (step 2) ─── */
	.mode-cards {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 16px;
	}
	.mode-card {
		padding: 24px;
		cursor: pointer;
		text-align: left;
		background: rgba(255,255,255,0.03);
		border: 1px solid var(--border-color);
		border-radius: 12px;
		position: relative;
		transition: all 0.2s;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.mode-card:hover:not(:disabled) {
		border-color: rgba(99, 102, 241, 0.4);
		background: rgba(99, 102, 241, 0.05);
	}
	.mode-card.selected {
		border-color: #6366f1;
		background: rgba(99, 102, 241, 0.08);
		box-shadow: 0 0 0 1px #6366f1 inset;
	}
	.mode-icon { font-size: 32px; }
	.mode-label { font-size: 15px; font-weight: 600; color: var(--text-primary); }
	.mode-desc { font-size: 13px; color: var(--text-secondary); line-height: 1.5; }
	.mode-select-indicator {
		width: 20px; height: 20px; border-radius: 50%;
		border: 2px solid var(--border-color);
		position: absolute; top: 16px; right: 16px;
		transition: all 0.2s;
	}
	.mode-select-indicator.active {
		background: var(--accent-gradient);
		border-color: transparent;
	}
	.recommended-badge {
		position: absolute;
		bottom: 12px; right: 12px;
		font-size: 10px; font-weight: 700;
		text-transform: uppercase;
		background: rgba(99, 102, 241, 0.2);
		color: #818cf8;
		padding: 3px 8px;
		border-radius: 20px;
		letter-spacing: 0.5px;
	}
	.recommended-badge.inline {
		position: static;
		display: inline-block;
		margin-top: 6px;
		font-size: 10px;
	}

	/* ─── Business type tiles (step 3) ─── */
	.biz-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 12px;
	}
	.biz-tile {
		padding: 20px 16px;
		cursor: pointer;
		text-align: center;
		background: rgba(255,255,255,0.03);
		border: 1px solid var(--border-color);
		border-radius: 12px;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
		transition: all 0.2s;
	}
	.biz-tile:hover:not(:disabled) {
		border-color: rgba(99, 102, 241, 0.4);
		background: rgba(99, 102, 241, 0.05);
		transform: translateY(-2px);
	}
	.biz-tile.selected {
		border-color: #6366f1;
		background: rgba(99, 102, 241, 0.1);
		box-shadow: 0 0 0 1px #6366f1 inset;
	}
	.biz-emoji { font-size: 28px; }
	.biz-label { font-size: 13px; font-weight: 600; color: var(--text-primary); }
	.biz-desc { font-size: 11px; color: var(--text-secondary); line-height: 1.4; }

	/* ─── Channel connect (step 4) ─── */
	.channel-start {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 16px;
		text-align: center;
	}
	.whatsapp-icon { font-size: 48px; }
	.channel-hint {
		font-size: 14px;
		color: var(--text-secondary);
		max-width: 320px;
		line-height: 1.5;
	}
	.qr-area {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 24px;
	}
	.qr-placeholder {
		width: 200px; height: 200px;
		border: 2px solid;
		border-image: linear-gradient(135deg, #6366f1, #a855f7) 1;
		border-radius: 12px;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		position: relative;
		overflow: hidden;
		background: rgba(99,102,241,0.04);
	}
	.qr-scanner-line {
		position: absolute;
		left: 0; right: 0;
		height: 2px;
		background: linear-gradient(90deg, transparent, rgba(99,102,241,0.8), transparent);
		animation: scan 2s ease-in-out infinite;
	}
	@keyframes scan {
		0% { top: 10%; }
		50% { top: 88%; }
		100% { top: 10%; }
	}
	.qr-inner-dots {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 8px;
		padding: 16px;
		opacity: 0.3;
	}
	.qr-dot {
		width: 10px; height: 10px;
		background: var(--text-secondary);
		border-radius: 2px;
	}
	.qr-label {
		font-size: 11px;
		color: var(--text-muted);
		position: absolute;
		bottom: 8px;
	}
	.qr-instructions {
		display: flex;
		flex-direction: column;
		gap: 10px;
		width: 100%;
		max-width: 340px;
	}
	.instruction-item {
		display: flex;
		align-items: center;
		gap: 12px;
		font-size: 14px;
		color: var(--text-secondary);
	}
	.step-num {
		width: 24px; height: 24px;
		background: rgba(99,102,241,0.15);
		color: #818cf8;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 12px;
		font-weight: 700;
		flex-shrink: 0;
	}
	.waiting-message-state, .message-received-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 12px;
		text-align: center;
	}
	.pulse-circle {
		width: 72px; height: 72px;
		border-radius: 50%;
		background: rgba(34,197,94,0.15);
		border: 2px solid var(--success);
		display: flex;
		align-items: center;
		justify-content: center;
		animation: gentle-pulse 2s ease-in-out infinite;
	}
	@keyframes gentle-pulse {
		0%, 100% { box-shadow: 0 0 0 0 rgba(34,197,94,0.3); }
		50% { box-shadow: 0 0 0 12px rgba(34,197,94,0); }
	}
	.pulse-inner {
		font-size: 24px;
		color: var(--success);
		font-weight: 700;
	}
	.waiting-message-state h3, .message-received-state h3 {
		font-size: 20px;
		font-weight: 700;
		color: var(--text-primary);
	}
	.waiting-message-state p, .message-received-state p {
		font-size: 14px;
		color: var(--text-secondary);
		line-height: 1.5;
	}
	.waiting-hint {
		font-size: 12px;
		color: var(--text-muted);
		font-style: italic;
	}
	.success-icon { font-size: 48px; }
	.message-preview {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 14px 20px;
		width: 100%;
		max-width: 360px;
	}
	.preview-icon { font-size: 20px; }
	.preview-text {
		font-size: 14px;
		color: var(--text-primary);
		font-style: italic;
	}

	/* ─── KB (step 5) ─── */
	.kb-textarea {
		resize: vertical;
		min-height: 80px;
		line-height: 1.5;
	}
	.concepts-list {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		margin-bottom: 16px;
	}
	.concept-chip {
		padding: 6px 14px;
		border-radius: 20px;
		font-size: 13px;
		color: #818cf8;
		background: rgba(99,102,241,0.1);
	}
	.concept-title { font-weight: 500; }
	.queued-hint {
		font-size: 13px;
		color: var(--text-secondary);
		margin-bottom: 16px;
		text-align: center;
	}
	.review-actions {
		display: flex;
		flex-direction: column;
		gap: 12px;
		align-items: center;
	}
	.secondary-link {
		font-size: 13px;
		color: #818cf8;
		text-decoration: none;
	}
	.secondary-link:hover { text-decoration: underline; }

	/* ─── Reply mode (step 6) ─── */
	.reply-mode-cards {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.reply-mode-cards .mode-card {
		flex-direction: row;
		align-items: flex-start;
		gap: 16px;
	}
	.radio-circle {
		width: 20px; height: 20px;
		border-radius: 50%;
		border: 2px solid var(--border-color);
		flex-shrink: 0;
		margin-top: 2px;
		transition: all 0.2s;
	}
	.radio-circle.checked {
		background: var(--accent-gradient);
		border-color: transparent;
		box-shadow: 0 0 0 2px rgba(99,102,241,0.3);
	}
	.mode-body {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	/* ─── Pipeline (step 7) ─── */
	.pipeline-viz {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 8px;
		margin-bottom: 28px;
		justify-content: center;
	}
	.pipeline-badge {
		padding: 6px 14px;
		border-radius: 20px;
		border: 1px solid;
		font-size: 13px;
		font-weight: 500;
		background: rgba(255,255,255,0.03);
	}
	.pipeline-arrow {
		color: var(--text-muted);
		font-size: 16px;
	}
	.pipeline-placeholder { text-align: center; margin-bottom: 28px; }
	.pipeline-actions {
		display: flex;
		flex-direction: column;
		gap: 12px;
		align-items: center;
	}

	/* ─── Team invite (step 8) ─── */
	.invite-form { margin-bottom: 16px; }
	.invite-row {
		display: flex;
		gap: 8px;
		align-items: center;
	}
	.role-select { width: 120px; flex-shrink: 0; }
	.invite-btn { flex-shrink: 0; white-space: nowrap; }
	.invite-list {
		display: flex;
		flex-direction: column;
		gap: 8px;
		margin-top: 12px;
	}
	.invite-item {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 10px 16px;
	}
	.invite-email { font-size: 13px; color: var(--text-primary); flex: 1; }
	.invite-role-badge {
		font-size: 11px;
		font-weight: 700;
		text-transform: uppercase;
		background: rgba(99,102,241,0.15);
		color: #818cf8;
		padding: 2px 8px;
		border-radius: 20px;
	}
	.invite-sent {
		font-size: 12px;
		color: var(--success);
		font-weight: 600;
	}
	.team-actions {
		display: flex;
		flex-direction: column;
		gap: 8px;
		align-items: center;
	}

	/* ─── Done (step 9) ─── */
	.done-card {
		text-align: center;
		overflow: hidden;
	}
	.confetti-bg {
		position: absolute;
		inset: 0;
		pointer-events: none;
		overflow: hidden;
	}
	.confetti-dot {
		position: absolute;
		width: 6px; height: 6px;
		border-radius: 50%;
		left: var(--x);
		top: var(--y);
		background: var(--accent-gradient);
		animation: float-dot 4s ease-in-out var(--delay) infinite alternate;
		opacity: 0.4;
	}
	@keyframes float-dot {
		from { transform: translateY(0) scale(1); opacity: 0.4; }
		to { transform: translateY(-20px) scale(1.4); opacity: 0.1; }
	}
	.checkmark-wrap {
		display: flex;
		justify-content: center;
		margin-bottom: 24px;
		position: relative;
	}
	.checkmark-circle {
		width: 80px; height: 80px;
	}
	.checkmark-svg {
		width: 100%; height: 100%;
	}
	.checkmark-bg {
		fill: none;
		stroke: #22c55e;
		stroke-width: 2;
		stroke-dasharray: 166;
		stroke-dashoffset: 166;
		animation: circle-draw 0.6s ease forwards 0.1s;
	}
	@keyframes circle-draw {
		to { stroke-dashoffset: 0; }
	}
	.checkmark-tick {
		stroke-dasharray: 48;
		stroke-dashoffset: 48;
		animation: tick-draw 0.4s ease forwards 0.7s;
	}
	@keyframes tick-draw {
		to { stroke-dashoffset: 0; }
	}
	.done-headline {
		font-size: 28px;
		font-weight: 800;
		background: var(--accent-gradient);
		-webkit-background-clip: text;
		-webkit-text-fill-color: transparent;
		margin-bottom: 12px;
	}
	.done-subtext {
		font-size: 15px;
		color: var(--text-secondary);
		line-height: 1.6;
		max-width: 400px;
		margin: 0 auto 28px;
	}
	.done-cta {
		max-width: 320px;
		margin: 0 auto;
	}
</style>
