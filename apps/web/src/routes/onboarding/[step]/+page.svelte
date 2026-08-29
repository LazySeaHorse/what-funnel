<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { apiRequest } from '$lib/api';
	import { decodeWorkspaceSettings } from '$lib/workspace-settings';
	import Icon from '$lib/Icon.svelte';
	import OnboardingChrome from '$lib/components/onboarding/OnboardingChrome.svelte';
	import OnboardingFooter from '$lib/components/onboarding/OnboardingFooter.svelte';
	import BusinessInfoStep from '$lib/components/onboarding/BusinessInfoStep.svelte';
	import ChannelsStep from '$lib/components/onboarding/ChannelsStep.svelte';
	import PipelineStep from '$lib/components/onboarding/PipelineStep.svelte';

	// Step number from route: 1..8
	let stepNum = $derived(parseInt(($page.params as any)?.step ?? '1', 10) || 1);

	let loading = $state(true);
	let submitting = $state(false);
	let error = $state('');
	let pipelineID = $state('');
	let productMode = $state<'full_workspace' | 'chatbot_only'>('full_workspace');

	// Stepper metadata (7 total setup steps)
	const STEP_ITEMS = [
		{ num: 1, label: 'Business info' },
		{ num: 2, label: 'Channels' },
		{ num: 3, label: 'Lead setup' },
		{ num: 4, label: 'Team members' },
		{ num: 5, label: 'AI Assistant' },
		{ num: 6, label: 'Knowledge Base' },
		{ num: 7, label: 'Review & Finish' }
	];
	let visibleStepItems = $derived(productMode === 'chatbot_only' ? STEP_ITEMS.filter((item) => item.num !== 3 && item.num !== 4) : STEP_ITEMS);
	let displayStepNum = $derived(Math.max(1, visibleStepItems.findIndex((item) => item.num === stepNum) + 1));

	// Step 1: Business info
	let s1BusinessName = $state('');
	let s1BusinessType = $state('');
	let s1Timezone = $state('(GMT+00:00) UTC');

	// Step 2: Channels
	let channels = $state([
		{ id: 'whatsapp', name: 'WhatsApp', type: 'matrix_whatsapp', icon: 'whatsapp', connected: false, color: '#25D366' },
		{ id: 'instagram', name: 'Instagram', type: 'matrix_instagram', icon: 'instagram', connected: false, color: '#E1306C' },
		{ id: 'messenger', name: 'Facebook Messenger', type: 'matrix_messenger', icon: 'messenger', connected: false, color: '#0084FF' },
		{ id: 'telegram', name: 'Telegram', type: 'matrix_telegram', icon: 'telegram', connected: false, color: '#229ED9' }
	]);

	// Step 3: Lead pipeline
	let pipelineStages = $state([
		{ key: 'new_lead', label: 'New Lead', color: '#F59E0B' },
		{ key: 'contacted', label: 'Contacted', color: '#3B82F6' },
		{ key: 'follow_up', label: 'Follow-up', color: '#8B5CF6' },
		{ key: 'interested', label: 'Interested', color: '#06B6D4' },
		{ key: 'converted', label: 'Converted', color: '#10B981' }
	]);

	// Step 4: Team members & Workspace slug
	let s4Slug = $state('');
	let s4NewUsername = $state('');
	let s4NewPassword = $state('');
	let s4NewRole = $state<'agent' | 'manager'>('agent');
	let s4Users = $state<Array<{ id: string; username: string; role: string; plaintextPassword?: string }>>([]);
	let s4AddingUser = $state(false);
	let s4UserError = $state('');
	let s4CopiedPassId = $state<string | null>(null);

	// Step 5: AI Assistant
	let s5AiMode = $state<'auto_answer' | 'suggest_only' | 'manual'>('auto_answer');
	let aiProviderConfigured = $state(false);
	let aiProviderApiKey = $state('');
	let aiProviderBaseURL = $state('https://api.openai.com/v1');
	let aiCompletionModel = $state('gpt-4o-mini');
	let aiEmbeddingModel = $state('text-embedding-3-small');

	// Step 6: Knowledge Base
	let s6RawText = $state('');
	let s6Status = $state<'input' | 'processing' | 'results'>('input');
	let s6Concepts = $state<Array<{ id?: string; title: string; type?: string; category?: string; tags?: string[]; body_markdown?: string; content?: string }>>([]);
	let s6Compiling = $state(false);
	let s6Error = $state('');

	function slugify(name: string): string {
		return name
			.toLowerCase()
			.trim()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-+|-+$/g, '');
	}

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
			const account = await apiRequest('/workspace/account');
			productMode = account?.product_mode === 'chatbot_only' ? 'chatbot_only' : 'full_workspace';
			const [pipelines, aiStatus] = await Promise.all([
				productMode === 'full_workspace' ? apiRequest('/workspace/pipelines') : Promise.resolve([]),
				apiRequest('/workspace/account/ai-config/status')
			]);
			aiProviderConfigured = aiStatus?.configured === true;
			if (aiStatus?.base_url) aiProviderBaseURL = aiStatus.base_url;
			if (aiStatus?.completion_model) aiCompletionModel = aiStatus.completion_model;
			if (aiStatus?.embedding_model) aiEmbeddingModel = aiStatus.embedding_model;
			if (account.name) {
				s1BusinessName = account.name;
				if (!s4Slug) s4Slug = slugify(account.name);
			}
			const settings = decodeWorkspaceSettings(account.settings);
			if (settings.business_type) s1BusinessType = settings.business_type;
			if (settings.timezone) s1Timezone = settings.timezone;
			if (settings.ai_enabled === false) s5AiMode = 'manual';
			else if (settings.ai_reply_mode_default === 'auto_send') s5AiMode = 'auto_answer';
			else if (settings.ai_reply_mode_default === 'draft_only') s5AiMode = 'suggest_only';

			if (productMode === 'full_workspace') {
				const pipeline = Array.isArray(pipelines) ? pipelines[0] : null;
				if (!pipeline?.id) throw new Error('Your default lead pipeline could not be loaded.');
				pipelineID = pipeline.id;
				if (Array.isArray(pipeline.states) && pipeline.states.length > 0) pipelineStages = pipeline.states;
			}

			try {
				const slugData = await apiRequest('/workspace/account/slug');
				if (slugData?.slug) s4Slug = slugData.slug;
			} catch {}

			if (productMode === 'full_workspace') try {
				const userList = await apiRequest('/workspace/users');
				if (Array.isArray(userList)) {
					s4Users = userList
						.filter((u: any) => u.username)
						.map((u: any) => ({
							id: u.id,
							username: u.username,
							role: u.role
						}));
				}
			} catch {}

			if (productMode === 'chatbot_only' && (stepNum === 3 || stepNum === 4)) {
				await skipWorkspaceOnlySteps();
				goToStep(5);
				return;
			}

			const chList = await apiRequest('/channels');
			if (Array.isArray(chList) && chList.length > 0) {
				for (const c of chList) {
					const found = channels.find((item) => item.type === c.type);
					if (found) found.connected = (c.status === 'connected');
				}
			}
		} catch (err: any) {
			error = err?.message || 'We could not load your saved setup. Refresh and try again.';
		} finally {
			loading = false;
		}
	});

	// ─────────────────────────────────────────────────────────────
	// Step Handlers
	// ─────────────────────────────────────────────────────────────
	function goToStep(num: number) {
		if (productMode === 'chatbot_only' && (num === 3 || num === 4)) num = 5;
		goto(`/onboarding/${num}`);
	}

	async function skipWorkspaceOnlySteps() {
		await Promise.all([
			apiRequest('/onboarding/status', { method: 'PATCH', body: { step: 'pipeline_setup', action: 'skip' } }),
			apiRequest('/onboarding/status', { method: 'PATCH', body: { step: 'team_setup', action: 'skip' } })
		]);
	}

	function handleBack() {
		if (stepNum === 6 && s6Status === 'results') {
			s6Status = 'input';
			return;
		}
		if (productMode === 'chatbot_only' && stepNum === 5) {
			goToStep(2);
		} else if (stepNum > 1) {
			goToStep(stepNum - 1);
		} else {
			goto('/login');
		}
	}

	function generatePassword() {
		const chars = 'abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%';
		let pass = '';
		for (let i = 0; i < 12; i++) {
			pass += chars.charAt(Math.floor(Math.random() * chars.length));
		}
		s4NewPassword = pass;
	}

	async function copyPassword(id: string, pass: string) {
		try {
			await navigator.clipboard.writeText(pass);
			s4CopiedPassId = id;
			setTimeout(() => {
				if (s4CopiedPassId === id) s4CopiedPassId = null;
			}, 2000);
		} catch {}
	}

	async function addTeamMember() {
		s4UserError = '';
		if (!s4NewUsername.trim()) {
			s4UserError = 'Username is required.';
			return;
		}
		if (!s4NewPassword.trim()) {
			s4UserError = 'Password is required.';
			return;
		}
		s4AddingUser = true;
		try {
			const res = await apiRequest('/workspace/users', {
				method: 'POST',
				body: {
					username: s4NewUsername.trim(),
					password: s4NewPassword.trim(),
					role: s4NewRole
				}
			});
			s4Users = [
				...s4Users,
				{
					id: res.id,
					username: res.username || s4NewUsername.trim(),
					role: res.role || s4NewRole,
					plaintextPassword: res.password || s4NewPassword.trim()
				}
			];
			s4NewUsername = '';
			s4NewPassword = '';
			s4NewRole = 'agent';
		} catch (err: any) {
			s4UserError = err?.message || 'Failed to add user.';
		} finally {
			s4AddingUser = false;
		}
	}

	async function removeTeamMember(id: string) {
		try {
			await apiRequest(`/workspace/users/${id}`, { method: 'DELETE' });
			s4Users = s4Users.filter(u => u.id !== id);
		} catch (err: any) {
			s4UserError = err?.message || 'Failed to remove user.';
		}
	}

	async function startCompilingKB() {
		if (!s6RawText.trim()) {
			await apiRequest('/onboarding/status', {
				method: 'PATCH',
				body: { step: 'kb_setup', action: 'skip' }
			});
			goToStep(7);
			return;
		}

		s6Compiling = true;
		s6Status = 'processing';
		s6Error = '';

		try {
			const res = await apiRequest('/api/kb/compile-paste', {
				method: 'POST',
				body: { raw_text: s6RawText.trim() }
			});

			if (Array.isArray(res?.added_concepts) && res.added_concepts.length > 0) {
				s6Concepts = res.added_concepts;
			} else {
				const fetched = await apiRequest('/api/kb/concepts');
				if (Array.isArray(fetched?.concepts) && fetched.concepts.length > 0) {
					s6Concepts = fetched.concepts;
				} else {
					throw new Error('The AI compiler returned no knowledge concepts. Check your AI provider settings and try again.');
				}
			}

			await apiRequest('/onboarding/status', {
				method: 'PATCH',
				body: { step: 'kb_setup', action: 'complete' }
			});

			s6Status = 'results';
		} catch (err: any) {
			s6Error = err?.message || 'Failed to process knowledge text. Check your AI provider settings and try again.';
			s6Concepts = [];
			s6Status = 'input';
		} finally {
			s6Compiling = false;
		}
	}

	async function skipWaitingToDashboard() {
		try {
			await apiRequest('/onboarding/status', {
				method: 'PATCH',
				body: { step: 'done', action: 'complete' }
			});
			goto('/inbox');
		} catch (err: any) {
			s6Error = err?.message || 'Could not finish setup. Please try again.';
		}
	}

	function appendTemplateChunk(label: string, text: string) {
		if (s6RawText.includes(label)) return;
		s6RawText = s6RawText.trim() + `\n\n${label}:\n${text}`;
	}

	async function handleContinue() {
		error = '';
		submitting = true;

		try {
			if (stepNum === 1) {
				if (!s1BusinessName.trim()) throw new Error('Business name is required.');
				await apiRequest('/workspace/account', {
					method: 'PATCH',
					body: { name: s1BusinessName.trim() }
				});
				await apiRequest('/workspace/account/settings', {
					method: 'PATCH',
					body: { business_type: s1BusinessType, timezone: s1Timezone }
				});

				if (!s4Slug) {
					s4Slug = slugify(s1BusinessName);
				}

				await apiRequest('/onboarding/status', {
					method: 'PATCH',
					body: { step: 'business_basics', action: 'complete' }
				});

				goToStep(2);
			} else if (stepNum === 2) {
				await apiRequest('/onboarding/status', {
					method: 'PATCH',
					body: { step: 'channel_connect', action: channels.some((channel) => channel.connected) ? 'complete' : 'skip' }
				});

				if (productMode === 'chatbot_only') {
					await skipWorkspaceOnlySteps();
					goToStep(5);
				} else {
					goToStep(3);
				}
			} else if (stepNum === 3) {
				if (!pipelineID) throw new Error('Your default lead pipeline is unavailable. Refresh and try again.');
				await apiRequest(`/workspace/pipelines/${pipelineID}`, {
					method: 'PUT',
					body: {
						name: 'Default Pipeline',
						states: pipelineStages
					}
				});

				await apiRequest('/onboarding/status', {
					method: 'PATCH',
					body: { step: 'pipeline_setup', action: 'complete' }
				});

				goToStep(4);
			} else if (stepNum === 4) {
				const effectiveSlug = s4Slug.trim() || slugify(s1BusinessName) || 'workspace';
				await apiRequest('/workspace/account/slug', {
					method: 'PUT',
					body: { slug: effectiveSlug }
				});
				s4Slug = effectiveSlug;

				await apiRequest('/onboarding/status', {
					method: 'PATCH',
					body: { step: 'team_setup', action: s4Users.length > 0 ? 'complete' : 'skip' }
				});

				goToStep(5);
			} else if (stepNum === 5) {
				const replyMode = s5AiMode === 'auto_answer' ? 'auto_send' : 'draft_only';
				if (s5AiMode !== 'manual' && !aiProviderConfigured && !aiProviderApiKey.trim()) {
					throw new Error('Add your AI provider API key, or choose Manual only.');
				}
				if (s5AiMode !== 'manual') {
					await apiRequest('/workspace/account/ai-config', {
						method: 'PUT',
						body: {
							config: JSON.stringify({
								api_key: aiProviderApiKey.trim(),
								base_url: aiProviderBaseURL.trim(),
								completion_model: aiCompletionModel.trim(),
								embedding_model: aiEmbeddingModel.trim()
							})
						}
					});
					aiProviderConfigured = true;
					aiProviderApiKey = '';
				}
				await apiRequest('/workspace/account/settings', {
					method: 'PATCH',
					body: { ai_enabled: s5AiMode !== 'manual', ai_reply_mode_default: replyMode }
				});

				await apiRequest('/onboarding/status', {
					method: 'PATCH',
					body: { step: 'reply_mode', action: 'complete' }
				});

				goToStep(6);
			} else if (stepNum === 6) {
				if (s6Status === 'input') {
					await startCompilingKB();
				} else {
					goToStep(7);
				}
			} else if (stepNum === 7) {
				await apiRequest('/onboarding/status', {
					method: 'PATCH',
					body: { step: 'review_finish', action: 'complete' }
				});
				await apiRequest('/onboarding/status', {
					method: 'PATCH',
					body: { step: 'done', action: 'complete' }
				});

				goToStep(8);
			}
		} catch (err: any) {
			error = err?.message || 'Failed to save step settings. Please try again.';
		} finally {
			submitting = false;
		}
	}

	function toggleChannel(_ch: any) {
		goto('/inbox?tab=settings');
	}

	let connectedChannelsText = $derived(() => {
		const conn = channels.filter(c => c.connected).map(c => c.name);
		return conn.length > 0 ? conn.join(', ') : 'WhatsApp, Instagram';
	});

	let aiModeLabel = $derived(() => {
		if (s5AiMode === 'auto_answer') return 'Auto answer when confident';
		if (s5AiMode === 'suggest_only') return 'Suggest replies only';
		return 'Manual only';
	});

	let kbTopicsSummary = $derived(() => {
		if (s6Concepts.length > 0) {
			return `${s6Concepts.length} concepts organized by AI`;
		}
		return 'Business information compiled';
	});
