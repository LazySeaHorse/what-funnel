<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { apiRequest } from '$lib/api';
	import Icon from '$lib/Icon.svelte';

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
	let stepNum = $derived(parseInt(($page.params as any)?.step ?? '1', 10) || 1);

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
	// Step 3 – Business Type (NO EMOJIS - SVG icons via icon name)
	// ──────────────────────────────────────────────
	const BUSINESS_TYPES = [
		{ key: 'salon', label: 'Salon / Beauty', icon: 'scissors', desc: 'Bookings, appointments, and beauty services' },
		{ key: 'photography', label: 'Photography', icon: 'camera', desc: 'Client enquiries, sessions, and galleries' },
		{ key: 'tutoring', label: 'Tutoring / Education', icon: 'book', desc: 'Enrolments, schedules, and lesson tracking' },
		{ key: 'home_services', label: 'Home Services', icon: 'wrench', desc: 'Jobs, quotes, and service scheduling' },
		{ key: 'other', label: 'Other Business', icon: 'briefcase', desc: 'General business enquiries and support' }
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
	let s6ReplyMode = $state<'draft_only' | 'auto_send'>('draft_only');

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
		try {
			await apiRequest('/auth/me');
		} catch {
			goto('/login');
			return;
		}

		try {
			const status = await apiRequest('/onboarding/status');
			if (status) {
				completedSteps = status.completed_steps ?? [];
				skippedSteps = status.skipped_steps ?? [];
				businessType = status.business_type ?? '';
			}
		} catch (_) {}

		try {
			const account = await apiRequest('/workspace/account');
			if (account) {
				productMode = account.product_mode || 'full_workspace';
			}
		} catch (_) {}

		try {
			const tmpl = await apiRequest('/onboarding/templates');
			if (Array.isArray(tmpl)) templates = tmpl;
		} catch (_) {}

		if (stepNum === 1) {
			try {
				s1IsLoggedIn = true;
				if (completedSteps.includes('signup')) {
					advanceToNext(1);
					return;
				}
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
			try {
				await apiRequest('/onboarding/status', {
					method: 'PATCH',
					body: { step: 'done', action: 'complete' }
				});
			} catch (_) {}
		}

		loading = false;
	});

	// Step 1 handlers
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

	// Step 2 handlers
	async function handleS2Select(mode: 'chatbot_only' | 'full_workspace') {
		s2Selected = mode;
		submitting = true;
		error = '';
		try {
			await apiRequest('/workspace/account/product-mode', {
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

	// Step 3 handlers
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

	// Step 4 handlers
	async function handleS4CreateChannel() {
		submitting = true;
		error = '';
		try {
			const defaultCreds = JSON.stringify({
				homeserver_url: 'http://localhost:8008',
				user_id: `@whatsapp_bridge:localhost`,
				access_token: 'onboarding-token'
			});
			const channel = await apiRequest('/channels', {
				method: 'POST',
				body: {
					type: 'matrix_whatsapp',
					bridge_identity: `whatsapp-${Date.now()}`,
					bridge_credentials: defaultCreds
				}
			});
			s4ChannelID = channel?.id ?? '';
			s4Phase = 'waiting-qr';
		} catch (err: any) {
			error = err.message || 'Failed to create channel.';
		} finally {
			submitting = false;
		}
	}

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

	// Step 5 handlers
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

	// Step 6 handlers
	async function handleS6Continue() {
		submitting = true;
		error = '';
		try {
			const apiMode = s6ReplyMode === 'draft_only' ? 'draft_only' : 'auto_send';
			await apiRequest('/users/me/reply-mode', {
				method: 'PATCH',
				body: { reply_mode: apiMode }
			});
			await completeStep('reply_mode');
		} catch (err: any) {
			error = err.message || 'Failed to save reply mode.';
			submitting = false;
		}
	}

	// Step 8 handlers
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

	function pipelineColor(color: string) {
		return color || '#0B6E99';
	}
</script>

{#if loading}
	<div class="step-card glass-panel">
		<div class="loading-state">
			<div class="spinner"></div>
			<p>Loading step...</p>
		</div>
	</div>

<!-- STEP 1 — Sign Up -->
{:else if stepNum === 1}
	<div class="step-card glass-panel fade-in">
		<div class="step-header">
			<h2>Create your account</h2>
			<p>Get started with What Funnel in seconds</p>
		</div>

		{#if s1IsLoggedIn}
			<div class="loading-state">
				<div class="spinner"></div>
				<p style="color: var(--text-secondary); font-size: 13.5px;">You're signed in. Continuing...</p>
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
					{submitting ? 'Creating account...' : 'Create Account & Continue'}
					<Icon name="arrow-right" size={16} />
				</button>
			</form>

			<div class="step-footer-link">
				Already have an account? <a href="/login">Sign in</a>
			</div>
		{/if}
	</div>

<!-- STEP 2 — Choose Setup -->
{:else if stepNum === 2}
	<div class="step-card glass-panel fade-in">
		<div class="step-header">
			<h2>How do you want to use What Funnel?</h2>
			<p>Choose the setup that fits your business. You can change this anytime.</p>
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
				<div class="mode-icon-box blue">
					<Icon name="bot" size={24} color="var(--blue-text)" />
				</div>
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
				<div class="mode-icon-box pink">
					<Icon name="layout" size={24} color="var(--pink-text)" />
				</div>
				<div class="mode-label">Full lead workspace</div>
				<div class="mode-desc">Everything in automated plan, plus shared inbox, lead tracking, and team collaboration.</div>
				<div class="mode-select-indicator" class:active={s2Selected === 'full_workspace'}></div>
				<div class="badge-blue recommended-badge">Recommended</div>
			</button>
		</div>

		{#if submitting}
			<div class="submitting-hint">Saving choice...</div>
		{/if}
	</div>

<!-- STEP 3 — Business Type -->
{:else if stepNum === 3}
	<div class="step-card glass-panel fade-in">
		<div class="step-header">
			<h2>What type of business are you?</h2>
			<p>We'll set up your assistant with targeted templates to get you started.</p>
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
					<div class="biz-icon-box">
						<Icon name={biz.icon} size={22} color="var(--blue-text)" />
					</div>
					<div class="biz-label">{biz.label}</div>
					<div class="biz-desc">{biz.desc}</div>
				</button>
			{/each}
		</div>

		{#if submitting}
			<div class="submitting-hint">Applying template...</div>
		{/if}
	</div>

<!-- STEP 4 — Connect Channel -->
{:else if stepNum === 4}
	<div class="step-card glass-panel fade-in">
		<div class="step-header">
			<h2>Connect your WhatsApp number</h2>
			<p>Link your WhatsApp so your assistant can receive and reply to messages.</p>
		</div>

		{#if error}
			<div class="error-banner">{error}</div>
		{/if}

		{#if s4Phase === 'start'}
			<div class="channel-start">
				<div class="channel-icon-box">
					<Icon name="whatsapp" size={32} color="var(--blue-text)" />
				</div>
				<p class="channel-hint">We'll create a WhatsApp connection and show you a QR code to scan.</p>
				<button class="btn-primary full-width" onclick={handleS4CreateChannel} disabled={submitting}>
					{submitting ? 'Creating connection...' : 'Create WhatsApp connection'}
					<Icon name="arrow-right" size={16} />
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
					<div class="instruction-item"><span class="step-num">2</span> Tap Menu → Linked Devices</div>
					<div class="instruction-item"><span class="step-num">3</span> Tap "Link a Device" and scan this QR code</div>
				</div>
				<button class="skip-link" onclick={() => skipStep('channel_connect')}>
					Skip for now, I'll connect later
				</button>
			</div>

		{:else if s4Phase === 'waiting-message'}
			<div class="waiting-message-state">
				<div class="pulse-circle">
					<Icon name="check" size={24} color="var(--success)" strokeWidth={3} />
				</div>
				<h3>Connected!</h3>
				<p>Send a WhatsApp message to your number to test the connection.</p>
				<p class="waiting-hint">Message your number from another phone or ask a colleague.</p>
				<button class="skip-link" onclick={() => completeStep('channel_connect')}>
					Skip test — continue
				</button>
			</div>

		{:else if s4Phase === 'message-received'}
			<div class="message-received-state">
				<div class="success-icon-box">
					<Icon name="sparkles" size={28} color="var(--blue-text)" />
				</div>
				<h3>Message Received!</h3>
				<div class="message-preview glass-panel">
					<Icon name="chat" size={18} color="var(--blue-text)" />
					<span class="preview-text">"{s4MessagePreview}"</span>
				</div>
				<button class="btn-primary full-width" onclick={() => completeStep('channel_connect')} style="margin-top: 20px;">
					Continue
					<Icon name="arrow-right" size={16} />
				</button>
			</div>
		{/if}
	</div>

<!-- STEP 5 — Knowledge Base -->
{:else if stepNum === 5}
	<div class="step-card glass-panel fade-in">
		{#if s5Phase === 'form'}
			<div class="step-header">
				<h2>Tell your assistant about your business</h2>
				<p>Fill in key details — your assistant uses this to answer customer queries.</p>
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
							placeholder="Describe your services, working hours, pricing, location, and common customer FAQs..."
							rows={5}
							disabled={submitting}
						></textarea>
					</div>
				{/if}

				<button type="submit" class="btn-primary full-width" disabled={submitting}>
					{submitting ? 'Setting up assistant...' : 'Set up my assistant'}
					<Icon name="arrow-right" size={16} />
				</button>
				<button type="button" class="skip-link" onclick={() => skipStep('kb_setup')}>
					Skip for now
				</button>
			</form>

		{:else if s5Phase === 'review'}
			<div class="step-header">
				<h2>Extracted Knowledge</h2>
				<p>Concepts parsed from your text. You can edit these in Settings later.</p>
			</div>

			{#if s5Concepts.length > 0}
				<div class="concepts-list">
					{#each s5Concepts as concept}
						<div class="badge-blue concept-chip">
							<Icon name="kb" size={13} color="var(--blue-text)" />
							<span>{concept.title ?? concept.name ?? concept}</span>
						</div>
					{/each}
				</div>
			{/if}

			{#if s5QueuedCount > 0}
				<p class="queued-hint">{s5QueuedCount} suggestion{s5QueuedCount !== 1 ? 's' : ''} queued for review in Settings.</p>
			{/if}

			<div class="review-actions">
				<button class="btn-primary full-width" onclick={() => completeStep('kb_setup')} disabled={submitting}>
					Looks good, continue
					<Icon name="arrow-right" size={16} />
				</button>
				<a href="/settings/knowledge-base" class="secondary-link">Edit in settings</a>
			</div>
		{/if}
	</div>

<!-- STEP 6 — Reply Mode -->
{:else if stepNum === 6}
	<div class="step-card glass-panel fade-in">
		<div class="step-header">
			<h2>How should replies be sent?</h2>
			<p>Select reply behavior. You can change this in Settings anytime.</p>
		</div>

		{#if error}
			<div class="error-banner">{error}</div>
		{/if}

		<div class="reply-mode-cards">
			<button
				class="mode-card glass-panel"
				class:selected={s6ReplyMode === 'draft_only'}
				onclick={() => s6ReplyMode = 'draft_only'}
				disabled={submitting}
			>
				<div class="radio-circle" class:checked={s6ReplyMode === 'draft_only'}></div>
				<div class="mode-body">
					<div class="mode-label">Review before sending</div>
					<div class="mode-desc">Assistant drafts replies for your review first. You remain in complete control.</div>
					<span class="badge-blue inline" style="margin-top: 4px; display: inline-block;">Recommended</span>
				</div>
			</button>

			<button
				class="mode-card glass-panel"
				class:selected={s6ReplyMode === 'auto_send'}
				onclick={() => s6ReplyMode = 'auto_send'}
				disabled={submitting}
			>
				<div class="radio-circle" class:checked={s6ReplyMode === 'auto_send'}></div>
				<div class="mode-body">
					<div class="mode-label">Send automatically</div>
					<div class="mode-desc">Replies are sent instantly when confidence is high. You can view logs afterwards.</div>
				</div>
			</button>
		</div>

		<button class="btn-primary full-width" onclick={handleS6Continue} disabled={submitting} style="margin-top: 24px;">
			{submitting ? 'Saving...' : 'Continue'}
			<Icon name="arrow-right" size={16} />
		</button>
	</div>

<!-- STEP 7 — Pipeline Setup -->
{:else if stepNum === 7}
	<div class="step-card glass-panel fade-in">
		<div class="step-header">
			<h2>Your Lead Pipeline</h2>
			<p>We've pre-configured pipeline stages based on your business type.</p>
		</div>

		{#if pipelineStates.length > 0}
			<div class="pipeline-viz">
				{#each pipelineStates as st, i}
					<div class="pipeline-badge" style="border-color: {pipelineColor(st.color)}; color: {pipelineColor(st.color)};">
						{st.label}
					</div>
					{#if i < pipelineStates.length - 1}
						<div class="pipeline-arrow">
							<Icon name="arrow-right" size={14} color="var(--text-muted)" />
						</div>
					{/if}
				{/each}
			</div>
		{:else}
			<div class="pipeline-placeholder">
				<p style="color: var(--text-secondary); font-size: 13.5px;">Pipeline stages loading...</p>
			</div>
		{/if}

		<div class="pipeline-actions">
			<button class="btn-primary full-width" onclick={() => completeStep('pipeline_setup')}>
				Looks good, continue
				<Icon name="arrow-right" size={16} />
			</button>
			<a href="/settings/pipeline" class="secondary-link">Customize pipeline now</a>
		</div>
	</div>

<!-- STEP 8 — Team Invite -->
{:else if stepNum === 8}
	<div class="step-card glass-panel fade-in">
		<div class="step-header">
			<h2>Add your team</h2>
			<p>Invite colleagues now or skip and add them later from Settings.</p>
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
						<span class="badge-blue">{inv.role}</span>
						<span class="invite-sent">
							<Icon name="check" size={14} color="var(--success)" /> Sent
						</span>
					</div>
				{/each}
			</div>
		{/if}

		<div class="team-actions">
			<button class="btn-primary full-width" onclick={() => handleS8Done(false)} disabled={submitting} style="margin-top: 20px;">
				{s8HasInvited ? 'Done adding people' : 'Continue'}
				<Icon name="arrow-right" size={16} />
			</button>
			<button class="skip-link" onclick={() => handleS8Done(true)}>
				Skip for now
			</button>
		</div>
	</div>

<!-- STEP 9 — Done! -->
{:else if stepNum === 9}
	<div class="step-card glass-panel fade-in done-card">
		<div class="checkmark-wrap">
			<div class="checkmark-circle">
				<Icon name="sparkles" size={32} color="var(--blue-text)" />
			</div>
		</div>

		<h2 class="done-headline">Setup Complete!</h2>
		<p class="done-subtext">
			{#if productMode === 'chatbot_only'}
				Your automated assistant is active and ready.
			{:else}
				Your workspace is ready. Head to the inbox to start managing conversations.
			{/if}
		</p>

		<button class="btn-primary full-width done-cta" onclick={() => goto('/inbox')}>
			{productMode === 'chatbot_only' ? 'Go to Inbox' : 'Go to Inbox'}
			<Icon name="arrow-right" size={16} />
		</button>
	</div>
{/if}

<style>
	@keyframes fadeSlideIn {
		from { opacity: 0; transform: translateY(12px); }
		to   { opacity: 1; transform: translateY(0); }
	}
	.fade-in {
		animation: fadeSlideIn 0.3s ease both;
	}

	.step-card {
		max-width: 640px;
		margin: 0 auto;
		padding: 32px 36px;
		position: relative;
	}

	.step-header {
		text-align: center;
		margin-bottom: 28px;
	}
	.step-header h2 {
		font-size: 22px;
		font-weight: 700;
		color: var(--text-primary);
		margin-bottom: 6px;
		line-height: 1.3;
	}
	.step-header p {
		font-size: 13.5px;
		color: var(--text-secondary);
	}

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
		font-size: 11.5px;
		font-weight: 600;
		color: var(--text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.4px;
	}

	.full-width {
		width: 100%;
		height: 42px;
		font-size: 14px;
		font-weight: 500;
	}

	.error-banner {
		padding: 10px 14px;
		background: var(--danger-bg);
		border: 1px solid rgba(235, 87, 87, 0.3);
		border-radius: 6px;
		color: var(--danger);
		font-size: 13px;
		margin-bottom: 12px;
	}

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
		transition: color 0.15s;
	}
	.skip-link:hover { color: var(--text-secondary); }

	.step-footer-link {
		margin-top: 20px;
		text-align: center;
		font-size: 13px;
		color: var(--text-secondary);
	}
	.step-footer-link a {
		color: var(--blue-text);
		text-decoration: none;
		font-weight: 500;
	}

	.loading-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 12px;
		padding: 24px 0;
		color: var(--text-secondary);
		font-size: 13.5px;
	}
	.spinner {
		width: 24px;
		height: 24px;
		border: 2px solid var(--border-color);
		border-top-color: var(--blue-primary);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}
	@keyframes spin { to { transform: rotate(360deg); } }

	.submitting-hint {
		text-align: center;
		font-size: 12.5px;
		color: var(--text-muted);
		margin-top: 12px;
	}

	/* Mode cards */
	.mode-cards {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 14px;
	}
	.mode-card {
		padding: 20px;
		cursor: pointer;
		text-align: left;
		background: #FFFFFF;
		border: 1px solid var(--border-color);
		border-radius: 8px;
		position: relative;
		transition: all 0.15s ease;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.mode-card:hover:not(:disabled) {
		border-color: var(--blue-primary);
		background: var(--bg-hover);
	}
	.mode-card.selected {
		border-color: var(--blue-primary);
		background: var(--blue-bg);
	}

	.mode-icon-box {
		width: 40px;
		height: 40px;
		border-radius: 6px;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.mode-icon-box.blue { background: var(--blue-bg); border: 1px solid var(--blue-border); }
	.mode-icon-box.pink { background: var(--pink-bg); border: 1px solid var(--pink-border); }

	.mode-label { font-size: 14px; font-weight: 600; color: var(--text-primary); }
	.mode-desc { font-size: 12.5px; color: var(--text-secondary); line-height: 1.45; }
	
	.mode-select-indicator {
		width: 18px; height: 18px; border-radius: 50%;
		border: 2px solid var(--border-color);
		position: absolute; top: 16px; right: 16px;
		transition: all 0.15s;
	}
	.mode-select-indicator.active {
		background: var(--blue-primary);
		border-color: var(--blue-primary);
	}

	.recommended-badge {
		position: absolute;
		bottom: 12px; right: 12px;
	}

	/* Business tiles */
	.biz-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 10px;
	}
	.biz-tile {
		padding: 16px 12px;
		cursor: pointer;
		text-align: center;
		background: #FFFFFF;
		border: 1px solid var(--border-color);
		border-radius: 8px;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
		transition: all 0.15s ease;
	}
	.biz-tile:hover:not(:disabled) {
		border-color: var(--blue-primary);
		background: var(--bg-hover);
	}
	.biz-tile.selected {
		border-color: var(--blue-primary);
		background: var(--blue-bg);
	}
	.biz-icon-box {
		width: 36px;
		height: 36px;
		border-radius: 6px;
		background: var(--blue-bg);
		border: 1px solid var(--blue-border);
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.biz-label { font-size: 12.5px; font-weight: 600; color: var(--text-primary); }
	.biz-desc { font-size: 11px; color: var(--text-secondary); line-height: 1.35; }

	/* Channel connect */
	.channel-start {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 16px;
		text-align: center;
	}
	.channel-icon-box {
		width: 54px;
		height: 54px;
		border-radius: 12px;
		background: var(--blue-bg);
		border: 1px solid var(--blue-border);
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.channel-hint {
		font-size: 13.5px;
		color: var(--text-secondary);
		max-width: 320px;
		line-height: 1.5;
	}
	.qr-area {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 20px;
	}
	.qr-placeholder {
		width: 180px; height: 180px;
		border: 1px solid var(--blue-primary);
		border-radius: 8px;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		position: relative;
		overflow: hidden;
		background: var(--blue-bg);
	}
	.qr-scanner-line {
		position: absolute;
		left: 0; right: 0;
		height: 2px;
		background: var(--blue-primary);
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
		gap: 8px;
		width: 100%;
		max-width: 320px;
	}
	.instruction-item {
		display: flex;
		align-items: center;
		gap: 10px;
		font-size: 13px;
		color: var(--text-secondary);
	}
	.step-num {
		width: 22px; height: 22px;
		background: var(--blue-bg);
		color: var(--blue-text);
		border: 1px solid var(--blue-border);
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 11px;
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
		width: 56px; height: 56px;
		border-radius: 50%;
		background: var(--success-bg);
		border: 1px solid var(--success);
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.success-icon-box {
		width: 52px; height: 52px;
		border-radius: 12px;
		background: var(--blue-bg);
		border: 1px solid var(--blue-border);
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.message-preview {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 12px 16px;
		width: 100%;
		max-width: 340px;
		background: var(--bg-hover);
	}
	.preview-text {
		font-size: 13.5px;
		color: var(--text-primary);
	}

	/* KB */
	.kb-textarea {
		resize: vertical;
		min-height: 72px;
		line-height: 1.45;
	}
	.concepts-list {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		margin-bottom: 16px;
		justify-content: center;
	}
	.concept-chip {
		display: flex;
		align-items: center;
		gap: 6px;
	}
	.queued-hint {
		font-size: 13px;
		color: var(--text-secondary);
		margin-bottom: 16px;
		text-align: center;
	}
	.review-actions {
		display: flex;
		flex-direction: column;
		gap: 10px;
		align-items: center;
	}
	.secondary-link {
		font-size: 13px;
		color: var(--blue-text);
		text-decoration: none;
	}
	.secondary-link:hover { text-decoration: underline; }

	/* Reply mode */
	.reply-mode-cards {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.reply-mode-cards .mode-card {
		flex-direction: row;
		align-items: flex-start;
		gap: 14px;
	}
	.radio-circle {
		width: 18px; height: 18px;
		border-radius: 50%;
		border: 2px solid var(--border-color);
		flex-shrink: 0;
		margin-top: 2px;
		transition: all 0.15s;
	}
	.radio-circle.checked {
		background: var(--blue-primary);
		border-color: var(--blue-primary);
	}
	.mode-body {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	/* Pipeline */
	.pipeline-viz {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 8px;
		margin-bottom: 24px;
		justify-content: center;
	}
	.pipeline-badge {
		padding: 5px 12px;
		border-radius: 6px;
		border: 1px solid;
		font-size: 12.5px;
		font-weight: 500;
		background: #FFFFFF;
	}
	.pipeline-arrow {
		display: flex;
		align-items: center;
	}
	.pipeline-placeholder { text-align: center; margin-bottom: 24px; }
	.pipeline-actions {
		display: flex;
		flex-direction: column;
		gap: 10px;
		align-items: center;
	}

	/* Team invite */
	.invite-form { margin-bottom: 14px; }
	.invite-row {
		display: flex;
		gap: 8px;
		align-items: center;
	}
	.role-select { width: 110px; flex-shrink: 0; }
	.invite-btn { flex-shrink: 0; white-space: nowrap; }
	.invite-list {
		display: flex;
		flex-direction: column;
		gap: 6px;
		margin-top: 10px;
	}
	.invite-item {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 8px 12px;
		background: var(--bg-hover);
	}
	.invite-email { font-size: 13px; color: var(--text-primary); flex: 1; }
	.invite-sent {
		font-size: 12px;
		color: var(--success);
		font-weight: 600;
		display: flex;
		align-items: center;
		gap: 4px;
	}
	.team-actions {
		display: flex;
		flex-direction: column;
		gap: 8px;
		align-items: center;
	}

	/* Done */
	.done-card {
		text-align: center;
		padding: 40px 32px;
	}
	.checkmark-wrap {
		display: flex;
		justify-content: center;
		margin-bottom: 16px;
	}
	.checkmark-circle {
		width: 64px; height: 64px;
		border-radius: 50%;
		background: var(--blue-bg);
		border: 1px solid var(--blue-border);
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.done-headline {
		font-size: 24px;
		font-weight: 700;
		color: var(--text-primary);
		margin-bottom: 8px;
	}
	.done-subtext {
		font-size: 14px;
		color: var(--text-secondary);
		line-height: 1.5;
		max-width: 380px;
		margin: 0 auto 24px;
	}
	.done-cta {
		max-width: 280px;
		margin: 0 auto;
	}
</style>
