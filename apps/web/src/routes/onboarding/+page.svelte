<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';

	const STEP_KEYS = [
		'business_basics',
		'channel_connect',
		'pipeline_setup',
		'reply_mode',
		'preferences',
		'review_finish'
	];

	const STEP_KEY_TO_NUM: Record<string, number> = {
		business_basics: 1,
		channel_connect: 2,
		pipeline_setup: 3,
		reply_mode: 4,
		preferences: 5,
		review_finish: 6
	};

	onMount(async () => {
		try {
			await apiRequest('/auth/me');
		} catch {
			goto('/login');
			return;
		}

		try {
			const status = await apiRequest('/onboarding/status');

			if (status?.completed_at) {
				goto('/inbox');
				return;
			}

			const completed: string[] = status?.completed_steps ?? [];
			const skipped: string[] = status?.skipped_steps ?? [];

			// Find first step not completed and not skipped
			for (const key of STEP_KEYS) {
				if (!completed.includes(key) && !skipped.includes(key)) {
					goto(`/onboarding/${STEP_KEY_TO_NUM[key]}`);
					return;
				}
			}

			// All steps completed or skipped => go to step 7 (success)
			goto('/onboarding/7');
		} catch (err) {
			goto('/onboarding/1');
		}
	});
</script>

<div style="padding: 40px; text-align: center; max-width: 480px; margin: 0 auto; color: #64748B; font-size: 14px;">
	Loading your workspace setup...
</div>
