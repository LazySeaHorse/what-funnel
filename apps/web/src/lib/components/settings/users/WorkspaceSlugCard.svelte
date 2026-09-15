<script lang="ts">
	import { onMount } from 'svelte';
	import { apiRequest } from '$lib/api';
	import { Button, Input } from '$lib/components/ui';

	let { slug = $bindable(), onStatus }: { slug: string; onStatus: (kind: 'success' | 'error', message: string) => void } = $props();
	let saving = $state(false);

	onMount(() => { void load(); });

	async function load() {
		try {
			const response = await apiRequest('/workspace/account/slug');
			if (response?.slug) slug = response.slug;
		} catch {
			// The login prefix is optional for older workspaces.
		}
	}

	async function save() {
		if (!slug.trim()) return onStatus('error', 'Slug cannot be empty.');
		saving = true;
		try {
			await apiRequest('/workspace/account/slug', { method: 'PUT', body: { slug: slug.trim() } });
			onStatus('success', 'Workspace slug updated.');
		} catch (error) {
			onStatus('error', error instanceof Error ? error.message : 'Failed to update workspace slug.');
		} finally {
			saving = false;
		}
	}
</script>

<div class="p-4 bg-slate-50/80 border border-slate-200 rounded-2xl space-y-2.5">
	<div class="flex items-center justify-between">
		<label for="settings-workspace-slug" class="text-xs font-medium text-slate-900">Workspace login prefix</label>
		<Button variant="secondary" size="xs" onclick={() => void save()} disabled={saving} busy={saving}>Update prefix</Button>
	</div>
	<Input id="settings-workspace-slug" bind:value={slug} placeholder="company-prefix" class="font-mono" />
	<div class="space-y-1 text-[11px] text-slate-500">
		<p>Team members log in with: <span class="font-mono font-medium text-slate-800 bg-white px-1.5 py-0.5 rounded border border-slate-200">{slug || 'prefix'}-[username]</span></p>
		<p>Agents only see their assigned leads. Managers see all workspace leads.</p>
	</div>
</div>