</script>

<svelte:head>
	<title>Onboarding — What Funnel</title>
</svelte:head>

{#if stepNum >= 1 && stepNum <= 8}
	<!-- FULL-SCREEN ONBOARDING INTERFACE (Pure Tailwind) -->
	<div class="h-[100dvh] w-full bg-white flex flex-col lg:flex-row overflow-hidden font-sans text-slate-800 antialiased relative">
		
		<OnboardingChrome stepNum={stepNum} stepItems={visibleStepItems} onStep={goToStep} />

		<!-- Right Main Form Content Column: Takes Up Full Remaining Width -->
		<div class="flex-1 relative overflow-y-auto bg-white flex flex-col justify-between min-h-0 p-5 sm:p-10 lg:p-12 pb-24 sm:pb-8">
			<div class="w-full flex flex-col min-h-full justify-between relative z-10">
				<div class="w-full">
					<!-- Mobile Top Bar: Back Button & Step Progress Stepper -->
					<div class="lg:hidden flex items-center justify-between pb-4 mb-5 border-b border-slate-100">
						<button
							type="button"
							class="p-2 -ml-2 text-slate-500 hover:text-slate-900 rounded-lg active:bg-slate-100 transition cursor-pointer"
							onclick={handleBack}
							aria-label="Go back"
						>
							<svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
								<path d="M15 18l-6-6 6-6" />
							</svg>
						</button>

						{#if stepNum <= 7}
							<div class="flex items-center gap-1.5" aria-label={`Step ${displayStepNum} of ${visibleStepItems.length}`}>
								{#each visibleStepItems as item, idx}
									<div class="h-1.5 rounded-full transition-all duration-200 {item.num === stepNum ? 'w-5 bg-blue-600' : idx < displayStepNum - 1 ? 'w-2.5 bg-blue-600' : 'w-2 bg-slate-200'}"></div>
								{/each}
							</div>
						{:else}
							<div class="text-xs font-medium text-slate-500">Setup Complete</div>
						{/if}

						<div class="w-9"></div>
					</div>

					<!-- Steps own their presentation and form-local behavior; this page coordinates persistence and navigation. -->
					{#if stepNum === 1}
						<BusinessInfoStep step={displayStepNum} totalSteps={visibleStepItems.length} bind:businessName={s1BusinessName} bind:businessType={s1BusinessType} bind:timezone={s1Timezone} />
					{:else if stepNum === 2}
						<ChannelsStep step={displayStepNum} totalSteps={visibleStepItems.length} {channels} onConnect={toggleChannel} />
					{:else if stepNum === 3}
						<PipelineStep step={displayStepNum} totalSteps={visibleStepItems.length} bind:stages={pipelineStages} />
					<!-- STEP 4: TEAM MEMBERS & WORKSPACE SLUG -->
					{:else if stepNum === 4}
						<div class="text-center lg:text-left mb-6">
							<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {displayStepNum} of {visibleStepItems.length}</div>
							<h2 class="text-2xl sm:text-3xl font-medium text-slate-900 tracking-tight mb-1">Add your team members</h2>
							<p class="text-sm text-slate-500 font-normal">Set your workspace login prefix and add team agents or managers.</p>
						</div>

						<div class="space-y-6 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
							<!-- Workspace Slug Setup -->
							<div class="p-4 sm:p-5 bg-slate-50/80 border border-slate-200 rounded-2xl space-y-3">
								<div class="flex items-center justify-between">
									<label for="workspace-slug" class="block text-xs font-medium text-slate-900">Workspace login prefix (slug)</label>
									<span class="text-[11px] font-medium text-blue-600 bg-blue-50 px-2 py-0.5 rounded-md">Common prefix</span>
								</div>
								<div class="relative">
									<input
										id="workspace-slug"
										type="text"
										class="w-full px-4 py-2.5 bg-white border border-slate-200 rounded-xl text-sm font-mono text-slate-900 placeholder:text-slate-400 focus:border-blue-600 focus:ring-2 focus:ring-blue-100 outline-none transition-all"
										placeholder="company-name"
										bind:value={s4Slug}
									/>
								</div>
								<p class="text-xs text-slate-500 font-normal">
									Team members will log in using: <span class="font-mono font-medium text-slate-800 bg-white px-2 py-0.5 rounded border border-slate-200">{s4Slug || 'your-company'}-[username]</span>
								</p>
							</div>

							<!-- Add Team Member Form -->
							<div class="p-4 sm:p-5 bg-white border border-slate-200 rounded-2xl space-y-4">
								<h3 class="text-sm font-medium text-slate-900">Add a team member</h3>
								
								<div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
									<div>
										<label for="new-member-username" class="block text-xs font-medium text-slate-700 mb-1">Username</label>
										<input
											id="new-member-username"
											type="text"
											class="w-full px-3 py-2 bg-white border border-slate-200 rounded-xl text-xs text-slate-900 placeholder:text-slate-400 focus:border-blue-600 focus:ring-1 focus:ring-blue-100 outline-none"
											placeholder="e.g. john"
											bind:value={s4NewUsername}
										/>
									</div>

									<div>
										<div class="flex items-center justify-between mb-1">
											<label for="new-member-password" class="block text-xs font-medium text-slate-700">Password</label>
											<button type="button" class="text-[10px] text-blue-600 hover:underline cursor-pointer" onclick={generatePassword}>Generate</button>
										</div>
										<input
											id="new-member-password"
											type="text"
											class="w-full px-3 py-2 bg-white border border-slate-200 rounded-xl text-xs text-slate-900 font-mono placeholder:text-slate-400 focus:border-blue-600 focus:ring-1 focus:ring-blue-100 outline-none"
											placeholder="Password"
											bind:value={s4NewPassword}
										/>
									</div>

									<div>
										<label for="new-member-role" class="block text-xs font-medium text-slate-700 mb-1">Role</label>
										<div class="flex gap-2">
											<select
												id="new-member-role"
												bind:value={s4NewRole}
												class="flex-1 px-3 py-2 bg-white border border-slate-200 rounded-xl text-xs text-slate-900 focus:border-blue-600 focus:ring-1 focus:ring-blue-100 outline-none cursor-pointer"
											>
												<option value="agent">Agent</option>
												<option value="manager">Manager</option>
											</select>
											<button
												type="button"
												class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium rounded-xl transition cursor-pointer disabled:opacity-50 shrink-0 flex items-center gap-1.5"
												onclick={addTeamMember}
												disabled={s4AddingUser || !s4NewUsername.trim() || !s4NewPassword.trim()}
											>
												<Icon name="plus" size={14} color="#FFFFFF" />
												<span>Add</span>
											</button>
										</div>
									</div>
								</div>

								{#if s4UserError}
									<p class="text-xs text-rose-600 font-medium">{s4UserError}</p>
								{/if}
							</div>

							<!-- Created Members List -->
							{#if s4Users.length > 0}
								<div class="border border-slate-200 rounded-2xl overflow-hidden divide-y divide-slate-100 bg-white">
									<div class="px-4 py-2.5 bg-slate-50/70 text-xs font-medium text-slate-500">
										Added team members ({s4Users.length})
									</div>
									{#each s4Users as member}
										<div class="p-3.5 sm:p-4 flex items-center justify-between gap-3">
											<div class="flex items-center gap-3 min-w-0">
												<div class="w-8 h-8 rounded-full bg-blue-100 text-blue-700 font-medium flex items-center justify-center text-xs shrink-0">
													{member.username.charAt(0).toUpperCase()}
												</div>
												<div class="min-w-0">
													<div class="font-medium text-xs sm:text-sm text-slate-900 truncate">
														{member.username}
													</div>
													<div class="text-[11px] text-slate-400 font-mono">
														{(s4Slug || 'prefix') + '-' + member.username}
													</div>
												</div>
											</div>

											<div class="flex items-center gap-2 shrink-0">
												<span class="px-2 py-0.5 rounded-md text-[11px] font-medium {member.role === 'manager' ? 'bg-purple-100 text-purple-700' : 'bg-slate-100 text-slate-700'} capitalize">
													{member.role}
												</span>

												{#if member.plaintextPassword}
													<button
														type="button"
														class="flex items-center gap-1 px-2 py-1 rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-700 text-[11px] font-mono transition cursor-pointer"
														onclick={() => copyPassword(member.id, member.plaintextPassword!)}
														title="Copy login password"
													>
														<span>{member.plaintextPassword}</span>
														<Icon name={s4CopiedPassId === member.id ? 'check' : 'copy'} size={12} color={s4CopiedPassId === member.id ? '#10B981' : '#64748B'} />
													</button>
												{/if}

												<button
													type="button"
													class="p-1 text-slate-400 hover:text-rose-600 rounded-lg hover:bg-rose-50 transition cursor-pointer"
													onclick={() => removeTeamMember(member.id)}
													title="Remove user"
												>
													<Icon name="trash" size={14} color="currentColor" />
												</button>
											</div>
										</div>
									{/each}
								</div>
							{/if}

							<p class="text-xs text-slate-400 text-center lg:text-left">
								You can also add more agents or managers later in Settings.
							</p>
						</div>

					<!-- STEP 5: AI ASSISTANT -->
					{:else if stepNum === 5}
						<div class="text-center lg:text-left mb-6">
							<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {displayStepNum} of {visibleStepItems.length}</div>
							<h2 class="text-2xl sm:text-3xl font-medium text-slate-900 tracking-tight mb-1">Meet your AI Assistant</h2>
							<p class="text-sm text-slate-500 font-normal">How should your assistant handle conversations?</p>
						</div>

						<div class="space-y-3 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
							<!-- Option 1: Auto answer -->
							<button
								type="button"
								class="w-full text-left p-4 rounded-xl border transition-all cursor-pointer flex items-start justify-between {s5AiMode === 'auto_answer' ? 'border-blue-600 bg-blue-50/40 ring-1 ring-blue-600' : 'border-slate-200 bg-white hover:border-slate-300'}"
								onclick={() => s5AiMode = 'auto_answer'}
							>
								<div class="flex items-start gap-3.5">
									<div class="w-8 h-8 rounded-lg {s5AiMode === 'auto_answer' ? 'bg-blue-600 text-white' : 'bg-slate-100 text-slate-500'} flex items-center justify-center shrink-0 mt-0.5">
										<Icon name="bot" size={18} color="currentColor" />
									</div>
									<div>
										<div class="flex items-center gap-2">
											<span class="text-sm font-medium text-slate-900">Auto answer when confident</span>
											<span class="px-2 py-0.5 rounded-md bg-blue-100 text-blue-700 text-[10px] font-medium">Recommended</span>
										</div>
										<p class="text-xs text-slate-500 mt-1 leading-relaxed font-normal">AI will answer customer questions automatically when confidence is high.</p>
									</div>
								</div>
								<div class="w-4 h-4 rounded-full border flex items-center justify-center mt-1 shrink-0 {s5AiMode === 'auto_answer' ? 'border-blue-600' : 'border-slate-300'}">
									{#if s5AiMode === 'auto_answer'}
										<div class="w-2 h-2 rounded-full bg-blue-600"></div>
									{/if}
								</div>
							</button>

							<!-- Option 2: Suggest replies only -->
							<button
								type="button"
								class="w-full text-left p-4 rounded-xl border transition-all cursor-pointer flex items-start justify-between {s5AiMode === 'suggest_only' ? 'border-blue-600 bg-blue-50/40 ring-1 ring-blue-600' : 'border-slate-200 bg-white hover:border-slate-300'}"
								onclick={() => s5AiMode = 'suggest_only'}
							>
								<div class="flex items-start gap-3.5">
									<div class="w-8 h-8 rounded-lg {s5AiMode === 'suggest_only' ? 'bg-blue-600 text-white' : 'bg-slate-100 text-slate-500'} flex items-center justify-center shrink-0 mt-0.5">
										<Icon name="sparkles" size={18} color="currentColor" />
									</div>
									<div>
										<span class="text-sm font-medium text-slate-900">Suggest replies only</span>
										<p class="text-xs text-slate-500 mt-1 leading-relaxed font-normal">AI will draft suggested responses for your team to review and dispatch.</p>
									</div>
								</div>
								<div class="w-4 h-4 rounded-full border flex items-center justify-center mt-1 shrink-0 {s5AiMode === 'suggest_only' ? 'border-blue-600' : 'border-slate-300'}">
									{#if s5AiMode === 'suggest_only'}
										<div class="w-2 h-2 rounded-full bg-blue-600"></div>
									{/if}
								</div>
							</button>

							<!-- Option 3: Manual only -->
							<button
								type="button"
								class="w-full text-left p-4 rounded-xl border transition-all cursor-pointer flex items-start justify-between {s5AiMode === 'manual' ? 'border-blue-600 bg-blue-50/40 ring-1 ring-blue-600' : 'border-slate-200 bg-white hover:border-slate-300'}"
								onclick={() => s5AiMode = 'manual'}
							>
								<div class="flex items-start gap-3.5">
									<div class="w-8 h-8 rounded-lg {s5AiMode === 'manual' ? 'bg-blue-600 text-white' : 'bg-slate-100 text-slate-500'} flex items-center justify-center shrink-0 mt-0.5">
										<Icon name="edit" size={18} color="currentColor" />
									</div>
									<div>
										<span class="text-sm font-medium text-slate-900">Manual only</span>
										<p class="text-xs text-slate-500 mt-1 leading-relaxed font-normal">AI will not send messages automatically. All replies are composed manually.</p>
									</div>
								</div>
								<div class="w-4 h-4 rounded-full border flex items-center justify-center mt-1 shrink-0 {s5AiMode === 'manual' ? 'border-blue-600' : 'border-slate-300'}">
									{#if s5AiMode === 'manual'}
										<div class="w-2 h-2 rounded-full bg-blue-600"></div>
									{/if}
								</div>
							</button>

							{#if s5AiMode !== 'manual'}
								<div class="mt-4 space-y-4 rounded-xl border border-slate-200 bg-slate-50/70 p-4">
									<div>
										<div class="flex items-center justify-between gap-3">
											<h3 class="text-sm font-medium text-slate-900">AI provider</h3>
											{#if aiProviderConfigured}<span class="text-[11px] font-medium text-emerald-700">Configured</span>{/if}
										</div>
										<p class="mt-1 text-xs leading-relaxed text-slate-500">Credentials are encrypted before storage. What Funnel will not generate AI content until a provider is configured.</p>
									</div>
									<div class="space-y-1.5">
										<label for="ai-provider-key" class="block text-xs font-medium text-slate-700">API key {aiProviderConfigured ? '(leave blank to keep current key)' : ''}</label>
										<input id="ai-provider-key" type="password" autocomplete="new-password" bind:value={aiProviderApiKey} class="wf-input" placeholder={aiProviderConfigured ? 'Configured' : 'Required'} />
										{#if !aiProviderConfigured && !aiProviderApiKey.trim()}
											<p class="text-[11px] text-amber-700">Add your AI provider API key, or choose Manual only.</p>
										{/if}
									</div>
									<div class="space-y-1.5">
										<label for="ai-provider-url" class="block text-xs font-medium text-slate-700">OpenAI-compatible base URL</label>
										<input id="ai-provider-url" type="url" bind:value={aiProviderBaseURL} class="wf-input" required />
									</div>
									<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
										<label class="space-y-1.5 text-xs font-medium text-slate-700">Completion model<input aria-label="Completion model" bind:value={aiCompletionModel} class="wf-input" required /></label>
										<label class="space-y-1.5 text-xs font-medium text-slate-700">Embedding model<input aria-label="Embedding model" bind:value={aiEmbeddingModel} class="wf-input" required /></label>
									</div>
								</div>
							{/if}
						</div>

					<!-- STEP 6: KNOWLEDGE BASE -->
					{:else if stepNum === 6}
						<div class="text-center lg:text-left mb-6">
							<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {displayStepNum} of {visibleStepItems.length}</div>
							<h2 class="text-2xl font-medium text-slate-900 tracking-tight mb-1">Teach your AI assistant</h2>
							<p class="text-sm text-slate-500 font-normal max-w-lg lg:max-w-none mx-auto lg:mx-0">Add business notes, price lists, FAQs, hours, or policies. The AI compiler organizes it automatically.</p>
						</div>

						{#if s6Status === 'input'}
							<div class="space-y-4 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
								<div class="flex flex-wrap items-center justify-center lg:justify-start gap-2">
									<span class="text-xs font-medium text-slate-500">Quick templates:</span>
									<button type="button" class="px-2.5 py-1 text-xs font-medium rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-700 transition" onclick={() => appendTemplateChunk('Services & Pricing', '- Standard service: $50\n- Premium package: $120')}>
										+ Pricing
									</button>
									<button type="button" class="px-2.5 py-1 text-xs font-medium rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-700 transition" onclick={() => appendTemplateChunk('Business Hours', '- Monday–Friday: 9:00 AM – 6:00 PM\n- Saturday: 10:00 AM – 4:00 PM')}>
										+ Hours
									</button>
									<button type="button" class="px-2.5 py-1 text-xs font-medium rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-700 transition" onclick={() => appendTemplateChunk('Cancellation Policy', '- 24-hour advance notice required')}>
										+ Policy
									</button>
									<button type="button" class="px-2.5 py-1 text-xs font-medium rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-700 transition" onclick={() => appendTemplateChunk('FAQs', '- Free customer parking on-site\n- Walk-ins accepted based on availability')}>
										+ FAQs
									</button>
								</div>

								<textarea
									class="w-full h-52 p-4 bg-white border border-slate-200 rounded-xl text-xs sm:text-sm text-slate-900 placeholder:text-slate-400 focus:border-blue-600 focus:ring-2 focus:ring-blue-100 outline-none leading-relaxed resize-none font-normal"
									placeholder="Paste raw business info, services, pricing, business hours, cancellation rules, FAQ answers, or message templates..."
									bind:value={s6RawText}
								></textarea>
							</div>

						{:else if s6Status === 'processing'}
							<div class="py-12 flex flex-col items-center justify-center text-center space-y-4 w-full">
								<div class="w-12 h-12 rounded-2xl bg-blue-50 border border-blue-100 flex items-center justify-center text-blue-600">
									<Icon name="sparkles" size={24} color="currentColor" />
								</div>
								<h2 class="text-xl font-medium text-slate-900">Organizing your knowledge...</h2>
								<p class="text-xs sm:text-sm text-slate-500 max-w-sm lg:max-w-none font-normal">
									Structuring raw business notes into categorized concepts and FAQ patterns.
								</p>

								<button
									type="button"
									class="mt-4 px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-700 text-xs font-medium rounded-xl transition"
									onclick={skipWaitingToDashboard}
								>
									Skip waiting & go to Dashboard →
								</button>
							</div>

						{:else if s6Status === 'results'}
							<div class="flex items-center justify-between mb-4 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
								<div>
									<h2 class="text-2xl font-medium text-slate-900 tracking-tight">Structured Knowledge</h2>
									<p class="text-sm text-slate-500 font-normal">Concepts inferred from your business notes:</p>
								</div>
								<button type="button" class="px-3 py-1.5 text-xs font-medium text-blue-600 hover:bg-blue-50 rounded-lg transition" onclick={() => s6Status = 'input'}>
									Edit raw notes
								</button>
							</div>

							<div class="grid grid-cols-1 sm:grid-cols-2 gap-3 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
								{#each s6Concepts as concept}
									<div class="p-4 bg-slate-50/70 border border-slate-200 rounded-xl space-y-2">
										<div class="flex items-center justify-between">
											<span class="text-xs font-medium text-slate-900">{concept.title || 'Knowledge Concept'}</span>
											<span class="px-2 py-0.5 rounded-md bg-blue-100 text-blue-700 text-[10px] font-medium">{concept.category || concept.type || 'Rule'}</span>
										</div>
										<p class="text-xs text-slate-600 line-clamp-3 font-normal">{concept.body_markdown || concept.content || ''}</p>
									</div>
								{/each}
							</div>
						{/if}

					<!-- STEP 7: REVIEW AND FINISH -->
					{:else if stepNum === 7}
						<div class="text-center lg:text-left mb-6">
							<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {displayStepNum} of {visibleStepItems.length}</div>
							<h2 class="text-2xl font-medium text-slate-900 tracking-tight mb-1">Review and finish</h2>
							<p class="text-sm text-slate-500 font-normal">Here’s a summary of your workspace setup.</p>
						</div>

						<div class="space-y-3 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
							<!-- Business -->
							<div class="flex items-center justify-between p-4 bg-white border border-slate-200 rounded-xl w-full">
								<div class="flex items-center gap-3">
									<div class="w-8 h-8 rounded-lg bg-blue-50 text-blue-600 flex items-center justify-center shrink-0">
										<Icon name="store" size={16} color="currentColor" />
									</div>
									<div>
										<div class="text-[11px] font-medium text-slate-400 uppercase">Business</div>
										<div class="text-sm font-medium text-slate-900">{s1BusinessName || 'Your workspace'}</div>
									</div>
								</div>
								<button type="button" class="text-xs font-medium text-blue-600 hover:underline" onclick={() => goToStep(1)}>Edit</button>
							</div>

							<!-- Channels -->
							<div class="flex items-center justify-between p-4 bg-white border border-slate-200 rounded-xl w-full">
								<div class="flex items-center gap-3">
									<div class="w-8 h-8 rounded-lg bg-blue-50 text-blue-600 flex items-center justify-center shrink-0">
										<Icon name="chat" size={16} color="currentColor" />
									</div>
									<div>
										<div class="text-[11px] font-medium text-slate-400 uppercase">Channels</div>
										<div class="text-sm font-medium text-slate-900">{connectedChannelsText()}</div>
									</div>
								</div>
								<button type="button" class="text-xs font-medium text-blue-600 hover:underline" onclick={() => goToStep(2)}>Edit</button>
							</div>

							{#if productMode === 'full_workspace'}
							<!-- Lead Pipeline -->
							<div class="flex items-center justify-between p-4 bg-white border border-slate-200 rounded-xl w-full">
								<div class="flex items-center gap-3">
									<div class="w-8 h-8 rounded-lg bg-blue-50 text-blue-600 flex items-center justify-center shrink-0">
										<Icon name="pipeline" size={16} color="currentColor" />
									</div>
									<div>
										<div class="text-[11px] font-medium text-slate-400 uppercase">Lead pipeline</div>
										<div class="text-sm font-medium text-slate-900">{pipelineStages.length} stages configured</div>
									</div>
								</div>
								<button type="button" class="text-xs font-medium text-blue-600 hover:underline" onclick={() => goToStep(3)}>Edit</button>
							</div>

							<!-- Team Members -->
							<div class="flex items-center justify-between p-4 bg-white border border-slate-200 rounded-xl w-full">
								<div class="flex items-center gap-3">
									<div class="w-8 h-8 rounded-lg bg-blue-50 text-blue-600 flex items-center justify-center shrink-0">
										<Icon name="users" size={16} color="currentColor" />
									</div>
									<div>
										<div class="text-[11px] font-medium text-slate-400 uppercase">Team</div>
										<div class="text-sm font-medium text-slate-900">{s4Users.length > 0 ? `${s4Users.length} team member(s) added (prefix: ${s4Slug || 'default'})` : `Prefix: ${s4Slug || 'default'} (no extra members)`}</div>
									</div>
								</div>
								<button type="button" class="text-xs font-medium text-blue-600 hover:underline" onclick={() => goToStep(4)}>Edit</button>
							</div>
							{/if}

							<!-- AI Assistant -->
							<div class="flex items-center justify-between p-4 bg-white border border-slate-200 rounded-xl w-full">
								<div class="flex items-center gap-3">
									<div class="w-8 h-8 rounded-lg bg-blue-50 text-blue-600 flex items-center justify-center shrink-0">
										<Icon name="bot" size={16} color="currentColor" />
									</div>
									<div>
										<div class="text-[11px] font-medium text-slate-400 uppercase">AI Assistant</div>
										<div class="text-sm font-medium text-slate-900">{aiModeLabel()}</div>
									</div>
								</div>
								<button type="button" class="text-xs font-medium text-blue-600 hover:underline" onclick={() => goToStep(5)}>Edit</button>
							</div>

							<!-- Knowledge Base -->
							<div class="flex items-center justify-between p-4 bg-white border border-slate-200 rounded-xl w-full">
								<div class="flex items-center gap-3">
									<div class="w-8 h-8 rounded-lg bg-blue-50 text-blue-600 flex items-center justify-center shrink-0">
										<Icon name="book" size={16} color="currentColor" />
									</div>
									<div>
										<div class="text-[11px] font-medium text-slate-400 uppercase">Knowledge Base</div>
										<div class="text-sm font-medium text-slate-900">{kbTopicsSummary()}</div>
									</div>
								</div>
								<button type="button" class="text-xs font-medium text-blue-600 hover:underline" onclick={() => goToStep(6)}>Edit</button>
							</div>
						</div>

					<!-- STEP 8: ALL SET! READY TO GO -->
					{:else if stepNum === 8}
						<div class="flex flex-col items-center lg:items-start text-center lg:text-left max-w-lg lg:max-w-none mx-auto lg:mx-0 pb-6 sm:py-2">
							<div
								class="lg:hidden w-[calc(100%+2.5rem)] -mx-5 -mt-2 mb-6 flex items-center justify-center overflow-hidden"
								style="-webkit-mask-image: linear-gradient(to bottom, transparent, black 15%, black 85%, transparent), linear-gradient(to right, transparent, black 28%, black 72%, transparent); -webkit-mask-composite: source-in; mask-image: linear-gradient(to bottom, transparent, black 15%, black 85%, transparent), linear-gradient(to right, transparent, black 28%, black 72%, transparent); mask-composite: intersect;"
							>
								<img
									src="/images/onboarding-happy.webp"
									alt="Workspace Ready Mascot"
									class="w-full max-h-64 object-contain"
								/>
							</div>

							<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Setup Complete</div>
							<h2 class="text-2xl sm:text-3xl font-medium text-slate-900 tracking-tight mb-2">You’re all set! 🎉</h2>
							<p class="text-sm text-slate-500 mb-8 font-normal leading-relaxed max-w-md lg:max-w-none">{productMode === 'chatbot_only' ? 'Your channels and AI assistant are ready to handle customer conversations.' : 'Your workspace is configured and ready to turn conversations into customers.'}</p>

							<div class="space-y-3 w-full text-left">
								<div class="p-3.5 sm:p-4 bg-blue-50/60 border border-blue-100 rounded-xl flex items-center gap-3.5">
									<div class="w-9 h-9 rounded-lg bg-blue-600 text-white flex items-center justify-center shrink-0">
										<Icon name="check" size={18} color="#FFFFFF" strokeWidth={2.5} />
									</div>
									<div>
										<div class="text-sm font-medium text-slate-900">Workspace is fully configured</div>
										<div class="text-xs text-slate-500 font-normal mt-0.5">{productMode === 'chatbot_only' ? 'Your business profile, channels, knowledge, and AI preferences are active.' : 'Your business profile, team, channels, and reply preferences are active.'}</div>
									</div>
								</div>

								<div class="p-3.5 sm:p-4 bg-white border border-slate-200 rounded-xl flex items-center gap-3.5">
									<div class="w-9 h-9 rounded-lg bg-slate-100 text-slate-600 flex items-center justify-center shrink-0">
										<Icon name="chat" size={18} color="currentColor" />
									</div>
									<div>
										<div class="text-sm font-medium text-slate-900">Omni-Channel Inbox</div>
										<div class="text-xs text-slate-500 font-normal mt-0.5">Manage live conversations from WhatsApp, Instagram, Messenger, and Telegram.</div>
									</div>
								</div>
							</div>
						</div>
					{/if}
				</div>

				<OnboardingFooter
					stepNum={stepNum}
					kbStatus={s6Status}
					rawText={s6RawText}
					submitting={submitting}
					compiling={s6Compiling}
					continueDisabled={stepNum === 5 && s5AiMode !== 'manual' && !aiProviderConfigured && !aiProviderApiKey.trim()}
					onBack={handleBack}
					onContinue={handleContinue}
					onTour={() => goto('/inbox?tour=true')}
					onInbox={() => goto('/inbox')}
				/>

				{#if error}
					<div role="alert" class="mt-4 p-3 bg-rose-50 border border-rose-200 rounded-xl text-rose-700 text-xs font-medium w-full">{error}</div>
				{/if}
			</div>
		</div>
	</div>
{/if}
