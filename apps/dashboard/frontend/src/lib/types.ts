export interface Service {
	name: string;
	namespace: string;
	url: string;
	hostname: string;
}

export interface TopPod {
	name: string;
	namespace: string;
	cpu: number;
	memory: number;
}

export interface NodeInfo {
	name: string;
	arch: string;
	os: string;
	roles: string[];
	ready: boolean;
	unschedulable: boolean;
	gpu: string;
	workloadClass: string;
	labels: Record<string, string>;
	cpuCores: number;
	memoryBytes: number;
	cpuUsage: number;
	memoryUsage: number;
	diskBytes: number;
	diskUsage: number;
	loadAvg: number;
	topPods: TopPod[];
}

export interface PodMetric {
	name: string;
	namespace: string;
	cpuUsage: number;
	memoryUsage: number;
}

export interface ClusterEvent {
	timestamp: string;
	type: string;
	reason: string;
	object: string;
	namespace: string;
	message: string;
}

export interface Workload {
	name: string;
	kind: string;
	namespace: string;
	node: string;
	podCount: number;
	cpu: number;
	memory: number;
}

export interface NetPolicyRule {
	namespace: string;
	name: string;
	direction: string;
	peers: string[];
	ports: number[];
}

export interface DashboardState {
	services: Service[];
	nodes: NodeInfo[];
	cluster: ClusterSummary;
	events: ClusterEvent[];
	workloads: Workload[];
	netPolicies: NetPolicyRule[];
}

export interface ClusterSummary {
	totalNodes: number;
	readyNodes: number;
	totalPods: number;
	totalCPU: number;
	totalMemory: number;
	usedCPU: number;
	usedMemory: number;
	services: number;
}
