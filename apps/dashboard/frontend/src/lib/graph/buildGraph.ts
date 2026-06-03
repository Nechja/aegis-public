import type { NodeInfo, Workload, Service } from '../types';
import type { GraphData, GraphNode, GraphLink } from './types';

export function buildGraph(
	nodes: NodeInfo[],
	workloads: Workload[],
	services: Service[],
): GraphData {
	const graphNodes: GraphNode[] = [];
	const graphLinks: GraphLink[] = [];
	const serviceSet = new Set(services.map(s => s.name));

	// Machine nodes
	for (const n of nodes) {
		graphNodes.push({
			id: `node:${n.name}`,
			kind: 'node',
			label: n.name,
			group: n.arch,
			value: n.cpuCores / 1000, // cores
			cpu: n.cpuUsage,
			cpuCapacity: n.cpuCores,
			memory: n.memoryUsage,
			memoryCapacity: n.memoryBytes,
			meta: {
				arch: n.arch,
				roles: n.roles.join(', '),
				ready: n.ready ? 'yes' : 'no',
			},
		});
	}

	// Deduplicate workloads: same name+namespace+kind across nodes → one graph node per unique workload
	// but keep links to each node they run on
	const wlMap = new Map<string, { wl: Workload; nodes: Map<string, number>; totalCpu: number; totalMem: number; totalPods: number }>();

	for (const w of workloads) {
		const key = `${w.namespace}/${w.name}`;
		let entry = wlMap.get(key);
		if (!entry) {
			entry = { wl: w, nodes: new Map(), totalCpu: 0, totalMem: 0, totalPods: 0 };
			wlMap.set(key, entry);
		}
		const existing = entry.nodes.get(w.node) ?? 0;
		entry.nodes.set(w.node, existing + w.podCount);
		entry.totalCpu += w.cpu;
		entry.totalMem += w.memory;
		entry.totalPods += w.podCount;
	}

	for (const [key, entry] of wlMap) {
		const { wl } = entry;
		const hasService = serviceSet.has(wl.name);

		graphNodes.push({
			id: `wl:${key}`,
			kind: 'workload',
			label: wl.name,
			group: wl.namespace,
			value: entry.totalPods,
			cpu: entry.totalCpu,
			cpuCapacity: 0,
			memory: entry.totalMem,
			memoryCapacity: 0,
			meta: {
				kind: wl.kind,
				namespace: wl.namespace,
				pods: String(entry.totalPods),
				...(hasService ? { service: 'true' } : {}),
			},
		});

		// Link workload to each node it runs on
		for (const [nodeName, podCount] of entry.nodes) {
			graphLinks.push({
				source: `wl:${key}`,
				target: `node:${nodeName}`,
				value: podCount,
			});
		}
	}

	return { nodes: graphNodes, links: graphLinks };
}
