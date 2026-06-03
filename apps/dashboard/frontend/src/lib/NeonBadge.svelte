<script lang="ts">
	import type { Snippet } from 'svelte';
	let { color = 'var(--text-muted)', glow = true, children }: {
		color?: string;
		glow?: boolean;
		children: Snippet;
	} = $props();
</script>

<span
	class="badge"
	class:glow
	style="
		color: {color};
		border-color: {color};
		--badge-color: {color};
	"
>
	{@render children()}
</span>

<style>
	.badge {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		font-size: 10px;
		font-weight: 500;
		padding: 1px 6px;
		border: 1px solid;
		border-radius: 2px;
		text-transform: uppercase;
		letter-spacing: 0.5px;
		white-space: nowrap;
		transition: box-shadow 0.2s ease;
	}

	.badge.glow {
		box-shadow: 0 0 4px var(--badge-color), 0 0 8px color-mix(in srgb, var(--badge-color) 25%, transparent);
		text-shadow: 0 0 3px var(--badge-color);
	}

	.badge.glow:hover {
		box-shadow: 0 0 6px var(--badge-color), 0 0 14px color-mix(in srgb, var(--badge-color) 35%, transparent);
	}
</style>
