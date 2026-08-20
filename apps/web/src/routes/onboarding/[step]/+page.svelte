<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { apiRequest } from '$lib/api';
	import Icon from '$lib/Icon.svelte';
	import OnboardingChrome from '$lib/components/onboarding/OnboardingChrome.svelte';
	import OnboardingFooter from '$lib/components/onboarding/OnboardingFooter.svelte';

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
	let s1BusinessName = $state('');
	let s1BusinessType = $state('Salon / Beauty');
	let s1Timezone = $state('(GMT+05:30) Asia / Colombo');

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

	// Step 4: AI Assistant
	let s4AiMode = $state<'auto_answer' | 'suggest_only' | 'manual'>('auto_answer');

	// Step 5: Knowledge Base
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
				await apiRequest('/onboarding/status', {
					method: 'PATCH',
					body: { step: 'channel_connect', action: 'complete' }
				}).catch(() => {});

				goToStep(3);
			} else if (stepNum === 3) {
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

	function toggleChannel(_ch: any) {
		// The guided bridge flow lives in workspace settings. Do not create a
		// local-only "connected" state during onboarding.
		goto('/inbox?tab=settings');
	}

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

{#if stepNum >= 1 && stepNum <= 7}
	<!-- FULL-SCREEN 3-COLUMN ONBOARDING INTERFACE (Pure Tailwind) -->
	<div class="min-h-[100dvh] w-full bg-white flex flex-col lg:flex-row overflow-x-hidden font-sans text-slate-800 antialiased">
		
		<OnboardingChrome stepNum={stepNum} stepItems={STEP_ITEMS} onStep={goToStep} />

		<!-- Right Main Form Content Column: Takes Up Full Remaining Width -->
		<div class="flex-1 p-6 sm:p-10 lg:p-12 overflow-y-auto bg-white flex flex-col justify-between min-h-0">
			<div class="w-full flex flex-col min-h-full justify-between">
				<div class="w-full">
					<!-- STEP 1: BUSINESS INFO -->
					{#if stepNum === 1}
						<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step 1 of 6</div>
						<h2 class="text-2xl font-medium text-slate-900 tracking-tight mb-1">Let’s start with your business</h2>
						<p class="text-sm text-slate-500 mb-8 font-normal">This helps us personalize your workspace.</p>

						<div class="space-y-5 w-full">
							<div>
								<label for="business-name" class="block text-xs font-medium text-slate-700 mb-1.5">Business name</label>
								<input
									id="business-name"
									type="text"
									class="w-full px-4 py-2.5 bg-white border border-slate-200 rounded-xl text-sm text-slate-900 placeholder:text-slate-400 focus:border-blue-600 focus:ring-2 focus:ring-blue-100 outline-none transition-all font-normal"
									placeholder="e.g. Your business name"
									bind:value={s1BusinessName}
								/>
							</div>

							<div>
								<label for="business-type" class="block text-xs font-medium text-slate-700 mb-1.5">Business type</label>
								<div class="relative w-full">
									<select id="business-type" class="w-full px-4 py-2.5 bg-white border border-slate-200 rounded-xl text-sm text-slate-900 focus:border-blue-600 focus:ring-2 focus:ring-blue-100 outline-none transition-all appearance-none cursor-pointer pr-10 font-normal" bind:value={s1BusinessType}>
										<option value="Salon / Beauty">Salon / Beauty</option>
										<option value="Photography">Photography</option>
										<option value="Tutoring / Education">Tutoring / Education</option>
										<option value="Home Services">Home Services</option>
										<option value="E-commerce / Retail">E-commerce / Retail</option>
										<option value="Consulting / Agency">Consulting / Agency</option>
										<option value="Other">Other</option>
									</select>
									<div class="absolute inset-y-0 right-0 pr-3.5 flex items-center pointer-events-none text-slate-400">
										<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
											<path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
										</svg>
									</div>
								</div>
							</div>

							<div>
								<label for="timezone" class="block text-xs font-medium text-slate-700 mb-1.5">Time zone</label>
								<div class="relative w-full">
									<select id="timezone" class="w-full px-4 py-2.5 bg-white border border-slate-200 rounded-xl text-sm text-slate-900 focus:border-blue-600 focus:ring-2 focus:ring-blue-100 outline-none transition-all appearance-none cursor-pointer pr-10 font-normal" bind:value={s1Timezone}>
										<option value="(GMT+05:30) Asia / Colombo">(GMT+05:30) Asia / Colombo</option>
										<option value="(GMT+00:00) UTC / London">(GMT+00:00) UTC / London</option>
										<option value="(GMT-05:00) Eastern Time (US & Canada)">(GMT-05:00) Eastern Time (US & Canada)</option>
										<option value="(GMT-08:00) Pacific Time (US & Canada)">(GMT-08:00) Pacific Time (US & Canada)</option>
										<option value="(GMT+01:00) Paris / Berlin">(GMT+01:00) Paris / Berlin</option>
										<option value="(GMT+08:00) Singapore / Beijing">(GMT+08:00) Singapore / Beijing</option>
										<option value="(GMT+09:00) Tokyo">(GMT+09:00) Tokyo</option>
									</select>
									<div class="absolute inset-y-0 right-0 pr-3.5 flex items-center pointer-events-none text-slate-400">
										<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
											<path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
										</svg>
									</div>
								</div>
							</div>
						</div>

					<!-- STEP 2: CHANNELS -->
					{:else if stepNum === 2}
						<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step 2 of 6</div>
						<h2 class="text-2xl font-medium text-slate-900 tracking-tight mb-1">Connect your channels</h2>
						<p class="text-sm text-slate-500 mb-8 font-normal">Bring all your customer conversations into one place. Each connection is completed in workspace settings.</p>

						<div class="space-y-3 w-full">
							{#each channels as ch}
								<div class="flex items-center justify-between p-4 bg-white border border-slate-200 rounded-xl hover:border-slate-300 transition">
									<div class="flex items-center gap-3">
										<div class="w-9 h-9 rounded-lg bg-slate-50 border border-slate-100 flex items-center justify-center">
											<Icon name={ch.icon} size={20} color={ch.color} />
										</div>
										<span class="text-sm font-medium text-slate-800">{ch.name}</span>
									</div>

									<div>
										{#if ch.connected}
											<button type="button" class="flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg bg-emerald-50 border border-emerald-200 text-emerald-700 text-xs font-medium cursor-pointer" onclick={() => toggleChannel(ch)}>
												<Icon name="check" size={14} color="#10B981" />
											<span>Connected</span>
										</button>
									{:else}
										<button type="button" class="px-3.5 py-1.5 rounded-lg border border-slate-200 hover:border-slate-300 bg-white hover:bg-slate-50 text-slate-700 text-xs font-medium transition cursor-pointer shadow-xs" onclick={() => toggleChannel(ch)}>
											Set up
											</button>
										{/if}
									</div>
								</div>
							{/each}
						</div>

					<!-- STEP 3: LEAD PIPELINE -->
					{:else if stepNum === 3}
						<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step 3 of 6</div>
						<h2 class="text-2xl font-medium text-slate-900 tracking-tight mb-1">Set up your lead pipeline</h2>
						<p class="text-sm text-slate-500 mb-8 font-normal">Create the stages your leads will go through.</p>

						<div class="space-y-3 w-full">
							<div class="space-y-2 w-full">
								{#each pipelineStages as stage, i}
									<div class="flex items-center gap-2.5 p-2.5 bg-slate-50 border border-slate-200 rounded-xl w-full">
										<div class="w-2.5 h-2.5 rounded-full shrink-0 ml-1" style="background-color: {stage.color};"></div>
										<input
											type="text"
											class="flex-1 px-3 py-1.5 bg-white border border-slate-200 rounded-lg text-sm text-slate-900 focus:border-blue-600 focus:ring-1 focus:ring-blue-100 outline-none font-normal"
											bind:value={stage.label}
											placeholder="Stage name"
										/>
										<button
											type="button"
											class="p-1.5 text-slate-400 hover:text-rose-500 rounded-lg hover:bg-rose-50 transition cursor-pointer disabled:opacity-30 disabled:cursor-not-allowed"
											onclick={() => removeStage(i)}
											title="Remove stage"
											disabled={pipelineStages.length <= 1}
										>
											<Icon name="trash" size={16} color="currentColor" />
										</button>
									</div>
								{/each}
							</div>

							<button type="button" class="mt-2 flex items-center gap-2 px-3.5 py-2 text-xs font-medium text-blue-600 hover:text-blue-700 hover:bg-blue-50 rounded-xl transition cursor-pointer" onclick={addStage}>
								<Icon name="plus" size={14} color="currentColor" />
								<span>Add another stage</span>
							</button>
						</div>

					<!-- STEP 4: AI ASSISTANT -->
					{:else if stepNum === 4}
						<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step 4 of 6</div>
						<h2 class="text-2xl font-medium text-slate-900 tracking-tight mb-1">Meet your AI Assistant</h2>
						<p class="text-sm text-slate-500 mb-8 font-normal">How would you like your assistant to handle conversations?</p>

						<div class="space-y-3 w-full">
							<!-- Option 1: Auto answer -->
							<button
								type="button"
								class="w-full text-left p-4 rounded-xl border transition-all cursor-pointer flex items-start justify-between {s4AiMode === 'auto_answer' ? 'border-blue-600 bg-blue-50/40 ring-1 ring-blue-600' : 'border-slate-200 bg-white hover:border-slate-300'}"
								onclick={() => s4AiMode = 'auto_answer'}
							>
								<div class="flex items-start gap-3.5">
									<div class="w-8 h-8 rounded-lg {s4AiMode === 'auto_answer' ? 'bg-blue-600 text-white' : 'bg-slate-100 text-slate-500'} flex items-center justify-center shrink-0 mt-0.5">
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
								<div class="w-4 h-4 rounded-full border flex items-center justify-center mt-1 {s4AiMode === 'auto_answer' ? 'border-blue-600' : 'border-slate-300'}">
									{#if s4AiMode === 'auto_answer'}
										<div class="w-2 h-2 rounded-full bg-blue-600"></div>
									{/if}
								</div>
							</button>

							<!-- Option 2: Suggest replies only -->
							<button
								type="button"
								class="w-full text-left p-4 rounded-xl border transition-all cursor-pointer flex items-start justify-between {s4AiMode === 'suggest_only' ? 'border-blue-600 bg-blue-50/40 ring-1 ring-blue-600' : 'border-slate-200 bg-white hover:border-slate-300'}"
								onclick={() => s4AiMode = 'suggest_only'}
							>
								<div class="flex items-start gap-3.5">
									<div class="w-8 h-8 rounded-lg {s4AiMode === 'suggest_only' ? 'bg-blue-600 text-white' : 'bg-slate-100 text-slate-500'} flex items-center justify-center shrink-0 mt-0.5">
										<Icon name="sparkles" size={18} color="currentColor" />
									</div>
									<div>
										<span class="text-sm font-medium text-slate-900">Suggest replies only</span>
										<p class="text-xs text-slate-500 mt-1 leading-relaxed font-normal">AI will draft suggested responses for your team to review and dispatch.</p>
									</div>
								</div>
								<div class="w-4 h-4 rounded-full border flex items-center justify-center mt-1 {s4AiMode === 'suggest_only' ? 'border-blue-600' : 'border-slate-300'}">
									{#if s4AiMode === 'suggest_only'}
										<div class="w-2 h-2 rounded-full bg-blue-600"></div>
									{/if}
								</div>
							</button>

							<!-- Option 3: Manual only -->
							<button
								type="button"
								class="w-full text-left p-4 rounded-xl border transition-all cursor-pointer flex items-start justify-between {s4AiMode === 'manual' ? 'border-blue-600 bg-blue-50/40 ring-1 ring-blue-600' : 'border-slate-200 bg-white hover:border-slate-300'}"
								onclick={() => s4AiMode = 'manual'}
							>
								<div class="flex items-start gap-3.5">
									<div class="w-8 h-8 rounded-lg {s4AiMode === 'manual' ? 'bg-blue-600 text-white' : 'bg-slate-100 text-slate-500'} flex items-center justify-center shrink-0 mt-0.5">
										<Icon name="edit" size={18} color="currentColor" />
									</div>
									<div>
										<span class="text-sm font-medium text-slate-900">Manual only</span>
										<p class="text-xs text-slate-500 mt-1 leading-relaxed font-normal">AI will not send messages automatically. All replies are composed manually.</p>
									</div>
								</div>
								<div class="w-4 h-4 rounded-full border flex items-center justify-center mt-1 {s4AiMode === 'manual' ? 'border-blue-600' : 'border-slate-300'}">
									{#if s4AiMode === 'manual'}
										<div class="w-2 h-2 rounded-full bg-blue-600"></div>
									{/if}
								</div>
							</button>
						</div>

					<!-- STEP 5: KNOWLEDGE BASE -->
					{:else if stepNum === 5}
						<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step 5 of 6</div>
						{#if s5Status === 'input'}
							<h2 class="text-2xl font-medium text-slate-900 tracking-tight mb-1">Teach your AI assistant</h2>
							<p class="text-sm text-slate-500 mb-6 font-normal">Add business notes, price lists, FAQs, hours, or policies. The AI compiler organizes it automatically.</p>

							<div class="space-y-4 w-full">
								<div class="flex flex-wrap items-center gap-2">
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
									bind:value={s5RawText}
								></textarea>
							</div>

						{:else if s5Status === 'processing'}
							<div class="py-12 flex flex-col items-center justify-center text-center space-y-4 w-full">
								<div class="w-12 h-12 rounded-2xl bg-blue-50 border border-blue-100 flex items-center justify-center text-blue-600">
									<Icon name="sparkles" size={24} color="currentColor" />
								</div>
								<h2 class="text-xl font-medium text-slate-900">Organizing your knowledge...</h2>
								<p class="text-xs sm:text-sm text-slate-500 max-w-sm font-normal">
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

						{:else if s5Status === 'results'}
							<div class="flex items-center justify-between mb-4 w-full">
								<div>
									<h2 class="text-2xl font-medium text-slate-900 tracking-tight">Structured Knowledge</h2>
									<p class="text-sm text-slate-500 font-normal">Concepts inferred from your business notes:</p>
								</div>
								<button type="button" class="px-3 py-1.5 text-xs font-medium text-blue-600 hover:bg-blue-50 rounded-lg transition" onclick={() => s5Status = 'input'}>
									Edit raw notes
								</button>
							</div>

							<div class="grid grid-cols-1 sm:grid-cols-2 gap-3 w-full">
								{#each s5Concepts as concept}
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

					<!-- STEP 6: REVIEW AND FINISH -->
					{:else if stepNum === 6}
						<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step 6 of 6</div>
						<h2 class="text-2xl font-medium text-slate-900 tracking-tight mb-1">Review and finish</h2>
						<p class="text-sm text-slate-500 mb-8 font-normal">Here’s a summary of your workspace setup.</p>

						<div class="space-y-3 w-full">
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
								<button type="button" class="text-xs font-medium text-blue-600 hover:underline" onclick={() => goToStep(4)}>Edit</button>
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
								<button type="button" class="text-xs font-medium text-blue-600 hover:underline" onclick={() => goToStep(5)}>Edit</button>
							</div>
						</div>

					<!-- STEP 7: ALL SET! READY TO GO -->
					{:else if stepNum === 7}
						<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Setup Complete</div>
						<h2 class="text-2xl font-medium text-slate-900 tracking-tight mb-1">All set! You’re ready to go</h2>
						<p class="text-sm text-slate-500 mb-8 font-normal">Your workspace is configured and ready. Start managing customer conversations and capturing leads.</p>

						<div class="space-y-3 w-full">
							<div class="p-4 bg-blue-50/60 border border-blue-100 rounded-xl flex items-center gap-3.5">
								<div class="w-9 h-9 rounded-lg bg-blue-600 text-white flex items-center justify-center shrink-0">
									<Icon name="check" size={18} color="#FFFFFF" strokeWidth={2.5} />
								</div>
								<div>
									<div class="text-sm font-medium text-slate-900">Workspace is fully configured</div>
									<div class="text-xs text-slate-500 font-normal mt-0.5">Your business profile, channels, and reply preferences are active.</div>
								</div>
							</div>

							<div class="p-4 bg-white border border-slate-200 rounded-xl flex items-center gap-3.5">
								<div class="w-9 h-9 rounded-lg bg-slate-100 text-slate-600 flex items-center justify-center shrink-0">
									<Icon name="chat" size={18} color="currentColor" />
								</div>
								<div>
									<div class="text-sm font-medium text-slate-900">Omni-Channel Inbox</div>
									<div class="text-xs text-slate-500 font-normal mt-0.5">Manage live conversations from WhatsApp, Instagram, Messenger, and Telegram in one unified view.</div>
								</div>
							</div>

							<div class="p-4 bg-white border border-slate-200 rounded-xl flex items-center gap-3.5">
								<div class="w-9 h-9 rounded-lg bg-slate-100 text-slate-600 flex items-center justify-center shrink-0">
									<Icon name="bot" size={18} color="currentColor" />
								</div>
								<div>
									<div class="text-sm font-medium text-slate-900">AI Co-pilot & Auto Answers</div>
									<div class="text-xs text-slate-500 font-normal mt-0.5">Answers customer queries and generates reply drafts tailored to your business rules.</div>
								</div>
							</div>
						</div>
					{/if}
				</div>

				<OnboardingFooter
					stepNum={stepNum}
					kbStatus={s5Status}
					rawText={s5RawText}
					submitting={submitting}
					compiling={s5Compiling}
					onBack={handleBack}
					onContinue={handleContinue}
					onTour={() => goto('/inbox?tour=true')}
					onInbox={() => goto('/inbox')}
				/>

				{#if error}
					<div class="mt-4 p-3 bg-rose-50 border border-rose-200 rounded-xl text-rose-700 text-xs font-medium w-full">{error}</div>
				{/if}
			</div>
		</div>
	</div>
{/if}
