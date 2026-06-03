import type { GraphNode } from './types';

export const COLORS = {
	node: { arm64: '#D787D7', amd64: '#00AFAF' } as Record<string, string>,
	namespace: new Map<string, string>([
		['argocd', '#D787D7'],
		['monitoring', '#5FAF00'],
		['workloads', '#00AFAF'],
		['linkerd', '#FF8700'],
		['linkerd-viz', '#FF8700'],
		['longhorn-system', '#FFAF5F'],
		['kube-system', '#6C6C6C'],
		['kyverno', '#AF5F87'],
		['cert-manager', '#6C6C6C'],
		['sealed-secrets', '#6C6C6C'],
		['crowdsec', '#AF005F'],
		['falco', '#AF005F'],
		['tailscale', '#00AFAF'],
		['keda', '#FFAF5F'],
		['trivy-system', '#AF005F'],
		['actions-runner-system', '#FFAF5F'],
		['minio', '#6C6C6C'],
		['redis-operator', '#6C6C6C'],
		['redis', '#AF5F87'],
		['postgres', '#58a6ff'],
		['cnpg-system', '#58a6ff'],
		['rabbitmq', '#FFAF5F'],
		['vms', '#D787D7'],
		['kubevirt', '#D787D7'],
		['meilisearch', '#FFAF5F'],
	]),
	// Auto-generate colors for unknown namespaces based on name hash
	autoColor(ns: string): string {
		const known = COLORS.namespace.get(ns);
		if (known) return known;
		// Generate a deterministic hue from the namespace name
		let hash = 0;
		for (let i = 0; i < ns.length; i++) hash = ns.charCodeAt(i) + ((hash << 5) - hash);
		const hue = Math.abs(hash) % 360;
		return `hsl(${hue}, 60%, 55%)`;
	},
	fallback: '#6C6C6C',
	link: '#30363d',
	particle: '#D787D7',
	bg: '#0a0a0f',
	grid: 0x1a1a2a,
	ambient: 0x222222,
};

export function colorFor(n: GraphNode): string {
	if (n.kind === 'node') return COLORS.node[n.group] ?? COLORS.node.amd64;
	return COLORS.autoColor(n.group);
}

export function nsColor(ns: string): string {
	return COLORS.autoColor(ns);
}

export function sizeFor(n: GraphNode): number {
	if (n.kind === 'node') return 6;
	return 2 + Math.min(n.value * 0.5, 3);
}

export function tooltip(n: GraphNode): string {
	const c = colorFor(n);
	const base = `font-family:monospace;color:#e6edf3;background:#0a0a0fee;border:1px solid ${c};border-radius:3px;box-shadow:0 0 15px ${c}60`;
	if (n.kind === 'node') {
		const cpuPct = n.cpuCapacity > 0 ? ((n.cpu / n.cpuCapacity) * 100).toFixed(0) : '0';
		return `<div style="${base};font-size:12px;padding:8px 12px">
			<strong style="color:${c}">${n.label}</strong> · ${n.meta.arch}<br/>
			<span style="color:#8b949e">cpu ${cpuPct}% · ${n.meta.roles}</span>
		</div>`;
	}
	return `<div style="${base};font-size:11px;padding:6px 10px">
		<strong>${n.label}</strong><br/>
		<span style="color:${c}">${n.meta.namespace}</span> · ${n.meta.kind}<br/>
		<span style="color:#8b949e">${n.meta.pods} pod${Number(n.meta.pods) !== 1 ? 's' : ''}</span>
	</div>`;
}
