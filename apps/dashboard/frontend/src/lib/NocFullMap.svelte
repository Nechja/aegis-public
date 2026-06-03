<script lang="ts">
	import type { NodeInfo, Workload, Service } from './types';
	import { buildNocNodes } from './noc/buildNoc';
	import { useSelection, findWorkload } from './noc/useNocState.svelte';
	import NocDetail from './noc/NocDetail.svelte';
	import { COLORS } from './graph/theme';
	import { iconPathFor } from './graph/icons';
	import { formatCPU, formatBytes, barColor } from './format';
	import type { GraphNode } from './graph/types';

	let { nodes, workloads, services, onClose }: {
		nodes: NodeInfo[];
		workloads: Workload[];
		services: Service[];
		onClose: () => void;
	} = $props();

	const selection = useSelection();
	const nocNodes = $derived(buildNocNodes(nodes, workloads, services));
	const detail = $derived(selection.selectedWl ? findWorkload(nocNodes, selection.selectedWl) : null);
	const namespaces = $derived([...new Set(workloads.map(w => w.namespace))].sort());

	function nsColor(ns: string): string {
		return COLORS.namespace.get(ns) ?? COLORS.fallback;
	}

	function getIcon(wl: { name: string; namespace: string; kind: string; pods: number; isService: boolean }): string {
		const fakeGN: GraphNode = {
			id: '', kind: 'workload', label: wl.name, group: wl.namespace,
			value: wl.pods, cpu: 0, cpuCapacity: 0, memory: 0, memoryCapacity: 0,
			meta: { kind: wl.kind, namespace: wl.namespace, pods: String(wl.pods),
				...(wl.isService ? { service: 'true' } : {}) },
		};
		return iconPathFor(fakeGN);
	}

	function cpuPct(node: NodeInfo): number {
		return node.cpuCores > 0 ? (node.cpuUsage / node.cpuCores) * 100 : 0;
	}

	function memPct(node: NodeInfo): number {
		return node.memoryBytes > 0 ? (node.memoryUsage / node.memoryBytes) * 100 : 0;
	}
</script>

