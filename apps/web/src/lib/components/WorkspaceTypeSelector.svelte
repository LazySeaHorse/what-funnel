<script lang="ts">
	import { ViewColumnsIcon, CpuChipIcon } from '@fvilers/heroicons-svelte/24/outline';

	let {
		value = $bindable('full_workspace'),
		name = 'product_mode',
		disabled = false,
		onchange
	}: {
		value?: string;
		name?: string;
		disabled?: boolean;
		onchange?: (val: string) => void;
	} = $props();

	function handleChange(mode: string) {
		if (disabled) return;
		onchange?.(mode);
		value = mode;
	}
</script>

<div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
	<label
		class="relative flex items-center gap-2.5 p-3 border rounded-xl cursor-pointer transition-all duration-150 {value === 'full_workspace' ? 'border-blue-600 bg-blue-50/50 ring-1 ring-blue-600' : 'border-slate-200 bg-white hover:bg-slate-50'} {disabled ? 'opacity-60 cursor-not-allowed' : ''}"
	>
		<input
			type="radio"
			{name}
			value="full_workspace"
			checked={value === 'full_workspace'}
			onchange={() => handleChange('full_workspace')}
			{disabled}
			class="absolute inset-0 opacity-0 cursor-pointer z-10"
		/>
		<div class="w-7 h-7 rounded-lg {value === 'full_workspace' ? 'bg-blue-600 text-white' : 'bg-slate-100 text-slate-500'} flex items-center justify-center shrink-0 pointer-events-none">
			<ViewColumnsIcon class="w-3.5 h-3.5" />
		</div>
		<div class="text-left pointer-events-none">
			<div class="text-xs font-medium text-slate-800 leading-tight">Full workspace</div>
			<div class="text-[11px] text-slate-400 leading-tight mt-0.5 font-normal">Inbox and leads</div>
		</div>
	</label>

	<label
		class="relative flex items-center gap-2.5 p-3 border rounded-xl cursor-pointer transition-all duration-150 {value === 'chatbot_only' ? 'border-blue-600 bg-blue-50/50 ring-1 ring-blue-600' : 'border-slate-200 bg-white hover:bg-slate-50'} {disabled ? 'opacity-60 cursor-not-allowed' : ''}"
	>
		<input
			type="radio"
			{name}
			value="chatbot_only"
			checked={value === 'chatbot_only'}
			onchange={() => handleChange('chatbot_only')}
			{disabled}
			class="absolute inset-0 opacity-0 cursor-pointer z-10"
		/>
		<div class="w-7 h-7 rounded-lg {value === 'chatbot_only' ? 'bg-blue-600 text-white' : 'bg-slate-100 text-slate-500'} flex items-center justify-center shrink-0 pointer-events-none">
			<CpuChipIcon class="w-3.5 h-3.5" />
		</div>
		<div class="text-left pointer-events-none">
			<div class="text-xs font-medium text-slate-800 leading-tight">Chatbot only</div>
			<div class="text-[11px] text-slate-400 leading-tight mt-0.5 font-normal">Automations</div>
		</div>
	</label>
</div>
