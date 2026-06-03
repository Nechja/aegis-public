<script lang="ts">
	import { mdiServerNetwork, mdiCpu64Bit, mdiMemory, mdiPackageVariantClosed, mdiLanConnect } from '@mdi/js';
	import Icon from './Icon.svelte';
	import BlockBar from './BlockBar.svelte';
	import { formatBytes } from './format';
	import type { ClusterSummary } from './types';

	let { summary }: { summary: ClusterSummary } = $props();

	const cpuPct = $derived(summary.totalCPU > 0 ? (summary.usedCPU / summary.totalCPU) * 100 : 0);
	const memPct = $derived(summary.totalMemory > 0 ? (summary.usedMemory / summary.totalMemory) * 100 : 0);
	const nodeHealthy = $derived(summary.readyNodes === summary.totalNodes);
</script>

<div class="health-grid">
	<div class="health-cell">
		<span class="health-label">
			<Icon path={mdiServerNetwork} size={14} color={nodeHealthy ? 'var(--neon-green)' : 'var(--neon-rose)'} />
			nodes
		</span>
		<span class="health-value" style="color: {nodeHealthy ? 'var(--neon-green)' : 'var(--neon-rose)'}">
			{summary.readyNodes}/{summary.totalNodes}
		</span>
	</div>

	<div class="health-cell wide">
		<span class="health-label">
			<Icon path={mdiCpu64Bit} size={14} />
			cpu
		</span>
		<BlockBar pct={cpuPct} width={30} />
		<span class="health-value">{cpuPct.toFixed(0)}%</span>
	</div>

	<div class="health-cell wide">
		<span class="health-label">
			<Icon path={mdiMemory} size={14} />
			mem
		</span>
		<BlockBar pct={memPct} width={30} />
		<span class="health-value">{formatBytes(summary.usedMemory)}/{formatBytes(summary.totalMemory)}</span>
	</div>

	<div class="health-cell">
		<span class="health-label">
			<Icon path={mdiPackageVariantClosed} size={14} />
			pods
		</span>
		<span class="health-value">{summary.totalPods}</span>
	</div>

	<div class="health-cell">
		<span class="health-label">
			<Icon path={mdiLanConnect} size={14} />
			svc
		</span>
		<span class="health-value">{summary.services}</span>
	</div>
</div>

<style>
	.health-grid {
		display: flex;
		gap: 2px;
		background: var(--bg-inset);
		border: 1px solid var(--border-default);
		border-radius: var(--radius);
		padding: 2px;
		overflow: hidden;
	}

	.health-cell {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 10px 14px;
		background: var(--bg-card);
		border-radius: 2px;
		white-space: nowrap;
	}

	.health-cell.wide {
		flex: 1;
	}

	.health-label {
		display: flex;
		align-items: center;
		gap: 5px;
		font-size: 11px;
		color: var(--text-muted);
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	.health-value {
		font-size: 13px;
		font-weight: 600;
		color: var(--text-primary);
		margin-left: auto;
	}

	@media (max-width: 900px) {
		.health-grid {
			flex-wrap: wrap;
		}

		.health-cell.wide {
			flex: 1 1 100%;
			min-width: 0;
		}

		.health-cell {
			flex: 1 1 auto;
			padding: 8px 10px;
		}
	}

	@media (max-width: 480px) {
		.health-cell {
			padding: 6px 8px;
			gap: 5px;
		}
	}
</style>
