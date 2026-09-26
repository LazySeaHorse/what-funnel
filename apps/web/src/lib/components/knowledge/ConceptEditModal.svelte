<script lang="ts">
	import { Button } from '$lib/components/ui';
	import { apiRequest } from '$lib/api';
	import type { ConceptItem } from '$lib/components/knowledge/ConceptCard.svelte';

	interface Props {
		concept: ConceptItem;
		onSave?: (updated: ConceptItem) => void;
		onCancel?: () => void;
	}

	let { concept, onSave, onCancel }: Props = $props();

	// svelte-ignore state_referenced_locally
	let draft = $state<{ title: string; type: string; tags: string[]; body_text: string }>({
		title: concept.title || '',
		type: concept.type || 'faq',
		tags: Array.isArray(concept.tags) ? [...concept.tags] : [],
		body_text: concept.body_text || ''
	});
	let tagInput = $state('');
	let saving = $state(false);
	let saveError = $state('');

	function addTag() {
		const val = tagInput.trim().replace(/^#/, '');
		if (!val) return;
		if (!draft.tags.includes(val)) {
			draft.tags = [...draft.tags, val];
		}
		tagInput = '';
	}

	function removeTag(tag: string) {
		draft.tags = draft.tags.filter((t) => t !== tag);
	}

	async function handleSave() {
		if (!draft.title.trim()) {
			saveError = 'Title is required';
			return;
		}
		if (!draft.body_text.trim()) {
			saveError = 'Content is required';
			return;
		}
		saving = true;
		saveError = '';
		try {
			const res = await apiRequest(`/api/kb/concepts/${concept.id}`, {
				method: 'PUT',
				body: draft
			});
			const updated: ConceptItem = res.concept || { ...draft, id: concept.id };
			onSave?.(updated);
		} catch (err: any) {
			saveError = err.message || 'Failed to save concept changes';
		} finally {
			saving = false;
		}
	}
</script>

<div class="border-2 border-blue-500/60 rounded-2xl p-4 bg-white shadow-sm space-y-3 transition">
	<div class="flex items-center justify-between gap-2 border-b border-slate-100 pb-2.5">
		<span class="text-xs font-medium text-slate-900 uppercase tracking-wider">Edit Concept</span>
		{#if saveError}
			<span class="text-xs text-rose-600 font-medium">{saveError}</span>
		{/if}
	</div>
	<div class="grid grid-cols-1 sm:grid-cols-3 gap-2.5">
		<div class="sm:col-span-2">
			<label for={`edit-concept-title-${concept.id}`} class="block text-[11px] font-medium text-slate-500 mb-1">Title</label>
			<input
				id={`edit-concept-title-${concept.id}`}
				bind:value={draft.title}
				placeholder="Concept title"
				class="w-full bg-slate-50/50 focus:bg-white border border-slate-200 rounded-xl px-3 py-1.5 text-xs font-medium text-slate-900 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 transition"
			/>
		</div>
		<div>
			<label for={`edit-concept-type-${concept.id}`} class="block text-[11px] font-medium text-slate-500 mb-1">Category</label>
			<select
				id={`edit-concept-type-${concept.id}`}
				bind:value={draft.type}
				class="w-full bg-slate-50/50 focus:bg-white border border-slate-200 rounded-xl px-2.5 py-1.5 text-xs font-medium text-slate-800 outline-none focus:border-blue-500 capitalize transition"
			>
				<option value="faq">FAQ</option>
				<option value="pricing">Pricing</option>
				<option value="policy">Policy</option>
				<option value="hours">Hours</option>
				<option value="service">Service</option>
				<option value="general">General</option>
			</select>
		</div>
	</div>
	<div>
		<label for={`edit-concept-body-${concept.id}`} class="block text-[11px] font-medium text-slate-500 mb-1">Knowledge Content</label>
		<textarea
			id={`edit-concept-body-${concept.id}`}
			bind:value={draft.body_text}
			rows="4"
			class="w-full bg-slate-50/50 focus:bg-white border border-slate-200 rounded-xl p-3 text-xs text-slate-700 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/10 leading-relaxed transition"
		></textarea>
	</div>
	<div>
		<label for={`edit-concept-tag-input-${concept.id}`} class="block text-[11px] font-medium text-slate-500 mb-1">Tags</label>
		<div class="flex flex-wrap items-center gap-1.5 mb-1.5">
			{#each draft.tags as tag}
				<span class="inline-flex items-center gap-1 text-[11px] text-slate-700 bg-slate-100 border border-slate-200/80 px-2 py-0.5 rounded-md font-medium">
					<span>{tag}</span>
					<button type="button" onclick={() => removeTag(tag)} class="text-slate-400 hover:text-rose-600 cursor-pointer">×</button>
				</span>
			{/each}
		</div>
		<div class="flex items-center gap-2">
			<input
				id={`edit-concept-tag-input-${concept.id}`}
				bind:value={tagInput}
				onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), addTag())}
				placeholder="Add tag and press Enter"
				class="flex-1 max-w-xs bg-slate-50/50 focus:bg-white border border-slate-200 rounded-xl px-2.5 py-1 text-xs text-slate-700 outline-none focus:border-blue-500 transition"
			/>
			<Button variant="secondary" size="xs" onclick={addTag}>Add</Button>
		</div>
	</div>
	<div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
		<Button variant="ghost" size="sm" onclick={onCancel}>
			Cancel
		</Button>
		<Button
			variant="primary"
			size="sm"
			onclick={handleSave}
			disabled={saving}
			busy={saving}
		>
			Save changes
		</Button>
	</div>
</div>
