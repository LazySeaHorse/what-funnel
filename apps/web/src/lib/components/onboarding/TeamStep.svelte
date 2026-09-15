<script lang="ts">
	import {
		PlusIcon,
		CheckIcon,
		DocumentDuplicateIcon,
		TrashIcon
	} from '@fvilers/heroicons-svelte/24/outline';
	import { Button, Input } from '$lib/components/ui';

	type User = { id: string; username: string; role: string; plaintextPassword?: string };

	let {
		step,
		totalSteps,
		slug = $bindable(),
		users = $bindable(),
		onAddUser,
		onRemoveUser
	}: {
		step: number;
		totalSteps: number;
		slug: string;
		users: User[];
		onAddUser: (username: string, password: string, role: 'agent' | 'manager') => Promise<void>;
		onRemoveUser: (id: string) => Promise<void>;
	} = $props();

	let newUsername = $state('');
	let newPassword = $state('');
	let newRole = $state<'agent' | 'manager'>('agent');
	let adding = $state(false);
	let userError = $state('');
	let copiedPassId = $state<string | null>(null);

	function generatePassword() {
		const chars = 'abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%';
		let pass = '';
		for (let i = 0; i < 12; i++) {
			pass += chars.charAt(Math.floor(Math.random() * chars.length));
		}
		newPassword = pass;
	}

	async function copyPassword(id: string, pass: string) {
		try {
			await navigator.clipboard.writeText(pass);
			copiedPassId = id;
			setTimeout(() => {
				if (copiedPassId === id) copiedPassId = null;
			}, 2000);
		} catch {}
	}

	async function addUser() {
		userError = '';
		if (!newUsername.trim()) { userError = 'Username is required.'; return; }
		if (!newPassword.trim()) { userError = 'Password is required.'; return; }
		adding = true;
		try {
			await onAddUser(newUsername.trim(), newPassword.trim(), newRole);
			newUsername = '';
			newPassword = '';
			newRole = 'agent';
		} catch (err: any) {
			userError = err?.message || 'Failed to add user.';
		} finally {
			adding = false;
		}
	}
</script>

<div class="text-center lg:text-left mb-6">
	<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {step} of {totalSteps}</div>
	<h2 class="text-2xl sm:text-3xl font-medium text-slate-900 tracking-tight mb-1">Add team members</h2>
	<p class="text-sm text-slate-500 font-normal">Set the workspace login prefix and add users to your team.</p>
</div>

<div class="space-y-6 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
	<!-- Workspace Slug Setup -->
	<div class="p-4 sm:p-5 bg-slate-50/80 border border-slate-200 rounded-2xl space-y-3">
		<Input
			id="workspace-slug"
			label="Workspace login prefix"
			type="text"
			class="font-mono"
			placeholder="company-name"
			bind:value={slug}
		/>
		<div class="space-y-1 text-xs text-slate-500 font-normal">
			<p>
				Team members log in with: <span class="font-mono font-medium text-slate-800 bg-white px-2 py-0.5 rounded border border-slate-200">{slug || 'your-company'}-[username]</span>
			</p>
			<p>
				Agents only see their assigned leads. Managers see all workspace leads.
			</p>
		</div>
	</div>

	<!-- Add Team Member Form -->
	<div class="p-4 sm:p-5 bg-white border border-slate-200 rounded-2xl space-y-4">
		<h3 class="text-sm font-medium text-slate-900">Add team member</h3>

		<div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
			<Input
				id="new-member-username"
				label="Username"
				type="text"
				placeholder="e.g. john"
				bind:value={newUsername}
			/>

			<Input
				id="new-member-password"
				label="Password"
				type="text"
				class="font-mono"
				placeholder="Password"
				bind:value={newPassword}
			>
				{#snippet trailing()}
					<button type="button" class="text-[10px] text-blue-600 hover:underline cursor-pointer" onclick={generatePassword}>Generate</button>
				{/snippet}
			</Input>

			<div>
				<label for="new-member-role" class="block text-xs font-medium text-slate-700 mb-1">Role</label>
				<div class="flex gap-2">
					<select
						id="new-member-role"
						bind:value={newRole}
						class="flex-1 px-3 py-2 bg-white border border-slate-200 rounded-xl text-xs text-slate-900 focus:border-blue-600 focus:ring-1 focus:ring-blue-100 outline-none cursor-pointer"
					>
						<option value="agent">Agent</option>
						<option value="manager">Manager</option>
					</select>
					<Button
						variant="primary"
						onclick={addUser}
						disabled={adding || !newUsername.trim() || !newPassword.trim()}
						busy={adding}
					>
						<PlusIcon class="w-3.5 h-3.5" />
						<span>Add</span>
					</Button>
				</div>
			</div>
		</div>

		{#if userError}
			<p class="text-xs text-rose-600 font-medium">{userError}</p>
		{/if}
	</div>

	<!-- Created Members List -->
	{#if users.length > 0}
		<div class="border border-slate-200 rounded-2xl overflow-hidden divide-y divide-slate-100 bg-white">
			<div class="px-4 py-2.5 bg-slate-50/70 text-xs font-medium text-slate-500">
				Team members ({users.length})
			</div>
			{#each users as member}
				<div class="p-3.5 sm:p-4 flex items-center justify-between gap-3">
					<div class="flex items-center gap-3 min-w-0">
						<div class="w-8 h-8 rounded-full bg-blue-100 text-blue-700 font-medium flex items-center justify-center text-xs shrink-0">
							{member.username.charAt(0).toUpperCase()}
						</div>
						<div class="min-w-0">
							<div class="font-medium text-xs sm:text-sm text-slate-900 truncate">
								{member.username}
							</div>
							<div class="text-[11px] text-slate-400 font-mono">
								{(slug || 'prefix') + '-' + member.username}
							</div>
						</div>
					</div>

					<div class="flex items-center gap-2 shrink-0">
						<span class="px-2 py-0.5 rounded-md text-[11px] font-medium {member.role === 'manager' ? 'bg-purple-100 text-purple-700' : 'bg-slate-100 text-slate-700'} capitalize">
							{member.role}
						</span>

						{#if member.plaintextPassword}
							<button
								type="button"
								class="flex items-center gap-1 px-2 py-1 rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-700 text-[11px] font-mono transition cursor-pointer"
								onclick={() => copyPassword(member.id, member.plaintextPassword!)}
								title="Copy password"
							>
								<span>{member.plaintextPassword}</span>
								{#if copiedPassId === member.id}
									<CheckIcon class="w-3 h-3 text-emerald-600" />
								{:else}
									<DocumentDuplicateIcon class="w-3 h-3 text-slate-500" />
								{/if}
							</button>
						{/if}

						<button
							type="button"
							class="p-1 text-slate-400 hover:text-rose-600 rounded-lg hover:bg-rose-50 transition cursor-pointer"
							onclick={() => onRemoveUser(member.id)}
							title="Remove user"
						>
							<TrashIcon class="w-3.5 h-3.5" />
						</button>
					</div>
				</div>
			{/each}
		</div>
	{/if}

	<p class="text-xs text-slate-400 text-center lg:text-left">
		You can add more team members in Settings.
	</p>
</div>
