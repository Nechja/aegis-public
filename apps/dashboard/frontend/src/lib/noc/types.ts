// NOC map data types

export interface NocNode {
	id: string;
	label: string;
	arch: string;
	roles: string[];
	gpu: string;
	workloadClass: string;
	ready: boolean;
	cpuPct: number;
	memPct: number;
	diskPct: number;
	loadAvg: number;
	workloads: NocWorkload[];
}

export interface NocWorkload {
	id: string;
	label: string;
	namespace: string;
	kind: string;
	nodeId: string;
	pods: number;
	cpu: number;
	memory: number;
	isService: boolean;
	color: string;
	iconPath: string;
}
