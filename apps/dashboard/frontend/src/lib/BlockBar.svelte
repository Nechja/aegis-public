<script lang="ts">
	import { blockBar, barColor } from './format';

	let { pct, width = 24 }: { pct: number; width?: number } = $props();
</script>

<span class="bar" style="color: {barColor(pct)}">{blockBar(pct, width)}</span>

<style>
	.bar {
		font-size: 9px;
		line-height: 1;
		letter-spacing: -0.5px;
		flex: 1;
		transition: color 0.4s ease;
	}
</style>
