<script lang="ts">
	import type { Snippet } from 'svelte';
	let { title, children }: { title: string; children: Snippet } = $props();
</script>

<div class="panel">
	<div class="panel-header">{title}</div>
	{@render children()}
</div>

<style>
	.panel {
		flex-shrink: 0;
	}

	.panel-header {
		font-size: 11px;
		font-weight: 600;
		color: var(--neon-orange);
		text-transform: uppercase;
		letter-spacing: 1.5px;
		padding: 4px 0;
		margin-bottom: 8px;
		border-bottom: 1px solid var(--border-default);
		text-shadow: var(--glow-orange);
	}
</style>
