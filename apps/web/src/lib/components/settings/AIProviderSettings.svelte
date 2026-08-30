<script lang="ts">
	import { onMount } from 'svelte';
	import { apiRequest } from '$lib/api';

	let configured = $state(false);
	let apiKey = $state('');
	let baseURL = $state('https://generativelanguage.googleapis.com/v1beta/openai/');
	let completionModel = $state('gemma-4-26b-a4b-it');
	let embeddingModel = $state('gemini-embedding-001');
	let showKey = $state(false);
	let loading = $state(true);
	let saving = $state(false);
	let testing = $state(false);
	let message = $state<{ kind: 'success' | 'error'; text: string } | null>(null);

	onMount(async () => {
		try {
			const status = await apiRequest('/workspace/account/ai-config/status');
			configured = status?.configured === true;
			if (status?.base_url) baseURL = status.base_url;
			if (status?.completion_model) completionModel = status.completion_model;
			if (status?.embedding_model) embeddingModel = status.embedding_model;
		} catch (error: any) {
			message = { kind: 'error', text: error?.message || 'Failed to load AI provider configuration.' };
		} finally {
			loading = false;
		}
	});

	async function testConnection() {
		if (!configured && !apiKey.trim()) {
			message = { kind: 'error', text: 'API key is required to test connection.' };
			return;
		}
		if (!baseURL.trim() || !completionModel.trim() || !embeddingModel.trim()) {
			message = { kind: 'error', text: 'Base URL, completion model, and embedding model are required.' };
			return;
		}

		testing = true;
		message = null;
		try {
			const res = await apiRequest('/workspace/account/ai-config/test', {
				method: 'POST',
				body: {
					config: JSON.stringify({
						api_key: apiKey.trim(),
						base_url: baseURL.trim().replace(/\/$/, ''),
						completion_model: completionModel.trim(),
						embedding_model: embeddingModel.trim()
					})
				}
			});
			message = { kind: 'success', text: res?.message || 'AI provider connection verified successfully!' };
		} catch (error: any) {
			message = { kind: 'error', text: error?.message || 'AI provider test failed.' };
		} finally {
			testing = false;
		}
	}

	async function save() {
		if (!configured && !apiKey.trim()) {
			message = { kind: 'error', text: 'API key is required.' };
			return;
		}
		if (!baseURL.trim() || !completionModel.trim() || !embeddingModel.trim()) {
			message = { kind: 'error', text: 'Base URL, completion model, and embedding model are required.' };
			return;
		}

		saving = true;
		message = null;
		try {
			// Validate credentials first
			await apiRequest('/workspace/account/ai-config/test', {
				method: 'POST',
				body: {
					config: JSON.stringify({
						api_key: apiKey.trim(),
						base_url: baseURL.trim().replace(/\/$/, ''),
						completion_model: completionModel.trim(),
						embedding_model: embeddingModel.trim()
					})
				}
			});

			await apiRequest('/workspace/account/ai-config', {
				method: 'PUT',
				body: {
					config: JSON.stringify({
						api_key: apiKey.trim(),
						base_url: baseURL.trim().replace(/\/$/, ''),
						completion_model: completionModel.trim(),
						embedding_model: embeddingModel.trim()
					})
				}
			});
			apiKey = '';
			showKey = false;
			configured = true;
			message = { kind: 'success', text: 'AI provider configuration saved.' };
		} catch (error: any) {
			message = { kind: 'error', text: error?.message || 'Failed to save AI provider configuration.' };
		} finally {
			saving = false;
		}
	}
</script>

<form class="space-y-6" onsubmit={(event) => { event.preventDefault(); void save(); }} aria-busy={loading}>
	<div>
		<div class="flex flex-wrap items-center gap-2">
			<h2 class="text-base font-medium text-slate-900">AI provider</h2>
			<span class="rounded-md px-2 py-0.5 text-[10px] font-medium {configured ? 'bg-emerald-50 text-emerald-700' : 'bg-amber-50 text-amber-700'}">{loading ? 'Loading' : configured ? 'Configured' : 'Not configured'}</span>
		</div>
		<p class="mt-1 text-xs leading-relaxed text-slate-500">Connect an OpenAI-compatible provider with your own API key. The system encrypts credentials before storage.</p>
	</div>

	{#if message}
		<div role={message.kind === 'error' ? 'alert' : 'status'} class="rounded-xl border p-4 text-xs {message.kind === 'error' ? 'border-rose-200 bg-rose-50 text-rose-700' : 'border-emerald-200 bg-emerald-50 text-emerald-700'}">{message.text}</div>
	{/if}
	{#if configured}
		<div class="rounded-xl border border-emerald-200 bg-emerald-50/60 p-4 text-xs leading-relaxed text-emerald-800">An AI provider is connected. The system encrypts and saves your API key. You can update the base URL and model names without entering a new key.</div>
	{/if}

	<fieldset disabled={loading || saving || testing} class="space-y-4 text-xs disabled:opacity-60">
		<div class="space-y-1.5">
			<label for="aiSettingsApiKey" class="block font-medium text-slate-700">{configured ? 'New API key' : 'API key'}</label>
			<div class="relative">
				<input id="aiSettingsApiKey" type={showKey ? 'text' : 'password'} autocomplete="new-password" bind:value={apiKey} class="wf-input pr-16" placeholder={configured ? 'Enter a replacement key' : 'Enter your provider API key'} />
				<button type="button" onclick={() => showKey = !showKey} class="absolute inset-y-0 right-0 px-3 text-[11px] font-medium text-slate-500 hover:text-slate-800" aria-label={showKey ? 'Hide API key' : 'Show API key'}>{showKey ? 'Hide' : 'Show'}</button>
			</div>
			<p class="text-[11px] leading-relaxed text-slate-500">The system does not show the API key after saving.</p>
		</div>
		<div class="space-y-1.5"><label for="aiSettingsBaseURL" class="block font-medium text-slate-700">OpenAI-compatible base URL</label><input id="aiSettingsBaseURL" type="url" bind:value={baseURL} class="wf-input" required /></div>
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
			<div class="space-y-1.5"><label for="aiSettingsCompletionModel" class="block font-medium text-slate-700">Completion model</label><input id="aiSettingsCompletionModel" bind:value={completionModel} class="wf-input" required /></div>
			<div class="space-y-1.5"><label for="aiSettingsEmbeddingModel" class="block font-medium text-slate-700">Embedding model</label><input id="aiSettingsEmbeddingModel" bind:value={embeddingModel} class="wf-input" required /></div>
		</div>
	</fieldset>
	<div class="flex items-center justify-between border-t border-slate-100 pt-5">
		<button
			type="button"
			onclick={testConnection}
			disabled={loading || saving || testing || (!configured && !apiKey.trim())}
			class="rounded-lg border border-slate-200 bg-white px-4 py-2 text-xs font-medium text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50"
		>
			{testing ? 'Testing...' : 'Test connection'}
		</button>
		<button type="submit" disabled={loading || saving || testing || (!configured && !apiKey.trim())} class="wf-button-primary px-5 py-2.5 disabled:cursor-not-allowed disabled:opacity-50">{saving ? 'Saving...' : configured ? 'Save changes' : 'Save provider'}</button>
	</div>
</form>
