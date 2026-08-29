<script lang="ts">
	import { onMount } from 'svelte';
	import { apiRequest } from '$lib/api';
	import type { WorkspaceState } from '$lib/workspace.svelte';

	let { workspace }: { workspace?: WorkspaceState } = $props();
	let channels = $state<any[]>([]);
	let connections = $state<any[]>([]);
	let showDialog = $state(false);
	let platform = $state<'whatsapp' | 'instagram' | 'messenger' | 'telegram'>('whatsapp');
	let activeConnection = $state<any>(null);
	let secret = $state('');
	let code = $state('');
	let busy = $state(false);
	let loading = $state(true);
	let qrRefreshToken = $state(Date.now());
	let error = $state('');
	let notice = $state('');

	function platformName(value: string) {
		return ({ whatsapp: 'WhatsApp', instagram: 'Instagram', messenger: 'Messenger', telegram: 'Telegram' } as Record<string, string>)[value] || value;
	}

	function channelName(channel: any) {
		return platformName(channel.type.replace('matrix_', ''));
	}

	function connectionFor(channelID: string) {
		return connections.find((connection) => connection.channel_id === channelID);
	}

	async function refresh(refreshBridge = false) {
		const connectionPath = refreshBridge ? '/bridge-connections?refresh=true' : '/bridge-connections';
		const [channelResult, connectionResult] = await Promise.all([apiRequest('/channels'), apiRequest(connectionPath)]);
		channels = (Array.isArray(channelResult) ? channelResult : []).filter((channel) => channel.status !== 'disconnected');
		connections = Array.isArray(connectionResult) ? connectionResult : [];
		if (activeConnection) activeConnection = connections.find((connection) => connection.channel_id === activeConnection.channel_id) || activeConnection;
	}

	onMount(async () => {
		try { await refresh(); }
		catch (reason: any) { error = reason?.message || 'Failed to load channels.'; }
		finally { loading = false; }
	});

	function closeDialog() {
		showDialog = false;
		activeConnection = null;
		secret = '';
		code = '';
	}

	async function startConnection() {
		busy = true;
		error = '';
		try {
			activeConnection = await apiRequest('/bridge-connections', { method: 'POST', body: { platform } });
			qrRefreshToken = Date.now();
			await refresh(true);
		} catch (reason: any) { error = reason?.message || 'Failed to connect channel.'; }
		finally { busy = false; }
	}

	async function submitSession() {
		if (!activeConnection || !secret.trim()) return;
		busy = true;
		try {
			activeConnection = await apiRequest(`/bridge-connections/${activeConnection.channel_id}/session`, { method: 'POST', body: { session: secret } });
			secret = '';
			await refresh(true);
		} catch (reason: any) { error = reason?.message || 'Failed to hand the session to the bridge.'; }
		finally { busy = false; }
	}

	async function submitCode() {
		if (!activeConnection || !code.trim()) return;
		busy = true;
		try {
			activeConnection = await apiRequest(`/bridge-connections/${activeConnection.channel_id}/code`, { method: 'POST', body: { code } });
			code = '';
			await refresh(true);
		} catch (reason: any) { error = reason?.message || 'Failed to send the login response.'; }
		finally { busy = false; }
	}

	async function disconnect(channelID: string) {
		if (!confirm('Disconnect this channel? Existing conversations will remain available.')) return;
		try {
			await apiRequest(`/channels/${channelID}/disconnect`, { method: 'POST' });
			await workspace?.refreshChannels();
			await refresh();
			notice = 'Channel disconnected.';
		} catch (reason: any) { error = reason?.message || 'Failed to disconnect channel.'; }
	}

	$effect(() => {
		if (!showDialog || !activeConnection || ['connected', 'failed', 'cancelled'].includes(activeConnection.state)) return;
		const timer = window.setInterval(async () => {
			try { await refresh(true); qrRefreshToken = Date.now(); } catch {}
		}, 3500);
		return () => window.clearInterval(timer);
	});