<div class="map-overlay">
	<div class="map-chrome">
		<div class="map-title">
			<h2>aegis cluster map</h2>
			<span class="node-count">{nodes.length} nodes · {workloads.length} workloads · {services.length} services</span>
		</div>
		<div class="map-actions">
			<div class="ns-legend">
				{#each namespaces as ns}
					<button class="legend-item"
						class:active={selection.selectedNs === ns}
						style="--c: {nsColor(ns)}"
						onclick={() => selection.selectNs(ns)}>
						<span class="legend-dot"></span>{ns}
					</button>
				{/each}
			</div>
			<button class="close-btn" onclick={onClose}>✕</button>
		</div>
	</div>

	<div class="map-body">
		<!-- Services strip -->
		<div class="services-rail">
			<div class="rail-label">ingress</div>
			{#each services as svc}
				<a class="svc-badge" href={svc.url} target="_blank" rel="noopener"
					style="--c: {nsColor(svc.namespace)}">
					🔗 {svc.name}
				</a>
			{/each}
		</div>

		<!-- Node regions -->
		<div class="node-map">
			{#each nocNodes as node (node.id)}
				{@const n = nodes.find(x => x.name === node.id)}
				{@const pCpu = n ? cpuPct(n) : 0}
				{@const pMem = n ? memPct(n) : 0}
				{@const nodeColor = (n?.arch === 'arm64') ? 'var(--neon-pink)' : 'var(--neon-cyan)'}
				<div class="node-region" class:wide={node.workloads.length > 20} style="--nc: {nodeColor}">
					<div class="node-label">
						<span class="node-name">{node.id}</span>
						<span class="node-arch">{n?.arch}</span>
						<span class="node-roles">{n?.roles.join(' · ')}</span>
						<div class="node-bars">
							<div class="mini-bar">
								<span class="bar-label">cpu</span>
								<div class="bar-track"><div class="bar-fill" style="width:{pCpu}%; background:{barColor(pCpu)}"></div></div>
								<span class="bar-val">{pCpu.toFixed(0)}%</span>
							</div>
							<div class="mini-bar">
								<span class="bar-label">mem</span>
								<div class="bar-track"><div class="bar-fill" style="width:{pMem}%; background:{barColor(pMem)}"></div></div>
								<span class="bar-val">{pMem.toFixed(0)}%</span>
							</div>
						</div>
					</div>

					<div class="wl-grid">
						{#each node.workloads as wl (wl.id)}
							{@const dimmed = selection.selectedNs && wl.namespace !== selection.selectedNs}
							{@const selected = selection.selectedWl === wl.id}
							<button class="wl-dot" class:dimmed class:selected
								style="--c: {wl.color}"
								title="{wl.namespace}/{wl.label} · {wl.kind} · {wl.pods}p"
								onclick={() => selection.selectWl(wl.id)}>
								<svg viewBox="0 0 24 24" fill="currentColor"><path d={wl.iconPath} /></svg>
								{#if wl.isService}<span class="svc-ring"></span>{/if}
							</button>
						{/each}
					</div>
				</div>
			{/each}
		</div>
	</div>

	{#if detail}
		<div class="detail-pos">
			<NocDetail wl={detail} />
		</div>
	{/if}
</div>

<style>
	.map-overlay {
		position: fixed; inset: 0; z-index: 50;
		background: var(--bg-canvas);
		display: flex; flex-direction: column;
	}

	.map-chrome {
		display: flex; align-items: center; justify-content: space-between;
		padding: 6px 12px; border-bottom: 1px solid var(--border-default);
		background: var(--bg-card); flex-shrink: 0; gap: 12px;
	}

	.map-title { display: flex; align-items: baseline; gap: 8px; }
	h2 { font-size: 13px; font-weight: 700; color: var(--neon-pink); text-shadow: var(--glow-pink); text-transform: uppercase; letter-spacing: 1px; margin: 0; }
	.node-count { font-size: 9px; color: var(--text-muted); }

	.map-actions { display: flex; align-items: center; gap: 8px; }

	.ns-legend { display: flex; flex-wrap: wrap; gap: 2px; }
	.legend-item {
		font-family: var(--font); font-size: 7px; text-transform: uppercase;
		padding: 1px 4px; border: 1px solid transparent; border-radius: 2px;
		background: none; color: var(--c); cursor: pointer;
		display: flex; align-items: center; gap: 2px; opacity: 0.4;
		transition: all 0.1s;
	}
	.legend-item:hover { opacity: 0.7; }
	.legend-item.active { opacity: 1; border-color: var(--c); box-shadow: 0 0 4px var(--c); }
	.legend-dot { width: 4px; height: 4px; border-radius: 50%; background: var(--c); }

	.close-btn {
		font-family: var(--font); font-size: 14px; background: none;
		border: 1px solid var(--border-default); border-radius: 2px;
		color: var(--text-muted); cursor: pointer; padding: 1px 6px;
	}
	.close-btn:hover { border-color: var(--neon-rose); color: var(--neon-rose); }

	.map-body { flex: 1; overflow: auto; padding: 10px; display: flex; flex-direction: column; gap: 8px; }

	/* Services rail */
	.services-rail {
		display: flex; align-items: center; gap: 6px; flex-wrap: wrap;
		padding: 6px 10px; border: 1px solid var(--border-default);
		border-radius: var(--radius); background: var(--bg-card);
	}
	.rail-label { font-size: 8px; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.8px; }
	.svc-badge {
		font-family: var(--font); font-size: 8px; color: var(--c);
		border: 1px solid var(--c); border-radius: 2px; padding: 1px 6px;
		text-decoration: none; opacity: 0.6; transition: all 0.12s;
	}
	.svc-badge:hover { opacity: 1; box-shadow: 0 0 6px var(--c); }

	/* Node regions */
	.node-map {
		display: flex; flex-wrap: wrap; gap: 6px; flex: 1; align-content: start;
	}

	.node-region {
		flex: 1 1 280px; min-width: 240px;
		border: 1px solid color-mix(in srgb, var(--nc) 30%, transparent);
		border-radius: var(--radius); background: var(--bg-card);
		display: flex; flex-direction: column; overflow: hidden;
	}
	.node-region.wide { flex-basis: 100%; }

	.node-label {
		display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
		padding: 6px 10px; border-bottom: 1px solid var(--border-default);
	}
	.node-name { font-size: 13px; font-weight: 700; color: var(--nc); text-shadow: 0 0 6px var(--nc); }
	.node-arch { font-size: 8px; color: var(--nc); border: 1px solid var(--nc); padding: 0 4px; border-radius: 2px; text-transform: uppercase; }
	.node-roles { font-size: 8px; color: var(--text-muted); }
	.node-bars { margin-left: auto; display: flex; flex-direction: column; gap: 2px; }
	.mini-bar { display: flex; align-items: center; gap: 3px; }
	.bar-label { font-size: 7px; color: var(--text-muted); text-transform: uppercase; width: 18px; }
	.bar-track { width: 50px; height: 4px; background: var(--bg-inset); border-radius: 2px; overflow: hidden; }
	.bar-fill { height: 100%; border-radius: 2px; transition: width 0.4s ease; }
	.bar-val { font-size: 7px; color: var(--text-secondary); width: 20px; text-align: right; }

	/* Workload dots grid */
	.wl-grid {
		display: flex; flex-wrap: wrap; gap: 2px; padding: 6px 8px;
	}

	.wl-dot {
		width: 22px; height: 22px; border-radius: 3px;
		border: 1px solid color-mix(in srgb, var(--c) 30%, transparent);
		background: color-mix(in srgb, var(--c) 5%, transparent);
		color: var(--c); cursor: pointer;
		display: flex; align-items: center; justify-content: center;
		position: relative; transition: all 0.12s; padding: 0;
		font-family: var(--font);
	}
	.wl-dot svg { width: 14px; height: 14px; }
	.wl-dot:hover { border-color: var(--c); background: color-mix(in srgb, var(--c) 12%, transparent); box-shadow: 0 0 6px color-mix(in srgb, var(--c) 30%, transparent); }
	.wl-dot.selected { border-color: var(--c); background: color-mix(in srgb, var(--c) 15%, transparent); box-shadow: 0 0 8px var(--c); }
	.wl-dot.dimmed { opacity: 0.12; }

	.svc-ring {
		position: absolute; inset: -3px; border: 1px solid var(--c); border-radius: 5px; opacity: 0.4;
	}

	.detail-pos { position: fixed; bottom: 12px; right: 12px; z-index: 60; }

	@media (max-width: 900px) {
		.node-region { flex-basis: 100%; min-width: 0; }
		.ns-legend { display: none; }
	}
</style>
