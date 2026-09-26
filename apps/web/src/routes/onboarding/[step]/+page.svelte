<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
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
	import { fade } from 'svelte/transition';
	import { OnboardingWizardController } from '$lib/onboarding';

	let stepNum = $derived(parseInt(($page.params as any)?.step ?? '1', 10) || 1);

	const wizard = new OnboardingWizardController({
		getStepNum: () => stepNum,
		navigate: goto
	});

	onMount(() => {
		void wizard.init();
	});

	onDestroy(() => {
		wizard.dispose();
	});
</script>

<svelte:head>
	<title>Onboarding — What Funnel</title>
</svelte:head>

{#if wizard.stepNum >= 1 && wizard.stepNum <= 8}
	<!-- FULL-SCREEN ONBOARDING INTERFACE (Pure Tailwind) -->
	<div class="h-[100dvh] w-full bg-white flex flex-col lg:flex-row overflow-hidden font-sans text-slate-800 antialiased relative">
		
		<OnboardingChrome stepNum={wizard.stepNum} stepItems={wizard.visibleStepItems} onStep={(num) => wizard.goToStep(num)} />

		<!-- Right Main Form Content Column: Takes Up Full Remaining Width -->
		<div class="flex-1 flex flex-col min-h-0 h-full bg-white">
			<!-- Scrollable Form Content -->
			<div class="flex-1 relative overflow-y-auto min-h-0 p-5 sm:p-10 lg:p-12">
				<div class="w-full">
					<!-- Mobile Top Bar: Back Button & Step Progress Stepper -->
					<div class="lg:hidden flex items-center justify-between pb-4 mb-5 border-b border-slate-100">
						<button
							type="button"
							class="p-2 -ml-2 text-slate-500 hover:text-slate-900 rounded-lg active:bg-slate-100 transition cursor-pointer"
							onclick={() => wizard.handleBack()}
							aria-label="Go back"
						>
							<ChevronLeftIcon class="w-5 h-5" />
						</button>

						{#if wizard.stepNum <= 7}
							<div class="flex items-center gap-1.5" aria-label={`Step ${wizard.displayStepNum} of ${wizard.visibleStepItems.length}`}>
								{#each wizard.visibleStepItems as item, idx}
									<div class="h-1.5 rounded-full transition-all duration-200 {item.num === wizard.stepNum ? 'w-5 bg-blue-600' : idx < wizard.displayStepNum - 1 ? 'w-2.5 bg-blue-600' : 'w-2 bg-slate-200'}"></div>
								{/each}
							</div>
						{:else}
							<div class="text-xs font-medium text-slate-500">Setup complete</div>
						{/if}

						<div class="w-9"></div>
					</div>

					<!-- Steps own their presentation and form-local behavior; this page coordinates persistence and navigation. -->
					{#key wizard.stepNum}
						<div in:fade={{ duration: 140 }}>
							{#if wizard.stepNum === 1}
								<BusinessInfoStep
									step={wizard.displayStepNum}
									totalSteps={wizard.visibleStepItems.length}
									bind:businessName={wizard.businessName}
									bind:businessType={wizard.businessType}
									bind:timezone={wizard.timezone}
								/>
							{:else if wizard.stepNum === 2}
								<ChannelsStep
									step={wizard.displayStepNum}
									totalSteps={wizard.visibleStepItems.length}
									channels={wizard.channels}
									onConnect={(ch) => wizard.toggleChannel(ch)}
								/>
							{:else if wizard.stepNum === 3}
								<PipelineStep
									step={wizard.displayStepNum}
									totalSteps={wizard.visibleStepItems.length}
									bind:stages={wizard.pipelineStages}
								/>
							<!-- STEP 4: TEAM MEMBERS & WORKSPACE SLUG -->
							{:else if wizard.stepNum === 4}
								<TeamStep
									step={wizard.displayStepNum}
									totalSteps={wizard.visibleStepItems.length}
									bind:slug={wizard.slug}
									bind:users={wizard.users}
									onAddUser={(username, password, role) => wizard.addTeamMember(username, password, role)}
									onRemoveUser={(id) => wizard.removeTeamMember(id)}
								/>
							<!-- STEP 5: AI ASSISTANT -->
							{:else if wizard.stepNum === 5}
								<AIStep
									step={wizard.displayStepNum}
									totalSteps={wizard.visibleStepItems.length}
									bind:aiMode={wizard.aiMode}
									providerConfigured={wizard.aiProviderConfigured}
									bind:providerApiKey={wizard.aiProviderApiKey}
									bind:providerBaseURL={wizard.aiProviderBaseURL}
									bind:analysisModel={wizard.aiAnalysisModel}
									bind:replyModel={wizard.aiReplyModel}
									bind:embeddingModel={wizard.aiEmbeddingModel}
									bind:verifiedConfigFingerprint={wizard.aiVerifiedConfigFingerprint}
								/>

							<!-- STEP 6: KNOWLEDGE BASE -->
							{:else if wizard.stepNum === 6}
								<KnowledgeBaseStep
									step={wizard.displayStepNum}
									totalSteps={wizard.visibleStepItems.length}
									bind:rawText={wizard.rawText}
									status={wizard.kbStatus}
									bind:concepts={wizard.knowledgeIngestion.concepts}
									bind:patterns={wizard.knowledgeIngestion.patterns}
									compiling={wizard.knowledgeIngestion.busy}
									errorMessage={wizard.kbError}
									onSkipWaiting={() => wizard.skipWaitingToNextStep()}
									onEditNotes={() => wizard.editKnowledgeNotes()}
								/>

							<!-- STEP 7: REVIEW AND FINISH -->
							{:else if wizard.stepNum === 7}
								<ReviewStep
									step={wizard.displayStepNum}
									totalSteps={wizard.visibleStepItems.length}
									productMode={wizard.productMode}
									businessName={wizard.businessName}
									channelsText={wizard.connectedChannelsText}
									pipelineStageCount={wizard.pipelineStages.length}
									teamMemberCount={wizard.users.length}
									slug={wizard.slug}
									aiMode={wizard.aiModeLabel}
									knowledgeSummary={wizard.kbTopicsSummary}
									onEdit={(num) => wizard.goToStep(num)}
								/>

							<!-- STEP 8: ALL SET! READY TO GO -->
							{:else if wizard.stepNum === 8}
								<CompleteStep productMode={wizard.productMode} />
							{/if}
						</div>
					{/key}
				</div>
			</div>

			{#if wizard.error}
				<div class="px-5 sm:px-10 lg:px-12 pt-3 shrink-0 bg-white">
					<div role="alert" class="p-3 bg-rose-50 border border-rose-200 rounded-xl text-rose-700 text-xs font-medium w-full">{wizard.error}</div>
				</div>
			{/if}

			<OnboardingFooter
				stepNum={wizard.stepNum}
				kbStatus={wizard.kbStatus}
				rawText={wizard.rawText}
				submitting={wizard.submitting}
				compiling={wizard.knowledgeIngestion.busy}
				continueDisabled={wizard.continueDisabled}
				onBack={() => wizard.handleBack()}
				onContinue={() => wizard.handleContinue()}
				onTour={() => wizard.goToTour()}
				onInbox={() => wizard.goToInbox()}
			/>
		</div>
	</div>
{/if}
