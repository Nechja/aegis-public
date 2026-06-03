// Transforms raw data into NOC view model

import type { NodeInfo, Workload, Service } from '../types';
import type { NocNode, NocWorkload } from './types';
import { COLORS } from '../graph/theme';
import { iconPathFor } from '../graph/icons';
import type { GraphNode } from '../graph/types';

export function buildNocNodes(
	nodes: NodeInfo[],
	workloads: Workload[],
	services: Service[],
): NocNode[] {
	const serviceSet = new Set(services.map(s => s.name));

	// Group & dedup workloads per node
	const wlByNode = new Map<string, Map<string, NocWorkload>>();
	for (const n of nodes) wlByNode.set(n.name, new Map());

	for (const w of workloads) {
		const map = wlByNode.get(w.node);
		if (!map) continue;
		const key = `${w.namespace}/${w.name}`;
		const existing = map.get(key);
		if (existing) {
			existing.pods += w.podCount;
			existing.cpu += w.cpu;
			existing.memory += w.memory;
		} else {
			const color = COLORS.namespace.get(w.namespace) ?? COLORS.fallback;
			const fakeGN: GraphNode = {
				id: `wl:${key}`, kind: 'workload', label: w.name,
				group: w.namespace, value: w.podCount,
				cpu: w.cpu, cpuCapacity: 0, memory: w.memory, memoryCapacity: 0,
				meta: { kind: w.kind, namespace: w.namespace, pods: String(w.podCount),
					...(serviceSet.has(w.name) ? { service: 'true' } : {}) },
			};
			map.set(key, {
				id: key, label: w.name, namespace: w.namespace, kind: w.kind,
				nodeId: w.node, pods: w.podCount, cpu: w.cpu, memory: w.memory,
				isService: serviceSet.has(w.name), color, iconPath: iconPathFor(fakeGN),
			});
		}
	}

	// Sort: smallest first, biggest last (last gets full-width span in CSS grid)
	const sorted = [...nodes].sort((a, b) => {
		const aCount = wlByNode.get(a.name)?.size ?? 0;
		const bCount = wlByNode.get(b.name)?.size ?? 0;
		return aCount - bCount;
	});

	return sorted.map(n => ({
		id: n.name,
		label: n.name,
		arch: n.arch,
		roles: n.roles,
		gpu: n.gpu ?? '',
		workloadClass: n.workloadClass ?? '',
		ready: n.ready ?? true,
		cpuPct: n.cpuCores > 0 ? (n.cpuUsage / n.cpuCores) * 100 : 0,
		memPct: n.memoryBytes > 0 ? (n.memoryUsage / n.memoryBytes) * 100 : 0,
		diskPct: n.diskBytes > 0 ? (n.diskUsage / n.diskBytes) * 100 : 0,
		loadAvg: n.loadAvg ?? 0,
		workloads: [...(wlByNode.get(n.name)?.values() ?? [])],
	}));
}

