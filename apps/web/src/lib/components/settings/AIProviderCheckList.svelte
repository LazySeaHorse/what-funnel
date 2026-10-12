<script lang="ts">
	import type { AIProviderTestCheck } from '$lib/ai-provider';

	let { checks, class: className = 'space-y-2' }: { checks: AIProviderTestCheck[]; class?: string } = $props();
</script>

{#if checks.length > 0}
	<ul class={className} aria-label="AI provider check results">
		{#each checks as check (check.role)}
			<li
				class="flex items-start justify-between gap-3 rounded-lg border px-3 py-2 text-xs {check.ok
					? 'border-emerald-200 bg-emerald-50/70 text-emerald-800'
					: 'border-rose-200 bg-rose-50/70 text-rose-800'}"
			>
				<span>
					<span class="font-medium capitalize">{check.role}</span> · {check.model}
					{#if check.resolved_model && check.resolved_model !== check.model}
						→ {check.resolved_model}
					{/if}
				</span>
				<span class="text-right">{check.message}</span>
			</li>
		{/each}
	</ul>
{/if}
