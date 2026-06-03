// Client-side Mermaid diagram generation from live dashboard state

import type { NodeInfo, Workload, Service } from '../types';

const NS_CLASSES: Record<string, string> = {
	argocd: 'argocd', monitoring: 'monitoring', workloads: 'workloads',
	linkerd: 'linkerd', 'linkerd-viz': 'linkerd_viz',
	'longhorn-system': 'longhorn_system', 'kube-system': 'kube_system',
	falco: 'falco', crowdsec: 'crowdsec', kyverno: 'kyverno',
	'cert-manager': 'cert_manager', 'sealed-secrets': 'sealed_secrets',
	tailscale: 'tailscale', keda: 'keda', 'trivy-system': 'trivy_system',
	redis: 'redis', postgres: 'postgres', 'cnpg-system': 'cnpg_system',
	rabbitmq: 'rabbitmq', minio: 'minio',
	'actions-runner-system': 'actions_runner_system',
};

function san(s: string): string {
	return s.replace(/[-./\s]/g, '_');
}

function cls(ns: string): string {
	return NS_CLASSES[ns] ?? 'default_ns';
}

function styles(): string {
	return `  classDef node_hw fill:#161b22,stroke:#00AFAF,color:#e6edf3,stroke-width:2px
  classDef svc_ext fill:#0a0a0f,stroke:#00AFAF,color:#00AFAF,stroke-dasharray:5 5
  classDef argocd fill:#1a1025,stroke:#D787D7,color:#D787D7
  classDef monitoring fill:#1a2510,stroke:#5FAF00,color:#5FAF00
  classDef workloads fill:#102525,stroke:#00AFAF,color:#00AFAF
  classDef linkerd fill:#251a10,stroke:#FF8700,color:#FF8700
  classDef linkerd_viz fill:#251a10,stroke:#FF8700,color:#FF8700
  classDef longhorn_system fill:#252010,stroke:#FFAF5F,color:#FFAF5F
  classDef kube_system fill:#1a1a1a,stroke:#6C6C6C,color:#8b949e
  classDef falco fill:#250a15,stroke:#AF005F,color:#AF005F
  classDef crowdsec fill:#250a15,stroke:#AF005F,color:#AF005F
  classDef kyverno fill:#1a1525,stroke:#AF5F87,color:#AF5F87
  classDef cert_manager fill:#1a1a1a,stroke:#6C6C6C,color:#8b949e
  classDef sealed_secrets fill:#1a1a1a,stroke:#6C6C6C,color:#8b949e
  classDef tailscale fill:#102525,stroke:#00AFAF,color:#00AFAF
  classDef keda fill:#252010,stroke:#FFAF5F,color:#FFAF5F
  classDef trivy_system fill:#250a15,stroke:#AF005F,color:#AF005F
  classDef redis fill:#251520,stroke:#AF5F87,color:#AF5F87
  classDef postgres fill:#102030,stroke:#58a6ff,color:#58a6ff
  classDef cnpg_system fill:#102030,stroke:#58a6ff,color:#58a6ff
  classDef rabbitmq fill:#252010,stroke:#FFAF5F,color:#FFAF5F
  classDef minio fill:#252010,stroke:#FFAF5F,color:#FFAF5F
  classDef actions_runner_system fill:#252010,stroke:#FFAF5F,color:#FFAF5F
  classDef default_ns fill:#161b22,stroke:#6C6C6C,color:#8b949e
`;
}

