<script lang="ts">
	import type { NocWorkload } from './types';
	import { formatCPU, formatBytes } from '../format';

	let { wl }: { wl: NocWorkload } = $props();
</script>

<div class="detail" style="--accent: {wl.color}">
	<div class="head">{wl.label}</div>
	<div class="row"><span class="k">namespace</span><span class="v" style="color:{wl.color}">{wl.namespace}</span></div>
	<div class="row"><span class="k">kind</span><span class="v">{wl.kind}</span></div>
	<div class="row"><span class="k">node</span><span class="v">{wl.nodeId}</span></div>
	<div class="row"><span class="k">pods</span><span class="v">{wl.pods}</span></div>
	<div class="row"><span class="k">cpu</span><span class="v">{formatCPU(wl.cpu)}</span></div>
	<div class="row"><span class="k">memory</span><span class="v">{formatBytes(wl.memory)}</span></div>
	{#if wl.isService}<div class="svc">tailscale service</div>{/if}
</div>

<style>
	.detail {
		background: rgba(10, 10, 15, 0.95);
		border: 1px solid var(--accent);
		border-radius: 3px;
		padding: 10px 14px;
		min-width: 180px;
		box-shadow: 0 0 16px color-mix(in srgb, var(--accent) 25%, transparent);
	}
	.head { font-size: 12px; font-weight: 700; color: var(--text-primary); margin-bottom: 6px; padding-bottom: 5px; border-bottom: 1px solid var(--border-default); }
	.row { display: flex; justify-content: space-between; font-size: 10px; padding: 1px 0; }
	.k { color: var(--text-muted); text-transform: uppercase; font-size: 8px; letter-spacing: 0.5px; }
	.v { color: var(--text-primary); font-weight: 500; }
	.svc { margin-top: 5px; font-size: 8px; text-transform: uppercase; letter-spacing: 0.5px; color: var(--neon-cyan); border: 1px solid var(--neon-cyan); padding: 1px 5px; border-radius: 2px; display: inline-block; }
</style>
