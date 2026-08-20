<script lang="ts">
	let {
		stateKey = 'new',
		label = '',
		size = 'sm',
		class: className = ''
	}: {
		stateKey?: string;
		label?: string;
		size?: 'xs' | 'sm' | 'md';
		class?: string;
	} = $props();

	const stateConfig: Record<string, { label: string; bg: string; dot: string }> = {
		new: { label: 'New Lead', bg: 'bg-amber-50 text-amber-700 border-amber-200/60', dot: 'bg-amber-400' },
		contacted: { label: 'Contacted', bg: 'bg-blue-50 text-blue-700 border-blue-200/60', dot: 'bg-blue-500' },
		follow_up: { label: 'Follow-up', bg: 'bg-purple-50 text-purple-700 border-purple-200/60', dot: 'bg-purple-500' },
		interested: { label: 'Interested', bg: 'bg-emerald-50 text-emerald-700 border-emerald-200/60', dot: 'bg-emerald-500' },
		converted: { label: 'Converted', bg: 'bg-emerald-50 text-emerald-700 border-emerald-200/60', dot: 'bg-emerald-500' },
		closed_won: { label: 'Converted', bg: 'bg-emerald-50 text-emerald-700 border-emerald-200/60', dot: 'bg-emerald-500' }
	};

	const currentInfo = $derived(
		stateConfig[stateKey] || {
			label: label || stateKey.replace(/_/g, ' '),
			bg: 'bg-slate-50 text-slate-700 border-slate-200/80',
			dot: 'bg-slate-400'
		}
	);

	const displayLabel = $derived(label || currentInfo.label);

	const sizeClasses = {
		xs: 'px-2 py-0.5 text-[11px] gap-1.5',
		sm: 'px-2.5 py-1 text-xs gap-1.5',
		md: 'px-3 py-1.5 text-xs gap-2'
	};

	const dotSizes = {
		xs: 'w-1.5 h-1.5',
		sm: 'w-1.5 h-1.5',
		md: 'w-2 h-2'
	};
</script>

<span class="inline-flex items-center rounded-lg border font-medium {currentInfo.bg} {sizeClasses[size]} {className}">
	<span class="rounded-full shrink-0 {currentInfo.dot} {dotSizes[size]}"></span>
	<span class="truncate">{displayLabel}</span>
</span>