export function generateOverview(
	nodes: NodeInfo[],
	workloads: Workload[],
	services: Service[],
): string {
	const seen = new Set<string>();
	const nsCounts: Record<string, number> = {};
	for (const wl of workloads) {
		const key = `${wl.namespace}/${wl.name}`;
		if (!seen.has(key)) {
			seen.add(key);
			nsCounts[wl.namespace] = (nsCounts[wl.namespace] ?? 0) + 1;
		}
	}

	const nsList = Object.keys(nsCounts).sort();
	let out = 'graph TB\n\n' + styles();

	out += '  subgraph ingress["Tailscale Ingress"]\n    direction LR\n';
	for (const svc of services) {
		out += `    ts_${san(svc.name)}>"\u{1F517} ${svc.name}"]:::svc_ext\n`;
	}
	out += '  end\n\n';

	out += '  subgraph hw["Nodes"]\n    direction LR\n';
	for (const node of nodes) {
		out += `    ${san(node.name)}{{"🖥 ${node.name}\\n${node.arch}"}}:::node_hw\n`;
	}
	out += '  end\n\n';

	// Group namespaces by category for a cleaner layout
	const categories: Record<string, string[]> = {
		'Apps': [],
		'Platform': [],
		'Security': [],
		'Storage & Data': [],
	};

	const catMap: Record<string, string> = {
		workloads: 'Apps', 'actions-runner-system': 'Apps',
		argocd: 'Platform', 'kube-system': 'Platform', keda: 'Platform',
		tailscale: 'Platform', 'cert-manager': 'Platform', 'sealed-secrets': 'Platform',
		monitoring: 'Platform', linkerd: 'Platform', 'linkerd-viz': 'Platform',
		falco: 'Security', crowdsec: 'Security', kyverno: 'Security', 'trivy-system': 'Security',
		'longhorn-system': 'Storage & Data', redis: 'Storage & Data',
		postgres: 'Storage & Data', 'cnpg-system': 'Storage & Data',
		rabbitmq: 'Storage & Data', minio: 'Storage & Data',
		'redis-operator': 'Storage & Data',
	};

	for (const ns of nsList) {
		const cat = catMap[ns] ?? 'Platform';
		categories[cat].push(ns);
	}

	for (const [cat, nsNames] of Object.entries(categories)) {
		if (nsNames.length === 0) continue;
		const catID = san(cat);
		out += `  subgraph ${catID}["${cat}"]\n    direction LR\n`;
		for (const ns of nsNames) {
			out += `    ${san(ns)}["${ns}\\n${nsCounts[ns]} workloads"]:::${cls(ns)}\n`;
		}
		out += '  end\n\n';
	}

	for (const svc of services) {
		out += `  ts_${san(svc.name)} --> ${san(svc.namespace)}\n`;
	}

	return out;
}

export function generateNamespace(
	ns: string,
	nodes: NodeInfo[],
	workloads: Workload[],
	services: Service[],
): string {
	const serviceSet = new Set(services.map(s => s.name));

	// Dedup workloads, track nodes
	const wlMap = new Map<string, { name: string; kind: string; nodes: string[] }>();
	for (const wl of workloads) {
		if (wl.namespace !== ns) continue;
		const existing = wlMap.get(wl.name);
		if (existing) {
			if (!existing.nodes.includes(wl.node)) existing.nodes.push(wl.node);
		} else {
			wlMap.set(wl.name, { name: wl.name, kind: wl.kind, nodes: [wl.node] });
		}
	}

	const wlList = [...wlMap.values()].sort((a, b) => a.name.localeCompare(b.name));
	const nsID = san(ns);
	const c = cls(ns);

	let out = 'graph TB\n\n' + styles();

	out += `  subgraph ${nsID}["${ns}"]\n`;
	for (const wl of wlList) {
		const wlID = `${nsID}_${san(wl.name)}`;
		const label = wl.kind !== 'Deployment' ? `${wl.name}\\n(${wl.kind})` : wl.name;
		out += serviceSet.has(wl.name)
			? `    ${wlID}(["${label}"]):::${c}\n`
			: `    ${wlID}["${label}"]:::${c}\n`;
	}
	out += '  end\n\n';

	const activeNodes = new Set(wlList.flatMap(wl => wl.nodes));
	out += '  subgraph hw["Nodes"]\n    direction LR\n';
	for (const node of nodes) {
		if (!activeNodes.has(node.name)) continue;
		out += `    ${san(node.name)}{{"🖥 ${node.name}"}}:::node_hw\n`;
	}
	out += '  end\n\n';

	for (const wl of wlList) {
		const wlID = `${nsID}_${san(wl.name)}`;
		for (const nodeName of wl.nodes) {
			out += `  ${wlID} -.-> ${san(nodeName)}\n`;
		}
	}

	out += '\n';
	for (const wl of wlList) {
		const svc = services.find(s => s.name === wl.name);
		if (svc) {
			out += `  ts_${san(svc.name)}>"\u{1F517} ${svc.hostname}"]:::svc_ext --> ${nsID}_${san(wl.name)}\n`;
		}
	}

	return out;
}
