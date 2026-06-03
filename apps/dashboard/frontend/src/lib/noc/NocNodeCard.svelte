<script lang="ts">
	import { mdiChip, mdiExpansionCard, mdiExpansionCardVariant, mdiCpu64Bit, mdiMemory, mdiHarddisk, mdiChevronDown, mdiChevronUp } from '@mdi/js';
	import Icon from '../Icon.svelte';
	import BlockBar from '../BlockBar.svelte';
	import StatusDot from '../StatusDot.svelte';
	import NeonBadge from '../NeonBadge.svelte';
	import { formatCPU, formatBytes } from '../format';
	import { COLORS } from '../graph/theme';
	import type { NocNode, NocWorkload } from './types';

	let { node, expanded = false, selectedWl = null, selectedNs = null, onToggleExpand, onSelectWl }: {
		node: NocNode;
		expanded?: boolean;
		selectedWl?: string | null;
		selectedNs?: string | null;
		onToggleExpand?: () => void;
		onSelectWl?: (id: string) => void;
	} = $props();

	const nodeColor = node.arch === 'arm64' ? 'var(--neon-pink)' : 'var(--neon-cyan)';
	const archIcon = $derived(node.arch === 'arm64' ? mdiChip : mdiExpansionCard);

	// Top 4 workloads by CPU for collapsed view
	const topWls = $derived(
		[...node.workloads].sort((a, b) => b.cpu - a.cpu).slice(0, 4)
	);

	// Group by namespace for expanded view
	const groups = $derived(() => {
		const map = new Map<string, NocWorkload[]>();
		for (const wl of node.workloads) {
			(map.get(wl.namespace) ?? (map.set(wl.namespace, []), map.get(wl.namespace)!)).push(wl);
		}
		return [...map.entries()]
			.sort(([a], [b]) => a.localeCompare(b))
			.map(([ns, wls]) => ({
				ns,
				color: COLORS.namespace.get(ns) ?? COLORS.fallback,
				wls: wls.sort((a, b) => b.cpu - a.cpu),
			}));
	});

	function isDimmed(ns: string): boolean {
		if (!selectedNs && !selectedWl) return false;
		if (selectedNs) return ns !== selectedNs;
		return false;
	}
</script>

