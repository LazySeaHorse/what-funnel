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
	import { ChevronLeftIcon } from '@fvilers/heroicons-svelte/24/outline';

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
		{ num: 3, label: 'Lead pipeline' },
		{ num: 4, label: 'Team members' },
		{ num: 5, label: 'AI assistant' },
		{ num: 6, label: 'Knowledge base' },
		{ num: 7, label: 'Review and finish' }
	];
	let visibleStepItems = $derived.by(() => {
		let items = STEP_ITEMS;
		if (productMode === 'chatbot_only') {
			items = items.filter((item) => item.num !== 3 && item.num !== 4);
		}
		if (s5AiMode === 'manual') {
			items = items.filter((item) => item.num !== 6);
		}
		return items;
	});
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
	let aiProviderBaseURL = $state('https://generativelanguage.googleapis.com/v1beta/openai/');
	let aiCompletionModel = $state('gemma-4-26b-a4b-it');
	let aiEmbeddingModel = $state('gemini-embedding-001');

	// Step 6: Knowledge Base
	let s6RawText = $state('');
	let s6Status = $state<'input' | 'processing' | 'results' | 'publishing'>('input');
	let s6Concepts = $state<Array<{ id: string; title: string; type: string; tags: string[]; body_markdown: string; approved: boolean }>>([]);
	let s6Patterns = $state<Array<{ id: string; canonical_question: string; answer_markdown: string; trigger_phrases: string[]; approved: boolean }>>([]);
	let s6Compiling = $state(false);
	let s6Error = $state('');
	let s6IngestionID = $state('');
	let s6PollGeneration = 0;

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
			if (stepNum === 6 && settings.ai_enabled !== false) {
				void resumeLatestIngestion();
			}
		} catch (err: any) {
			error = err?.message || 'We could not load your saved setup. Refresh and try again.';
		} finally {
			loading = false;
		}
	});

	function ingestionConcepts(items: any[]) {
		return (items ?? []).map((item: any) => ({
			id: item.id,
			title: item.title ?? '',
			type: item.type ?? 'faq',
			tags: Array.isArray(item.tags) ? item.tags : [],
			body_markdown: item.body_markdown ?? '',
			approved: item.status !== 'rejected'
		}));
	}

	function ingestionPatterns(items: any[]) {
		return (items ?? []).map((item: any) => ({
			id: item.id,
			canonical_question: item.canonical_question ?? '',
			answer_markdown: item.answer_markdown ?? '',
			trigger_phrases: Array.isArray(item.trigger_phrases) ? item.trigger_phrases : [],
			approved: item.status !== 'rejected'
		}));
	}

	async function pollIngestion(id: string, generation: number) {
		while (generation === s6PollGeneration) {
			const ingestion = await apiRequest(`/api/kb/ingestions/${id}`);
			if (ingestion.status === 'review_required') {
				s6Concepts = ingestionConcepts(ingestion.concepts);
				s6Patterns = ingestionPatterns(ingestion.patterns);
				s6Status = 'results';
				s6Compiling = false;
				return;
			}
			if (ingestion.status === 'complete') {
				await apiRequest('/onboarding/status', {
					method: 'PATCH',
					body: { step: 'kb_setup', action: 'complete' }
				});
				s6Compiling = false;
				goToStep(7);
				return;
			}
			if (ingestion.status === 'failed') {
				throw new Error(ingestion.error || 'Knowledge ingestion failed. Please try again.');
			}
			s6Status = ingestion.status === 'publishing' ? 'publishing' : 'processing';
			await new Promise((resolve) => setTimeout(resolve, 750));
		}
	}

	async function resumeLatestIngestion() {
		try {
			const response = await apiRequest('/api/kb/ingestions/latest');
			const ingestion = response?.ingestion;
			if (!ingestion || ingestion.status === 'failed') return;
			s6IngestionID = ingestion.id;
			s6Compiling = true;
			const generation = ++s6PollGeneration;
			await pollIngestion(ingestion.id, generation);
		} catch (err: any) {
			s6Error = err?.message || 'Could not resume knowledge ingestion.';
			s6Status = 'input';
			s6Compiling = false;
		}
	}

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
			editKnowledgeNotes();
			return;
		}
		if (stepNum === 7 && s5AiMode === 'manual') {
			goToStep(5);
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
			const ingestion = await apiRequest('/api/kb/ingestions', {
				method: 'POST',
				body: { raw_text: s6RawText.trim() }
			});
			s6IngestionID = ingestion.id;
			const generation = ++s6PollGeneration;
			await pollIngestion(ingestion.id, generation);
		} catch (err: any) {
			s6Error = err?.message || 'Failed to process knowledge text. Check your AI provider settings and try again.';
			s6Concepts = [];
			s6Patterns = [];
			s6Status = 'input';
			s6Compiling = false;
		}
	}

	async function publishCompiledKB() {
		if (!s6IngestionID) throw new Error('The knowledge ingestion is unavailable. Organize your notes again.');
		if (!s6Concepts.some((concept) => concept.approved) && !s6Patterns.some((pattern) => pattern.approved)) {
			throw new Error('Select at least one concept or pattern to add.');
		}
		s6Compiling = true;
		s6Status = 'publishing';
		s6Error = '';
		try {
			await apiRequest(`/api/kb/ingestions/${s6IngestionID}/publish`, {
				method: 'POST',
				body: { concepts: s6Concepts, patterns: s6Patterns }
			});
			const generation = ++s6PollGeneration;
			await pollIngestion(s6IngestionID, generation);
		} catch (err: any) {
			s6Error = err?.message || 'Failed to add the reviewed concepts to your knowledge base.';
			s6Status = 'results';
			s6Compiling = false;
		}
	}

	function editKnowledgeNotes() {
		s6PollGeneration++;
		s6IngestionID = '';
		s6Concepts = [];
		s6Patterns = [];
		s6Error = '';
		s6Status = 'input';
	}

	function skipWaitingToNextStep() {
		s6PollGeneration++;
		s6Compiling = false;
		goToStep(7);
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
				const usedKeys = new Set<string>();
				const sanitizedStages = pipelineStages.map((s, idx) => {
					const slug = (s.label || '').trim().toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '');
					const baseKey = slug || s.key || `stage_${idx + 1}`;
					let key = baseKey;
					let counter = 1;
					while (usedKeys.has(key)) {
						counter++;
						key = `${baseKey}_${counter}`;
					}
					usedKeys.add(key);
					return {
						key,
						label: (s.label || '').trim() || 'Stage',
						color: s.color || '#3B82F6'
					};
				});

				await apiRequest(`/workspace/pipelines/${pipelineID}`, {
					method: 'PUT',
					body: {
						name: 'Default Pipeline',
						states: sanitizedStages
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
					await apiRequest('/workspace/account/ai-config/test', {
						method: 'POST',
						body: {
							config: JSON.stringify({
								api_key: aiProviderApiKey.trim(),
								base_url: aiProviderBaseURL.trim().replace(/\/$/, ''),
								completion_model: aiCompletionModel.trim(),
								embedding_model: aiEmbeddingModel.trim()
							})
						}
					});

					await apiRequest('/workspace/account/ai-config', {
						method: 'PUT',
						body: {
							config: JSON.stringify({
								api_key: aiProviderApiKey.trim(),
								base_url: aiProviderBaseURL.trim().replace(/\/$/, ''),
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

				if (s5AiMode === 'manual') {
					await apiRequest('/onboarding/status', {
						method: 'PATCH',
						body: { step: 'kb_setup', action: 'skip' }
					});
					goToStep(7);
				} else {
					goToStep(6);
				}
			} else if (stepNum === 6) {
				if (s6Status === 'input') {
					await startCompilingKB();
				} else if (s6Status === 'results') {
					await publishCompiledKB();
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
		if (s5AiMode === 'manual') {
			return 'Skipped (Manual replies)';
		}
		if (s6Concepts.length > 0) {
			return `${s6Concepts.length} concepts in knowledge base`;
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
							<ChevronLeftIcon class="w-5 h-5" />
						</button>

						{#if stepNum <= 7}
							<div class="flex items-center gap-1.5" aria-label={`Step ${displayStepNum} of ${visibleStepItems.length}`}>
								{#each visibleStepItems as item, idx}
									<div class="h-1.5 rounded-full transition-all duration-200 {item.num === stepNum ? 'w-5 bg-blue-600' : idx < displayStepNum - 1 ? 'w-2.5 bg-blue-600' : 'w-2 bg-slate-200'}"></div>
								{/each}
							</div>
						{:else}
							<div class="text-xs font-medium text-slate-500">Setup complete</div>
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
						<KnowledgeBaseStep step={displayStepNum} totalSteps={visibleStepItems.length} bind:rawText={s6RawText} status={s6Status} bind:concepts={s6Concepts} bind:patterns={s6Patterns} compiling={s6Compiling} errorMessage={s6Error} onSkipWaiting={skipWaitingToNextStep} onEditNotes={editKnowledgeNotes} />

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
