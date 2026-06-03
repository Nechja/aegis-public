// Domain → Graph model.
// The graph has two node types (machine nodes and workloads)
// and links that bind workloads to their host node.

export type GraphNodeKind = 'node' | 'workload';

export interface GraphNode {
	id: string;
	kind: GraphNodeKind;
	label: string;
	group: string;       // namespace for workloads, arch for nodes
	value: number;       // sizing hint: cpuCores for nodes, podCount for workloads
	cpu: number;         // current CPU millicores
	cpuCapacity: number; // total CPU capacity (nodes only, 0 for workloads)
	memory: number;
	memoryCapacity: number;
	meta: Record<string, string>; // extra info for detail panel
}

export interface GraphLink {
	source: string;
	target: string;
	value: number; // link weight (pod count)
}

export interface GraphData {
	nodes: GraphNode[];
	links: GraphLink[];
}