<div class="card" class:expanded style="--node-c: {nodeColor}">
	<button class="header" onclick={onToggleExpand}>
		<StatusDot color={node.cpuPct < 90 ? 'var(--neon-green)' : 'var(--neon-rose)'} />
		<h3>{node.label}</h3>
		<NeonBadge color={nodeColor}><Icon path={archIcon} size={12} /> {node.arch}</NeonBadge>
		{#if node.gpu}
			<NeonBadge color="var(--neon-green)"><Icon path={mdiExpansionCardVariant} size={12} /> {node.gpu === 'igpu' ? 'iGPU' : 'GPU'}</NeonBadge>
		{/if}
		{#if node.workloadClass}
			<NeonBadge color="var(--neon-peach)" glow={false}>{node.workloadClass}</NeonBadge>
		{/if}
		{#each node.roles as role}
			<NeonBadge color="var(--text-muted)" glow={false}>{role}</NeonBadge>
		{/each}
		{#if !node.ready}
			<NeonBadge color="var(--neon-rose)">not ready</NeonBadge>
		{/if}
		<span class="wl-count">{node.workloads.length}</span>
		{#if onToggleExpand}
			<span class="chevron"><Icon path={expanded ? mdiChevronUp : mdiChevronDown} size={16} /></span>
		{/if}
	</button>

	<div class="meters">
		<div class="meter-row">
			<Icon path={mdiCpu64Bit} size={12} color="var(--text-muted)" />
			<span class="m-label">cpu</span>
			<BlockBar pct={node.cpuPct} />
			<span class="m-val">{node.cpuPct.toFixed(0)}%</span>
		</div>
		<div class="meter-row">
			<Icon path={mdiMemory} size={12} color="var(--text-muted)" />
			<span class="m-label">mem</span>
			<BlockBar pct={node.memPct} />
			<span class="m-val">{node.memPct.toFixed(0)}%</span>
		</div>
		{#if node.diskPct > 0}
			<div class="meter-row">
				<Icon path={mdiHarddisk} size={12} color="var(--text-muted)" />
				<span class="m-label">disk</span>
				<BlockBar pct={node.diskPct} />
				<span class="m-val">{node.diskPct.toFixed(0)}%</span>
			</div>
		{/if}
		{#if node.loadAvg > 0}
			<div class="meter-row">
				<span class="m-icon-text">⚡</span>
				<span class="m-label">load</span>
				<span class="m-val-wide">{node.loadAvg.toFixed(2)}</span>
			</div>
		{/if}
	</div>

	{#if !expanded && topWls.length > 0}
		<div class="top-section">
			<span class="top-label">top</span>
			<div class="top-rows">
				{#each topWls as wl}
					<div class="top-row">
						<svg class="row-icon" viewBox="0 0 24 24" fill={wl.color}><path d={wl.iconPath} /></svg>
						<span class="row-name">{wl.label}</span>
						<span class="row-cpu">{formatCPU(wl.cpu)}</span>
					</div>
				{/each}
			</div>
		</div>
	{/if}

	{#if expanded}
		<div class="wl-list">
			{#each groups() as group (group.ns)}
				<div class="ns-section" class:dimmed={isDimmed(group.ns)} style="--ns-c: {group.color}">
					<div class="ns-label">{group.ns}</div>
					{#each group.wls as wl (wl.id)}
						<button class="wl-row" class:selected={selectedWl === wl.id} onclick={() => onSelectWl?.(wl.id)}>
							<svg class="row-icon" viewBox="0 0 24 24" fill={group.color}><path d={wl.iconPath} /></svg>
							<span class="row-name">{wl.label}</span>
							<span class="row-kind">{wl.kind}</span>
							<span class="row-pods">{wl.pods}p</span>
							<span class="row-cpu">{formatCPU(wl.cpu)}</span>
							<span class="row-mem">{formatBytes(wl.memory)}</span>
							{#if wl.isService}<span class="row-svc">svc</span>{/if}
						</button>
					{/each}
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.card {
		border: 1px solid color-mix(in srgb, var(--node-c) 25%, transparent);
		border-radius: var(--radius);
		background: var(--bg-card);
		display: flex;
		flex-direction: column;
		gap: 5px;
		padding: 10px 12px;
		transition: border-color 0.15s ease, box-shadow 0.15s ease;
		overflow: hidden;
		min-width: 0;
	}

	.card:hover { border-color: color-mix(in srgb, var(--node-c) 50%, transparent); }
	.card.expanded {
		border-color: var(--node-c);
		box-shadow: 0 0 8px color-mix(in srgb, var(--node-c) 12%, transparent);
	}

	/* Header */
	.header {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
		background: none; border: none; cursor: pointer;
		padding: 0; font-family: var(--font); text-align: left; width: 100%;
	}

	h3 { font-size: 13px; font-weight: 700; color: var(--node-c); text-shadow: 0 0 6px var(--node-c); margin: 0; }

	.wl-count {
		font-size: 9px; color: var(--text-muted);
		background: var(--bg-inset); padding: 0 5px; border-radius: 8px;
	}

	.chevron { margin-left: auto; color: var(--text-muted); transition: color 0.12s; }
	.header:hover .chevron { color: var(--node-c); }

	/* Meters */
	.meters { display: flex; flex-direction: column; gap: 2px; }
	.meter-row { display: flex; align-items: center; gap: 4px; }
	.m-label { font-size: 8px; color: var(--text-muted); text-transform: uppercase; width: 22px; }
	.m-val { font-size: 9px; color: var(--text-secondary); min-width: 22px; text-align: right; }
	.m-val-wide { font-size: 9px; color: var(--text-secondary); flex: 1; }
	.m-icon-text { font-size: 10px; width: 12px; text-align: center; }

	/* Top pods (collapsed) */
	.top-section {
		display: flex; gap: 6px;
		padding-top: 5px; border-top: 1px solid var(--border-default);
		overflow: hidden; min-width: 0;
	}

	.top-label {
		font-size: 9px; color: var(--text-muted); text-transform: uppercase;
		width: 22px; flex-shrink: 0; padding-top: 1px;
	}

	.top-rows { flex: 1; display: flex; flex-direction: column; gap: 2px; min-width: 0; overflow: hidden; }

	.top-row {
		display: flex; align-items: center; gap: 5px; font-size: 10px;
		min-width: 0;
	}

	/* Shared row styles */
	.row-icon { width: 11px; height: 11px; flex-shrink: 0; }
	.row-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-primary); font-size: 10px; }
	.row-kind { font-size: 8px; color: var(--text-muted); width: 55px; flex-shrink: 0; }
	.row-pods { font-size: 9px; color: var(--text-muted); width: 18px; text-align: right; flex-shrink: 0; }
	.row-cpu { font-size: 9px; color: var(--text-secondary); width: 34px; text-align: right; flex-shrink: 0; }
	.row-mem { font-size: 9px; color: var(--text-secondary); width: 34px; text-align: right; flex-shrink: 0; }
	.row-svc { font-size: 7px; color: var(--neon-cyan); border: 1px solid var(--neon-cyan); padding: 0 3px; border-radius: 2px; flex-shrink: 0; }

	/* Expanded list */
	.wl-list {
		display: flex; flex-direction: column; gap: 1px;
		padding-top: 5px; border-top: 1px solid var(--border-default);
		max-height: 400px; overflow-y: auto;
	}

	.ns-section {
		border-left: 2px solid var(--ns-c); margin-bottom: 1px;
		transition: opacity 0.15s;
	}

	.ns-section.dimmed { opacity: 0.15; }

	.ns-label {
		font-size: 7px; font-weight: 600; text-transform: uppercase;
		letter-spacing: 0.5px; color: var(--ns-c);
		padding: 1px 6px; background: color-mix(in srgb, var(--ns-c) 5%, transparent);
	}

	.wl-row {
		display: flex; align-items: center; gap: 5px;
		padding: 2px 6px; background: none; border: none;
		cursor: pointer; font-family: var(--font);
		width: 100%; text-align: left; transition: background 0.1s; border-radius: 1px;
	}

	.wl-row:hover { background: rgba(215, 135, 215, 0.04); }
	.wl-row.selected { background: rgba(215, 135, 215, 0.08); }
	.wl-row .row-name { font-weight: 500; }
</style>
