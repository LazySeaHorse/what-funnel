<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
	import AppLoadingScreen from '$lib/components/AppLoadingScreen.svelte';

	const STEP_KEY_TO_NUM: Record<string, number> = {
		business_basics: 1,
		channel_connect: 2,
		pipeline_setup: 3,
		team_setup: 4,
		reply_mode: 5,
		kb_setup: 6,
		review_finish: 7
	};
	const STEP_KEYS = Object.keys(STEP_KEY_TO_NUM);

	onMount(async () => {
		try {
			await apiRequest('/auth/me');
		} catch (err) {
			goto('/login');
			return;
		}

		try {
			const [status, account] = await Promise.all([
				apiRequest('/onboarding/status'),
				apiRequest('/workspace/account')
			]);

			if (status?.completed_at) {
				goto('/inbox');
				return;
			}

			const completed: string[] = status?.completed_steps ?? [];
			const skipped: string[] = status?.skipped_steps ?? [];
			const unavailableSteps = account?.product_mode === 'chatbot_only'
				? new Set(['pipeline_setup', 'team_setup'])
				: new Set<string>();

			for (const key of STEP_KEYS) {
				if (!unavailableSteps.has(key) && !completed.includes(key) && !skipped.includes(key)) {
					goto(`/onboarding/${STEP_KEY_TO_NUM[key]}`);
					return;
				}
			}

			// All done/skipped
			goto('/onboarding/8');
		} catch (_) {
			// Onboarding endpoint not available — go to inbox
			goto('/inbox');
		}
	});
</script>

<AppLoadingScreen message="Opening What Funnel…" detail="Checking your session and workspace." />
