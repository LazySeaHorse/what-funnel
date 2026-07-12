<script lang="ts">
	import '../../app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { apiRequest } from '$lib/api';

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

	let currentStep = $derived(parseInt((page.params as any)?.step ?? '1', 10) || 1);
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
	<div class="radial-1"></div>
	<div class="radial-2"></div>

	<div class="wizard-shell">
		<!-- Logo -->
		<div class="wizard-logo">
			<span class="logo-dot"></span>
			<span class="logo-word">What Funnel</span>
		</div>

		<!-- Progress Stepper -->
		<div class="stepper" role="progressbar" aria-valuenow={currentStep} aria-valuemin={1} aria-valuemax={9}>
			{#each STEP_LABELS as label, i}
				{#if isStepVisible(i)}
					{@const status = stepStatus(i)}
					<div class="step-item">
						<div class="step-circle" class:done={status === 'done'} class:current={status === 'current'} class:future={status === 'future'}>
							{#if status === 'done'}
								<svg width="12" height="12" viewBox="0 0 12 12" fill="none" aria-hidden="true">
									<path d="M2 6l3 3 5-5" stroke="#fff" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>
								</svg>
							{:else}
								<span>{i + 1}</span>
							{/if}
							{#if status === 'current'}
								<div class="pulse-ring"></div>
							{/if}
						</div>
						<span class="step-label" class:label-done={status === 'done'} class:label-current={status === 'current'} class:label-future={status === 'future'}>
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
			{@render children()}
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
		background-color: var(--bg-dark);
		display: flex;
		align-items: center;
		justify-content: center;
		position: relative;
		overflow: auto;
		padding: 24px 16px;
	}

	.radial-1 {
		position: fixed;
		top: -10%;
		right: -5%;
		width: 600px;
		height: 600px;
		background: radial-gradient(circle, rgba(99, 102, 241, 0.12) 0%, transparent 70%);
		pointer-events: none;
		z-index: 0;
	}

	.radial-2 {
		position: fixed;
		bottom: -15%;
		left: -10%;
		width: 700px;
		height: 700px;
		background: radial-gradient(circle, rgba(168, 85, 247, 0.08) 0%, transparent 70%);
		pointer-events: none;
		z-index: 0;
	}

	.wizard-shell {
		position: relative;
		z-index: 1;
		width: 100%;
		max-width: 720px;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 32px;
	}

	.wizard-logo {
		display: flex;
		align-items: center;
		gap: 10px;
	}

	.logo-dot {
		width: 12px;
		height: 12px;
		border-radius: 50%;
		background: var(--accent-gradient);
		flex-shrink: 0;
	}

	.logo-word {
		font-size: 22px;
		font-weight: 700;
		background: var(--accent-gradient);
		-webkit-background-clip: text;
		-webkit-text-fill-color: transparent;
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
		gap: 8px;
		flex-shrink: 0;
	}

	.step-circle {
		width: 32px;
		height: 32px;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 12px;
		font-weight: 600;
		position: relative;
		transition: all 0.3s ease;
	}

	.step-circle.done {
		background: var(--accent-gradient);
		color: #fff;
	}

	.step-circle.current {
		background: var(--accent-gradient);
		color: #fff;
		box-shadow: 0 0 20px rgba(99, 102, 241, 0.5);
	}

	.step-circle.future {
		background: rgba(255, 255, 255, 0.04);
		color: var(--text-muted);
		border: 1px solid var(--border-color);
	}

	.pulse-ring {
		position: absolute;
		inset: -4px;
		border-radius: 50%;
		border: 2px solid rgba(99, 102, 241, 0.5);
		animation: pulse-ring 2s ease-out infinite;
	}

	@keyframes pulse-ring {
		0% { opacity: 1; transform: scale(1); }
		100% { opacity: 0; transform: scale(1.5); }
	}

	.step-label {
		font-size: 10px;
		font-weight: 500;
		text-align: center;
		white-space: nowrap;
		transition: color 0.3s;
	}

	.label-done { color: #818cf8; }
	.label-current { color: var(--text-primary); }
	.label-future { color: var(--text-muted); }

	.step-connector {
		width: 24px;
		height: 1px;
		background: var(--border-color);
		flex-shrink: 0;
		margin-bottom: 18px;
		transition: background 0.3s;
	}

	.step-connector.connector-done {
		background: rgba(99, 102, 241, 0.6);
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
