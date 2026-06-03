<script lang="ts">
	let { path, size = 20, color = 'currentColor' }: { path: string; size?: number; color?: string } = $props();
</script>

<svg viewBox="0 0 24 24" width={size} height={size} fill={color} aria-hidden="true">
	<path d={path} />
</svg>
