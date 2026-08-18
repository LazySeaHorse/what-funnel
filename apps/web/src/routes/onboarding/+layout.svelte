<script lang="ts">
	import '../../app.css';

	let { children } = $props();
</script>

<div class="min-h-[100dvh] w-full flex flex-col bg-white text-slate-800 font-sans antialiased">
	{@render children()}
</div>
