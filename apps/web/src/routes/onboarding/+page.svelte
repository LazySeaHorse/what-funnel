<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';

	const STEP_KEYS = [
		'signup',
		'mode_selected',
		'business_basics',
		'channel_connect',
		'kb_setup',
		'reply_mode',
		'pipeline_setup',
		'team_invite'
	];

	const STEP_KEY_TO_NUM: Record<string, number> = {
		signup: 1,
		mode_selected: 2,
		business_basics: 3,
		channel_connect: 4,
		kb_setup: 5,
		reply_mode: 6,
		pipeline_setup: 7,
		team_invite: 8
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

			// All steps done or skipped => done
			goto('/onboarding/9');
		} catch (err) {
			// If onboarding status endpoint doesn't exist yet, just start at step 1
			goto('/onboarding/1');
		}
	});
</script>

<div class="glass-panel" style="padding: 40px; text-align: center; max-width: 680px; margin: 0 auto;">
	<div style="font-size: 14px; color: var(--text-secondary);">Checking your progress...</div>
</div>
