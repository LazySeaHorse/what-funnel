<script lang="ts">
	import { onMount } from 'svelte';
	import { SimulatorController } from '$lib/components/simulator/SimulatorController.svelte';
	import SimulatorControls from '$lib/components/simulator/SimulatorControls.svelte';
	import SimulatorConversation from '$lib/components/simulator/SimulatorConversation.svelte';
	import SimulatorDiagnostics from '$lib/components/simulator/SimulatorDiagnostics.svelte';

	const controller = new SimulatorController();
	onMount(() => {
		controller.start();
		return () => controller.dispose();
	});
</script>

<div class="grid grid-cols-1 lg:grid-cols-12 gap-5 h-full min-h-0 text-slate-800 text-xs">
	<SimulatorControls {controller} />
	<SimulatorConversation {controller} />
	<SimulatorDiagnostics {controller} />
</div>
