<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
	import Icon from '$lib/Icon.svelte';

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

<div class="min-h-[100dvh] w-full bg-[#F8F9FD] flex items-center justify-center p-4 font-sans text-slate-800 antialiased">
	<div class="bg-white rounded-2xl border border-slate-200 shadow-xs p-8 flex flex-col items-center gap-4 text-center">
		<div class="w-11 h-11 rounded-xl bg-blue-50 border border-blue-100 flex items-center justify-center text-blue-600">
			<Icon name="bot" size={22} color="currentColor" />
		</div>
		<div class="text-sm font-medium text-slate-500">Verifying session...</div>
	</div>
</div>
