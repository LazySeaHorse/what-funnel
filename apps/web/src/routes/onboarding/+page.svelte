<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
	import AppLoadingScreen from '$lib/components/AppLoadingScreen.svelte';

	const STEP_KEYS = [
		'business_basics',
		'channel_connect',
		'pipeline_setup',
		'team_setup',
		'reply_mode',
		'kb_setup',
		'review_finish'
	];

	const STEP_KEY_TO_NUM: Record<string, number> = {
		business_basics: 1,
		channel_connect: 2,
		pipeline_setup: 3,
		team_setup: 4,
		reply_mode: 5,
		kb_setup: 6,
		review_finish: 7
	};

	onMount(async () => {
		try {
			await apiRequest('/auth/me');
		} catch {
			await goto('/login');
			return;
		}

		try {
			const [status, account] = await Promise.all([
				apiRequest('/onboarding/status'),
				apiRequest('/workspace/account').catch(() => null)
			]);

			if (status?.completed_at) {
				await goto('/inbox');
				return;
			}

			const completed: string[] = status?.completed_steps ?? [];
			const skipped: string[] = status?.skipped_steps ?? [];
			const unavailableSteps = account?.product_mode === 'chatbot_only'
				? new Set(['pipeline_setup', 'team_setup'])
				: new Set<string>();

			// Find first step not completed and not skipped
			for (const key of STEP_KEYS) {
				if (!unavailableSteps.has(key) && !completed.includes(key) && !skipped.includes(key)) {
					await goto(`/onboarding/${STEP_KEY_TO_NUM[key]}`);
					return;
				}
			}

			// All steps completed or skipped => go to step 8 (success)
			await goto('/onboarding/8');
		} catch (err) {
			await goto('/onboarding/1');
		}
	});
</script>

<AppLoadingScreen message="Loading your workspace setup…" detail="Preparing your onboarding experience." />
