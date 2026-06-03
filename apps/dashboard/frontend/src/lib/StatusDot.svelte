<script lang="ts">
	let { color, size = 6, active = true }: { color: string; size?: number; active?: boolean } = $props();
</script>

<span
	class="dot"
	class:active
	style="
		width: {size}px;
		height: {size}px;
		background: {color};
		--dot-color: {color};
	"
></span>

<style>
	.dot {
		display: inline-block;
		border-radius: 50%;
		flex-shrink: 0;
		box-shadow: 0 0 4px var(--dot-color), 0 0 8px color-mix(in srgb, var(--dot-color) 40%, transparent);
	}

	.dot.active {
		animation: crt-pulse var(--crt-pulse-duration, 3.5s) ease-in-out infinite;
	}

	@keyframes crt-pulse {
		0%, 100% { opacity: 1; transform: scale(1); }
		8% { opacity: 0.82; }
		25% { opacity: 1; transform: scale(1.08); }
		42% { opacity: 0.88; }
		60% { opacity: 1; transform: scale(1.03); }
		78% { opacity: 0.85; }
		90% { opacity: 0.95; transform: scale(0.97); }
	}
</style>
