<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { apiRequest } from '$lib/api';
	import { decodeWorkspaceSettings } from '$lib/workspace-settings';
	import OnboardingChrome from '$lib/components/onboarding/OnboardingChrome.svelte';
	import OnboardingFooter from '$lib/components/onboarding/OnboardingFooter.svelte';
	import BusinessInfoStep from '$lib/components/onboarding/BusinessInfoStep.svelte';
	import ChannelsStep from '$lib/components/onboarding/ChannelsStep.svelte';
	import PipelineStep from '$lib/components/onboarding/PipelineStep.svelte';
	import TeamStep from '$lib/components/onboarding/TeamStep.svelte';
	import AIStep from '$lib/components/onboarding/AIStep.svelte';
	import KnowledgeBaseStep from '$lib/components/onboarding/KnowledgeBaseStep.svelte';
	import ReviewStep from '$lib/components/onboarding/ReviewStep.svelte';
	import CompleteStep from '$lib/components/onboarding/CompleteStep.svelte';

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
	let s4Users = $state<Array<{ id: string; username: string; role: string; plaintextPassword?: string }>>([]);


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



	async function addTeamMember(username: string, password: string, role: 'agent' | 'manager') {
		const res = await apiRequest('/workspace/users', {
			method: 'POST',
			body: { username, password, role }
		});
		s4Users = [
			...s4Users,
			{
				id: res.id,
				username: res.username || username,
				role: res.role || role,
				plaintextPassword: res.password || password
			}
		];
	}

	async function removeTeamMember(id: string) {
		await apiRequest(`/workspace/users/${id}`, { method: 'DELETE' });
		s4Users = s4Users.filter(u => u.id !== id);
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
						<TeamStep step={displayStepNum} totalSteps={visibleStepItems.length} bind:slug={s4Slug} bind:users={s4Users} onAddUser={addTeamMember} onRemoveUser={removeTeamMember} />
					<!-- STEP 5: AI ASSISTANT -->
					{:else if stepNum === 5}
						<AIStep step={displayStepNum} totalSteps={visibleStepItems.length} bind:aiMode={s5AiMode} providerConfigured={aiProviderConfigured} bind:providerApiKey={aiProviderApiKey} bind:providerBaseURL={aiProviderBaseURL} bind:completionModel={aiCompletionModel} bind:embeddingModel={aiEmbeddingModel} />

					<!-- STEP 6: KNOWLEDGE BASE -->
					{:else if stepNum === 6}
						<KnowledgeBaseStep step={displayStepNum} totalSteps={visibleStepItems.length} bind:rawText={s6RawText} status={s6Status} concepts={s6Concepts} compiling={s6Compiling} onSkipToDashboard={skipWaitingToDashboard} />

					<!-- STEP 7: REVIEW AND FINISH -->
					{:else if stepNum === 7}
						<ReviewStep
							step={displayStepNum}
							totalSteps={visibleStepItems.length}
							{productMode}
							businessName={s1BusinessName}
							channelsText={connectedChannelsText()}
							pipelineStageCount={pipelineStages.length}
							teamMemberCount={s4Users.length}
							slug={s4Slug}
							aiMode={aiModeLabel()}
							knowledgeSummary={kbTopicsSummary()}
							onEdit={goToStep}
						/>

					<!-- STEP 8: ALL SET! READY TO GO -->
					{:else if stepNum === 8}
						<CompleteStep {productMode} />
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
