<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';

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
	const STEP_KEYS = Object.keys(STEP_KEY_TO_NUM);

	onMount(async () => {
		try {
			await apiRequest('/auth/me');
		} catch (err) {
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

			for (const key of STEP_KEYS) {
				if (!completed.includes(key) && !skipped.includes(key)) {
					goto(`/onboarding/${STEP_KEY_TO_NUM[key]}`);
					return;
				}
			}

			// All done/skipped
			goto('/onboarding/9');
		} catch (_) {
			// Onboarding endpoint not available — go to inbox
			goto('/inbox');
		}
	});
</script>

<div class="auth-container">
	<div class="glass-panel" style="padding: 32px; text-align: center; border-radius: 12px; min-width: 250px;">
		<div style="font-size: 16px; font-weight: 500; color: var(--text-secondary);">Verifying session...</div>
	</div>
</div>
