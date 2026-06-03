<script lang="ts">
	import type { Service, NodeInfo, ClusterSummary, ClusterEvent, Workload } from './types';
	import { buildNocNodes } from './noc/buildNoc';
	import { useSelection, findWorkload } from './noc/useNocState.svelte';
	import NocNodeCard from './noc/NocNodeCard.svelte';
	import NocDetail from './noc/NocDetail.svelte';
	import ClusterHealth from './ClusterHealth.svelte';
	import EventLog from './EventLog.svelte';
	import ServiceCard from './ServiceCard.svelte';
	import { COLORS } from './graph/theme';
	import Panel from './Panel.svelte';

	let { nodes, workloads, services, cluster, events }: {
		nodes: NodeInfo[];
		workloads: Workload[];
		services: Service[];
		cluster: ClusterSummary | null;
		events: ClusterEvent[];
	} = $props();

	const selection = useSelection();
	let expandedNodes: Set<string> = $state(new Set());

	const nocNodes = $derived(buildNocNodes(nodes, workloads, services));

	const servicesByNs = $derived(() => {
		const map = new Map<string, typeof services>();
		for (const svc of services) {
			const list = map.get(svc.namespace) ?? [];
			list.push(svc);
			map.set(svc.namespace, list);
		}
		return [...map.entries()].sort(([a], [b]) => a.localeCompare(b));
	});
	const detail = $derived(selection.selectedWl ? findWorkload(nocNodes, selection.selectedWl) : null);

	function toggleExpand(name: string) {
		const next = new Set(expandedNodes);
		if (next.has(name)) next.delete(name); else next.add(name);
		expandedNodes = next;
	}
</script>

<div class="dash">
	<main class="main">
		{#if cluster}
			<ClusterHealth summary={cluster} />
		{/if}

		<div class="node-grid">
			{#each nocNodes as node (node.id)}
				{@const isExpanded = expandedNodes.has(node.id)}
				<div class="cell" class:expanded={isExpanded}>
					<NocNodeCard
						{node}
						expanded={isExpanded}
						selectedWl={selection.selectedWl}
						selectedNs={selection.selectedNs}
						onToggleExpand={() => toggleExpand(node.id)}
						onSelectWl={selection.selectWl}
					/>
				</div>
			{/each}
		</div>

		<Panel title="issues">
			<EventLog {events} />
		</Panel>
	</main>

	<aside class="sidebar">
		<Panel title="services ({services.length})">
			{#if services.length > 0}
				<div class="svc-groups">
					{#each servicesByNs() as [ns, svcs]}
						<div class="svc-ns" style="--ns-c: {COLORS.namespace.get(ns) ?? COLORS.fallback}">
							<div class="svc-ns-label">{ns}</div>
							<div class="svc-list">
								{#each svcs as service (service.name)}
									<ServiceCard {service} />
								{/each}
							</div>
						</div>
					{/each}
				</div>
			{:else}
				<span class="empty">no services</span>
			{/if}
		</Panel>
	</aside>

	{#if detail}
		<div class="detail-anchor">
			<NocDetail wl={detail} />
		</div>
	{/if}
</div>

<style>
	.dash {
		display: flex;
		gap: 12px;
		flex: 1;
		min-height: 0;
		overflow: hidden;
	}

	.main {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 10px;
		min-width: 0;
		overflow-y: auto;
	}

	.node-grid {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		align-items: stretch;
	}

	.cell {
		flex: 1 1 280px;
		min-width: 240px;
		display: flex;
	}

	.cell > :global(*) {
		flex: 1;
	}

	.cell.expanded {
		flex: 2 1 500px;
	}

	.sidebar {
		width: 220px;
		flex-shrink: 0;
		border-left: 1px solid var(--border-default);
		padding-left: 12px;
		overflow-y: auto;
	}

	.svc-groups { display: flex; flex-direction: column; gap: 4px; }

	.svc-ns {
		border-left: 2px solid var(--ns-c);
		padding-left: 6px;
	}

	.svc-ns-label {
		font-size: 7px; font-weight: 600; text-transform: uppercase;
		letter-spacing: 0.5px; color: var(--ns-c); padding: 1px 0;
	}

	.svc-list { display: flex; flex-direction: column; gap: 2px; }
	.empty { color: var(--text-muted); padding: 16px 0; font-size: 12px; }

	.detail-anchor {
		position: fixed;
		bottom: 14px;
		right: 14px;
		z-index: 20;
	}

	@media (max-width: 900px) {
		.dash { flex-direction: column; overflow: visible; }
		.main { overflow-y: visible; }
		.sidebar { width: 100%; border-left: none; border-top: 1px solid var(--border-default); padding-left: 0; padding-top: 12px; overflow-y: visible; }
		.cell { flex-basis: 100%; min-width: 0; }
	}
</style>