</script>

<svelte:window onkeydown={(event) => { if (event.key === 'Escape') closeDialog(); }} />

<div class="space-y-6" aria-busy={loading}>
	<div class="flex items-center justify-between gap-4">
		<div><h2 class="text-base font-medium text-slate-900">Connected channels</h2><p class="mt-1 text-xs text-slate-500">Connect and manage customer messaging accounts.</p></div>
		<button onclick={() => { showDialog = true; notice = ''; }} class="rounded-xl border border-blue-200 bg-blue-50 px-3 py-2 text-xs font-medium text-blue-600 hover:bg-blue-100">Connect channel</button>
	</div>
	{#if notice}<div role="status" class="rounded-xl border border-emerald-200 bg-emerald-50 p-3 text-xs text-emerald-700">{notice}</div>{/if}
	{#if error}<div role="alert" class="rounded-xl border border-rose-200 bg-rose-50 p-3 text-xs text-rose-700">{error}</div>{/if}
	{#if loading}<div role="status" class="py-6 text-xs text-slate-500">Loading channels…</div>
	{:else}<div class="space-y-3">
		{#each channels as channel (channel.id)}
			{@const connection = connectionFor(channel.id)}
			<div class="flex items-center justify-between gap-4 rounded-xl border border-slate-200 p-4">
				<div><div class="text-sm font-medium text-slate-800">{channelName(channel)}</div><div class="mt-1 text-[11px] text-slate-500">{connection?.detail || channel.status}</div></div>
				<div class="flex items-center gap-2">{#if connection && connection.state !== 'connected'}<button onclick={() => { activeConnection = connection; showDialog = true; qrRefreshToken = Date.now(); }} class="rounded-lg px-2.5 py-1.5 text-xs font-medium text-blue-600 hover:bg-blue-50">Continue</button>{/if}<button onclick={() => disconnect(channel.id)} class="rounded-lg px-2.5 py-1.5 text-xs font-medium text-rose-600 hover:bg-rose-50">Disconnect</button></div>
			</div>
		{/each}
		{#if channels.length === 0}<div class="rounded-xl border border-dashed border-slate-200 p-6 text-center text-xs text-slate-500">No channels connected yet.</div>{/if}
	</div>{/if}
</div>

{#if showDialog}
	<div class="fixed inset-0 z-[100] flex items-center justify-center bg-slate-950/40 p-4" role="presentation" onclick={(event) => { if (event.currentTarget === event.target) closeDialog(); }}>
		<div class="w-full max-w-lg rounded-2xl bg-white p-5 shadow-2xl" role="dialog" aria-modal="true" aria-labelledby="connect-channel-title">
			<div class="flex items-center justify-between gap-4"><div><h3 id="connect-channel-title" class="text-sm font-medium text-slate-900">{activeConnection ? `Connect ${platformName(activeConnection.platform)}` : 'Connect a channel'}</h3><p class="mt-1 text-xs text-slate-500">{activeConnection ? activeConnection.detail : 'WhatFunnel creates an isolated Matrix bridge user and guides the provider-specific login.'}</p></div><button aria-label="Close channel dialog" onclick={closeDialog} class="text-lg text-slate-400 hover:text-slate-600">×</button></div>
			{#if !activeConnection}
				<div class="my-5 space-y-3">
					<label class="block text-xs font-medium text-slate-700">Channel
						<select aria-label="Channel" bind:value={platform} class="wf-input mt-1.5 w-full">
							{#each ['whatsapp', 'instagram', 'messenger', 'telegram'] as option}
								<option value={option}>{platformName(option)}</option>
							{/each}
						</select>
					</label>
				</div>
				<div class="flex justify-end gap-2"><button onclick={closeDialog} class="wf-button px-3 py-2 text-slate-600 hover:bg-slate-100">Cancel</button><button onclick={startConnection} disabled={busy} class="wf-button-primary px-3 py-2">{busy ? 'Starting…' : 'Continue'}</button></div>
			{:else if activeConnection.state === 'awaiting_scan'}
				<div class="my-5 space-y-4"><div class="mx-auto h-60 w-60 overflow-hidden rounded-xl border border-slate-200 bg-white p-2"><img class="h-full w-full object-contain" src={`/api-gateway/bridge-connections/${activeConnection.channel_id}/qr?refresh=${qrRefreshToken}`} alt={`QR code for ${platformName(activeConnection.platform)} connection`} /></div><p class="rounded-xl border border-blue-100 bg-blue-50 p-3 text-[11px] leading-5 text-blue-800">{activeConnection.platform === 'telegram' ? 'In Telegram, open Settings, Devices, then Link Desktop Device. Scan this code and complete any two-factor prompt.' : 'In WhatsApp, open Settings, Linked devices, then Link a device. Scan this code with your phone.'}</p></div>
				<div class="flex justify-end gap-2"><button onclick={() => { qrRefreshToken = Date.now(); void refresh(true); }} class="wf-button px-3 py-2 text-slate-600 hover:bg-slate-100">Refresh QR</button><button onclick={closeDialog} class="wf-button-primary px-3 py-2">I’ve scanned it</button></div>
			{:else if activeConnection.state === 'awaiting_code'}
				<label class="my-5 block text-xs font-medium text-slate-700">Verification code<input bind:value={code} autocomplete="one-time-code" class="wf-input mt-1.5" placeholder="Verification code or 2FA password" /></label><div class="flex justify-end gap-2"><button onclick={closeDialog} class="wf-button px-3 py-2 text-slate-600 hover:bg-slate-100">Cancel</button><button onclick={submitCode} disabled={busy || !code.trim()} class="wf-button-primary px-3 py-2">{busy ? 'Sending…' : 'Submit'}</button></div>
			{:else if activeConnection.state === 'awaiting_session'}
				<div class="my-5 space-y-3 text-xs leading-5 text-slate-600"><p>Open {activeConnection.platform === 'instagram' ? 'instagram.com' : 'messenger.com'} in a private browser window and sign in. In browser developer tools, copy an authenticated GraphQL request as POSIX cURL.</p><label class="block font-medium text-slate-700">Copied cURL request<textarea bind:value={secret} autocomplete="off" spellcheck="false" class="wf-input mt-1.5 min-h-32 font-mono text-[11px]" placeholder="Paste the copied cURL request"></textarea></label></div><div class="flex justify-end gap-2"><button onclick={closeDialog} class="wf-button px-3 py-2 text-slate-600 hover:bg-slate-100">Cancel</button><button onclick={submitSession} disabled={busy || !secret.trim()} class="wf-button-primary px-3 py-2">{busy ? 'Handing off…' : 'Connect'}</button></div>
			{:else if activeConnection.state === 'connected'}<div class="my-5 rounded-xl border border-emerald-200 bg-emerald-50 p-4 text-xs leading-5 text-emerald-800">{platformName(activeConnection.platform)} is connected. New chats will sync through the bridge.</div><div class="flex justify-end"><button onclick={closeDialog} class="wf-button-primary px-3 py-2">Done</button></div>
			{:else if activeConnection.state === 'failed'}<div class="my-5 rounded-xl border border-rose-200 bg-rose-50 p-4 text-xs leading-5 text-rose-800">{activeConnection.detail || 'The bridge could not complete this login.'}</div><div class="flex justify-end"><button onclick={closeDialog} class="wf-button-primary px-3 py-2">Close</button></div>
			{:else}<div class="my-5 rounded-xl border border-amber-200 bg-amber-50 p-4 text-xs leading-5 text-amber-800">{activeConnection.detail || 'Waiting for the bridge.'}</div><div class="flex justify-end"><button onclick={() => refresh(true)} class="wf-button-primary px-3 py-2">Check status</button></div>{/if}
		</div>
	</div>
{/if}
