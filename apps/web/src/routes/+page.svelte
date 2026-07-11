<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';

	onMount(async () => {
		try {
			await apiRequest('/auth/me');
			goto('/inbox');
		} catch (err) {
			goto('/login');
		}
	});
</script>

<div class="auth-container">
	<div class="glass-panel" style="padding: 32px; text-align: center; border-radius: 12px; min-width: 250px;">
		<div style="font-size: 16px; font-weight: 500; color: var(--text-secondary);">Verifying session...</div>
	</div>
</div>
