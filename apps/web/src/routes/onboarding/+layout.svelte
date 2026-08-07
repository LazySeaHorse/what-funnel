<script lang="ts">
	import '../../app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { apiRequest } from '$lib/api';
	import Icon from '$lib/Icon.svelte';

	let { children } = $props();

	const STEP_LABELS = [
		'Account',
		'Setup',
		'Business',
		'Connect',
		'Knowledge',
		'Reply Mode',
		'Pipeline',
		'Team',
		'Done'
	];

	let currentStep = $derived(parseInt(($page.params as any)?.step ?? '1', 10) || 1);
	let completedSteps = $state<string[]>([]);
	let skippedSteps = $state<string[]>([]);
	let productMode = $state('full_workspace');

	onMount(async () => {
		try {
			const status = await apiRequest('/onboarding/status');
			if (status) {
				completedSteps = status.completed_steps ?? [];
				skippedSteps = status.skipped_steps ?? [];
			}
			const account = await apiRequest('/workspace/account').catch(() => null);
			if (account) {
				productMode = account.product_mode || 'full_workspace';
			}
		} catch (err) {
			// silently fail — layout is decorative only
		}
	});

	function stepStatus(index: number): 'done' | 'current' | 'future' {
		const stepNum = index + 1;
		if (stepNum < currentStep) return 'done';
		if (stepNum === currentStep) return 'current';
		return 'future';
	}

	function isStepVisible(index: number) {
		if (index === 6 && productMode === 'chatbot_only') return false;
		return true;
	}
</script>

<div class="wizard-bg">
	<div class="wizard-shell">
		<!-- Logo -->
		<div class="wizard-logo">
			<div class="logo-icon-box">
				<Icon name="bot" size={18} color="var(--blue-text)" />
			</div>
			<span class="logo-word">What Funnel</span>
		</div>

		<!-- Progress Stepper -->
		<div class="stepper" role="progressbar" aria-valuenow={currentStep} aria-valuemin={1} aria-valuemax={9}>
			{#each STEP_LABELS as label, i}
				{#if isStepVisible(i)}
					{@const status = stepStatus(i)}
					<div class="step-item">
						<div
							class="step-circle"
							class:done={status === 'done'}
							class:current={status === 'current'}
							class:future={status === 'future'}
						>
							{#if status === 'done'}
								<Icon name="check" size={12} color="#FFFFFF" strokeWidth={3} />
							{:else}
								<span>{i + 1}</span>
							{/if}
						</div>
						<span
							class="step-label"
							class:label-done={status === 'done'}
							class:label-current={status === 'current'}
							class:label-future={status === 'future'}
						>
							{label}
						</span>
					</div>
					{#if i < STEP_LABELS.length - 1 && isStepVisible(i + 1)}
						<div class="step-connector" class:connector-done={stepStatus(i) === 'done'}></div>
					{/if}
				{/if}
			{/each}
		</div>

		<!-- Step Content -->
		<div class="wizard-content">
			{#key currentStep}
				{@render children()}
			{/key}
		</div>

		<!-- Footer -->
		<div class="wizard-footer">
			Step {currentStep} of 9
		</div>
	</div>
</div>

<style>
	.wizard-bg {
		min-height: 100vh;
		background-color: var(--bg-page);
		display: flex;
		align-items: center;
		justify-content: center;
		position: relative;
		overflow: auto;
		padding: 32px 16px;
	}

	.wizard-shell {
		position: relative;
		z-index: 1;
		width: 100%;
		max-width: 720px;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 28px;
	}

	.wizard-logo {
		display: flex;
		align-items: center;
		gap: 10px;
	}

	.logo-icon-box {
		width: 28px;
		height: 28px;
		border-radius: 6px;
		background: var(--blue-bg);
		border: 1px solid var(--blue-border);
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.logo-word {
		font-size: 20px;
		font-weight: 700;
		color: var(--text-primary);
		letter-spacing: -0.3px;
	}

	.stepper {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0;
		width: 100%;
		overflow-x: auto;
		padding: 4px 0;
	}

	.step-item {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 6px;
		flex-shrink: 0;
	}

	.step-circle {
		width: 28px;
		height: 28px;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 12px;
		font-weight: 600;
		position: relative;
		transition: all 0.2s ease;
	}

	.step-circle.done {
		background: var(--blue-primary);
		color: #FFFFFF;
	}

	.step-circle.current {
		background: var(--blue-primary);
		color: #FFFFFF;
		box-shadow: 0 0 0 3px var(--blue-bg);
	}

	.step-circle.future {
		background: #FFFFFF;
		color: var(--text-muted);
		border: 1px solid var(--border-color);
	}

	.step-label {
		font-size: 10.5px;
		font-weight: 500;
		text-align: center;
		white-space: nowrap;
		transition: color 0.2s;
	}

	.label-done { color: var(--blue-text); }
	.label-current { color: var(--text-primary); font-weight: 600; }
	.label-future { color: var(--text-muted); }

	.step-connector {
		width: 20px;
		height: 1px;
		background: var(--border-color);
		flex-shrink: 0;
		margin-bottom: 16px;
		transition: background 0.2s;
	}

	.step-connector.connector-done {
		background: var(--blue-primary);
	}

	.wizard-content {
		width: 100%;
	}

	.wizard-footer {
		font-size: 12px;
		color: var(--text-muted);
		text-align: center;
	}
</style>
