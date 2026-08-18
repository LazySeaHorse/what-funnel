<script lang="ts">
	import '../../app.css';

	let { children } = $props();
</script>

<div class="onboarding-root">
	{@render children()}
</div>

<style>
	:global(body) {
		margin: 0;
		padding: 0;
		background-color: #FFFFFF;
		font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
		color: #1E293B;
		-webkit-font-smoothing: antialiased;
		overflow-x: hidden;
	}

	.onboarding-root {
		min-height: 100vh;
		width: 100%;
		display: flex;
		flex-direction: column;
		background-color: #FFFFFF;
		box-sizing: border-box;
	}
</style>
