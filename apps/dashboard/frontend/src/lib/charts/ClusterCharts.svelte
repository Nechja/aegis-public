<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import * as echarts from 'echarts';
	import type { NodeInfo, Workload, Service, NetPolicyRule } from '../types';
	import { COLORS, nsColor } from '../graph/theme';
	import { buildNocNodes } from '../noc/buildNoc';

	let { nodes, workloads, services, netPolicies = [], onClose }: {
		nodes: NodeInfo[];
		workloads: Workload[];
		services: Service[];
		netPolicies?: NetPolicyRule[];
		onClose: () => void;
	} = $props();

	let expandedPanel: string | null = $state(null);

	const charts: Record<string, { el: HTMLDivElement | null; instance: echarts.ECharts | null; init: () => void }> = {};

	const nocNodes = $derived(buildNocNodes(nodes, workloads, services));
	const namespaces = $derived([...new Set(workloads.map(w => w.namespace))].sort());


	function nodeColor(arch: string): string {
		return arch === 'arm64' ? '#D787D7' : '#00AFAF';
	}

	function dedupWorkloads() {
		const seen = new Map<string, { name: string; ns: string; kind: string; cpu: number; mem: number; pods: number; nodes: string[] }>();
		for (const node of nocNodes) {
			for (const wl of node.workloads) {
				const key = `${wl.namespace}/${wl.label}`;
				const existing = seen.get(key);
				if (existing) {
					existing.cpu += wl.cpu;
					existing.mem += wl.memory;
					existing.pods += wl.pods;
					if (!existing.nodes.includes(node.id)) existing.nodes.push(node.id);
				} else {
					seen.set(key, { name: wl.label, ns: wl.namespace, kind: wl.kind, cpu: wl.cpu, mem: wl.memory, pods: wl.pods, nodes: [node.id] });
				}
			}
		}
		return [...seen.values()];
	}

	function toggleExpand(id: string) {
		expandedPanel = expandedPanel === id ? null : id;
		// Delay resize to let CSS transition complete
		setTimeout(() => {
			Object.values(charts).forEach(c => c.instance?.resize());
		}, 250);
	}

	const baseTooltip = {
		backgroundColor: '#12121aee', borderColor: '#30363d',
		textStyle: { color: '#e6edf3', fontSize: 10, fontFamily: 'monospace' },
	};

	// ── CHORD: Node ↔ Namespace relationships ──
	function initChord() {
		const el = charts.chord.el;
		if (!el) return;
		const chart = echarts.init(el, undefined, { renderer: 'canvas' });
		charts.chord.instance = chart;

		// Build matrix: rows/cols = nodes + namespaces
		// Value = number of workload instances connecting that node to that namespace
		const nodeNames = nocNodes.map(n => n.label);
		const allNames = [...nodeNames, ...namespaces];
		const nameIdx = new Map<string, number>();
		allNames.forEach((name, i) => nameIdx.set(name, i));

		const matrix: number[][] = allNames.map(() => allNames.map(() => 0));

		for (const node of nocNodes) {
			const ni = nameIdx.get(node.label)!;
			for (const wl of node.workloads) {
				const nsi = nameIdx.get(wl.namespace);
				if (nsi !== undefined) {
					matrix[ni][nsi] += wl.pods;
					matrix[nsi][ni] += wl.pods;
				}
			}
		}

		const data = allNames.map((name, i) => {
			const isNode = i < nodeNames.length;
			const color = isNode ? nodeColor(nocNodes[i]?.arch ?? 'amd64') : nsColor(name);
			return {
				name,
				itemStyle: { color },
				label: {
					color,
					fontSize: isNode ? 11 : 9,
					fontWeight: isNode ? 'bold' as const : 'normal' as const,
					fontFamily: 'monospace',
				},
			};
		});

		const links: any[] = [];
		for (let i = 0; i < allNames.length; i++) {
			for (let j = i + 1; j < allNames.length; j++) {
				if (matrix[i][j] > 0) {
					const srcColor = i < nodeNames.length ? nodeColor(nocNodes[i]?.arch ?? 'amd64') : nsColor(allNames[i]);
					links.push({
						source: allNames[i],
						target: allNames[j],
						value: matrix[i][j],
						lineStyle: { color: srcColor, opacity: 0.3 },
					});
				}
			}
		}

		chart.setOption({
			backgroundColor: '#0a0a0f',
			tooltip: {
				...baseTooltip,
				trigger: 'item',
				formatter: (p: any) => {
					if (p.dataType === 'edge') {
						return `${p.data.source} ↔ ${p.data.target}<br/>${p.data.value} pods`;
					}
					return p.name;
				},
			},
			series: [{
				type: 'graph',
				layout: 'circular',
				circular: { rotateLabel: true },
				data,
				links,
				roam: true,
				label: { show: true, position: 'outside' },
				lineStyle: { curveness: 0.3 },
				emphasis: {
					focus: 'adjacency',
					lineStyle: { width: 3, opacity: 0.6 },
				},
				scaleLimit: { min: 0.5, max: 3 },
			}],
		});
	}

	// ── SUNBURST ──
	function initSunburst() {
		const el = charts.sunburst.el;
		if (!el) return;
		const chart = echarts.init(el, undefined, { renderer: 'canvas' });
		charts.sunburst.instance = chart;

		const data = nocNodes.map(node => ({
			name: node.label,
			itemStyle: { color: nodeColor(node.arch) },
			children: (() => {
				const byNs = new Map<string, any[]>();
				for (const wl of node.workloads) {
					const list = byNs.get(wl.namespace) ?? [];
					list.push({ name: wl.label, value: Math.max(wl.cpu, 1), itemStyle: { color: wl.color } });
					byNs.set(wl.namespace, list);
				}
				return [...byNs.entries()].map(([ns, children]) => ({
					name: ns, itemStyle: { color: nsColor(ns) }, children,
				}));
			})(),
		}));

		chart.setOption({
			backgroundColor: '#0a0a0f',
			tooltip: { ...baseTooltip, trigger: 'item' },
			series: [{ type: 'sunburst', data, radius: ['8%', '92%'],
				label: { color: '#e6edf3', fontSize: 8, fontFamily: 'monospace', minAngle: 3 },
				itemStyle: { borderColor: '#0a0a0f', borderWidth: 1 },
				levels: [{}, { label: { fontSize: 10, fontWeight: 'bold' }, itemStyle: { borderWidth: 2 } }, { label: { fontSize: 8 } }, { label: { show: false } }],
			}],
		});
	}

	// ── SANKEY: CPU — Arch → Node → Namespace → Kind → Workload ──
	function initSankey() {
		const el = charts.sankey.el;
		if (!el) return;
		const chart = echarts.init(el, undefined, { renderer: 'canvas' });
		charts.sankey.instance = chart;

		const { sNodes, sLinks } = buildResourceSankey('cpu');

		chart.setOption({
			backgroundColor: '#0a0a0f',
			tooltip: { ...baseTooltip, trigger: 'item',
				formatter: (p: any) => p.data?.source
					? `${p.data.source} → ${p.data.target}\n${p.data.value.toFixed(0)}m CPU`
					: p.name,
			},
			series: [{ type: 'sankey', data: sNodes, links: sLinks, layoutIterations: 48,
				emphasis: { focus: 'adjacency' },
				lineStyle: { color: 'gradient', opacity: 0.3 },
				label: { color: '#e6edf3', fontSize: 8, fontFamily: 'monospace' },
				itemStyle: { borderWidth: 0 },
				nodeGap: 6, nodeWidth: 12,
			}],
		});
	}

	function buildResourceSankey(metric: 'cpu' | 'mem') {
		const sNodes: any[] = [];
		const sLinks: any[] = [];
		const added = new Set<string>();
		const linkKeys = new Set<string>();

		function addN(id: string, color: string) {
			if (!added.has(id)) { sNodes.push({ name: id, itemStyle: { color } }); added.add(id); }
		}
		function addL(src: string, tgt: string, val: number) {
			const key = `${src}→${tgt}`;
			if (linkKeys.has(key)) {
				const link = sLinks.find(l => l.source === src && l.target === tgt);
				if (link) link.value += val;
				return;
			}
			linkKeys.add(key);
			sLinks.push({ source: src, target: tgt, value: val });
		}

		// Layer 1: Nodes
		for (const node of nocNodes) {
			const nId = `⬡ ${node.label}`;
			addN(nId, nodeColor(node.arch));
		}

		// Layer 2: Namespaces per node
		for (const node of nocNodes) {
			const nId = `⬡ ${node.label}`;
			const byNs = new Map<string, number>();
			for (const wl of node.workloads) {
				const val = metric === 'cpu' ? Math.max(wl.cpu, 1) : Math.max(wl.memory / (1024 * 1024), 1);
				byNs.set(wl.namespace, (byNs.get(wl.namespace) ?? 0) + val);
			}
			for (const [ns, val] of byNs) {
				const nsId = `ns: ${ns}`;
				addN(nsId, nsColor(ns));
				addL(nId, nsId, val);
			}
		}

		// Layer 4: Workload kinds per namespace
		const deduped = dedupWorkloads();
		const kindsByNs = new Map<string, Map<string, number>>();
		for (const wl of deduped) {
			const val = metric === 'cpu' ? Math.max(wl.cpu, 1) : Math.max(wl.mem / (1024 * 1024), 1);
			const nsKinds = kindsByNs.get(wl.ns) ?? new Map();
			nsKinds.set(wl.kind, (nsKinds.get(wl.kind) ?? 0) + val);
			kindsByNs.set(wl.ns, nsKinds);
		}

		for (const [ns, kinds] of kindsByNs) {
			for (const [kind, val] of kinds) {
				const kindId = `${ns}/${kind}`;
				addN(kindId, nsColor(ns));
				addL(`ns: ${ns}`, kindId, val);
			}
		}

		// Layer 5: Individual workloads (only significant ones)
		const threshold = metric === 'cpu' ? 5 : 20;
		for (const wl of deduped) {
			const val = metric === 'cpu' ? wl.cpu : wl.mem / (1024 * 1024);
			if (val < threshold) continue;
			const wlId = `wl: ${wl.name}`;
			addN(wlId, nsColor(wl.ns));
			addL(`${wl.ns}/${wl.kind}`, wlId, Math.max(val, 1));
		}

		return { sNodes, sLinks };
	}

	// ── TREEMAP ──
	function initTreemap() {
		const el = charts.treemap.el;
		if (!el) return;
		const chart = echarts.init(el, undefined, { renderer: 'canvas' });
		charts.treemap.instance = chart;

		const data = namespaces.map(ns => {
			const nsWls = dedupWorkloads().filter(wl => wl.ns === ns);
			return {
				name: ns, itemStyle: { color: nsColor(ns), borderColor: nsColor(ns) },
				children: nsWls.map(wl => ({ name: wl.name, value: Math.max(wl.mem / (1024 * 1024), 1), itemStyle: { color: nsColor(ns) } })),
			};
		}).filter(d => d.children.length > 0);

		chart.setOption({
			backgroundColor: '#0a0a0f',
			tooltip: { ...baseTooltip, trigger: 'item', formatter: (p: any) => `${p.name}<br/>${p.value?.toFixed(0) ?? 0} MB` },
			series: [{ type: 'treemap', data, roam: false,
				label: { color: '#e6edf3', fontSize: 9, fontFamily: 'monospace' },
				breadcrumb: { show: true, itemStyle: { color: '#12121a', borderColor: '#30363d', textStyle: { color: '#8b949e' } } },
				itemStyle: { borderColor: '#0a0a0f', borderWidth: 2, gapWidth: 1 },
				levels: [
					{ itemStyle: { borderWidth: 3 }, upperLabel: { show: true, color: '#e6edf3', fontSize: 11, fontWeight: 'bold', fontFamily: 'monospace' } },
					{ itemStyle: { borderWidth: 1 } },
				],
			}],
		});
	}

	// ── HEATMAP ──
	function initHeatmap() {
		const el = charts.heatmap.el;
		if (!el) return;
		const chart = echarts.init(el, undefined, { renderer: 'canvas' });
		charts.heatmap.instance = chart;

		const metrics = ['CPU %', 'MEM %', 'Workloads', 'Pods'];
		const nodeNames = nocNodes.map(n => n.label);

		const data: [number, number, number][] = [];
		nocNodes.forEach((node, ni) => {
			data.push([ni, 0, Math.round(node.cpuPct)]);
			data.push([ni, 1, Math.round(node.memPct)]);
			data.push([ni, 2, node.workloads.length]);
			data.push([ni, 3, node.workloads.reduce((s, w) => s + w.pods, 0)]);
		});

		chart.setOption({
			backgroundColor: '#0a0a0f',
			tooltip: baseTooltip,
			grid: { top: 10, bottom: 30, left: 70, right: 10 },
			xAxis: { type: 'category', data: nodeNames, axisLabel: { color: '#8b949e', fontSize: 10, fontFamily: 'monospace' }, axisLine: { lineStyle: { color: '#30363d' } } },
			yAxis: { type: 'category', data: metrics, axisLabel: { color: '#8b949e', fontSize: 9, fontFamily: 'monospace' }, axisLine: { lineStyle: { color: '#30363d' } } },
			visualMap: { min: 0, max: 100, show: false, inRange: { color: ['#0a0a0f', '#5FAF00', '#FF8700', '#AF005F'] } },
			series: [{ type: 'heatmap', data, label: { show: true, color: '#e6edf3', fontSize: 10, fontFamily: 'monospace' },
				itemStyle: { borderColor: '#0a0a0f', borderWidth: 2 },
			}],
		});
	}

	// Register charts
	// ── SANKEY: Memory flow ──
	// ── SANKEY: Ingress flow — full path through the cluster ──
	function initSankeyIngress() {
		const el = charts.sankey_ingress.el;
		if (!el) return;
		const chart = echarts.init(el, undefined, { renderer: 'canvas' });
		charts.sankey_ingress.instance = chart;

		const sNodes: any[] = [];
		const sLinks: any[] = [];
		const added = new Set<string>();
		const linkKeys = new Set<string>();

		function addNode(id: string, color: string) {
			if (added.has(id)) return;
			sNodes.push({ name: id, itemStyle: { color } });
			added.add(id);
		}

		function addLink(src: string, tgt: string, val: number) {
			const key = `${src}→${tgt}`;
			if (linkKeys.has(key)) return;
			linkKeys.add(key);
			sLinks.push({ source: src, target: tgt, value: val });
		}

		// Layer 1: Tailscale ingresses
		for (const svc of services) {
			addNode(`🔗 ${svc.name}`, '#00AFAF');
		}

		// Layer 2: Service workloads
		for (const svc of services) {
			const svcWl = `svc: ${svc.name}`;
			addNode(svcWl, nsColor(svc.namespace));
			addLink(`🔗 ${svc.name}`, svcWl, 1);
		}

		// Layer 3: Namespaces (group workloads by namespace)
		// Layer 4: Nodes
		const deduped = dedupWorkloads();
		const svcNames = new Set(services.map(s => s.name));

		// For each service, trace: service → namespace → co-located workloads → nodes
		for (const svc of services) {
			const nsId = `ns: ${svc.namespace}`;
			addNode(nsId, nsColor(svc.namespace));
			addLink(`svc: ${svc.name}`, nsId, 1);

			// All workloads in the same namespace as this service
			const nsWls = deduped.filter(wl => wl.ns === svc.namespace);
			for (const wl of nsWls) {
				const wlId = `wl: ${wl.name}`;
				addNode(wlId, nsColor(wl.ns));
				addLink(nsId, wlId, Math.max(wl.pods, 1));

				// Nodes this workload runs on
				for (const nodeName of wl.nodes) {
					const nodeId = `⬡ ${nodeName}`;
					addNode(nodeId, nodeColor(nocNodes.find(n => n.id === nodeName)?.arch ?? 'amd64'));
					addLink(wlId, nodeId, 1);
				}
			}
		}

		// Also show infrastructure dependencies — known data services
		const infraNs = ['redis', 'postgres', 'rabbitmq', 'minio', 'meilisearch'];
		for (const ins of infraNs) {
			const nsWls = deduped.filter(wl => wl.ns === ins);
			if (nsWls.length === 0) continue;

			const infraId = `infra: ${ins}`;
			addNode(infraId, nsColor(ins));

			// Link from workloads namespace to infrastructure
			for (const svc of services) {
				if (svc.namespace === 'workloads') {
					addLink(`ns: ${svc.namespace}`, infraId, 1);
				}
			}

			for (const wl of nsWls) {
				const wlId = `infra-wl: ${wl.name}`;
				addNode(wlId, nsColor(ins));
				addLink(infraId, wlId, Math.max(wl.pods, 1));

				for (const nodeName of wl.nodes) {
					const nodeId = `⬡ ${nodeName}`;
					addNode(nodeId, nodeColor(nocNodes.find(n => n.id === nodeName)?.arch ?? 'amd64'));
					addLink(wlId, nodeId, 1);
				}
			}
		}

		if (sNodes.length === 0) {
			chart.setOption({
				backgroundColor: '#0a0a0f',
				title: { text: 'No ingress services found', textStyle: { color: '#6e7681', fontSize: 12, fontFamily: 'monospace' }, left: 'center', top: 'center' },
			});
			return;
		}

		chart.setOption({
			backgroundColor: '#0a0a0f',
			tooltip: {
				...baseTooltip, trigger: 'item',
				formatter: (p: any) => p.data?.source ? `${p.data.source} → ${p.data.target}` : p.name,
			},
			series: [{ type: 'sankey', data: sNodes, links: sLinks, layoutIterations: 48,
				emphasis: { focus: 'adjacency' },
				lineStyle: { color: 'gradient', opacity: 0.35 },
				label: { color: '#e6edf3', fontSize: 9, fontFamily: 'monospace' },
				itemStyle: { borderWidth: 0 },
				nodeGap: 8, nodeWidth: 15,
			}],
		});
	}

	// ── SANKEY: Memory — same depth as CPU ──
	function initSankeyMem() {
		const el = charts.sankey_mem.el;
		if (!el) return;
		const chart = echarts.init(el, undefined, { renderer: 'canvas' });
		charts.sankey_mem.instance = chart;

		const { sNodes, sLinks } = buildResourceSankey('mem');

		chart.setOption({
			backgroundColor: '#0a0a0f',
			tooltip: { ...baseTooltip, trigger: 'item',
				formatter: (p: any) => p.data?.source
					? `${p.data.source} → ${p.data.target}\n${p.data.value.toFixed(0)} MB`
					: p.name,
			},
			series: [{ type: 'sankey', data: sNodes, links: sLinks, layoutIterations: 48,
				emphasis: { focus: 'adjacency' },
				lineStyle: { color: 'gradient', opacity: 0.3 },
				label: { color: '#e6edf3', fontSize: 8, fontFamily: 'monospace' },
				itemStyle: { borderWidth: 0 },
				nodeGap: 6, nodeWidth: 12,
			}],
		});
	}

	// ── SANKEY: Storage flow — fetches PVC data from Prometheus ──
	function initSankeyStorage() {
		const el = charts.sankey_storage.el;
		if (!el) return;
		const chart = echarts.init(el, undefined, { renderer: 'canvas' });
		charts.sankey_storage.instance = chart;

		const promURL = '/api/prom'; // proxy through dashboard? No — query direct
		// We'll build from workload data + known storage namespaces
		// Since PVC data isn't in our WebSocket state yet, show storage namespaces and their node placement

		const sNodes: any[] = [];
		const sLinks: any[] = [];
		const added = new Set<string>();
		const linkKeys = new Set<string>();

		function addN(id: string, color: string) {
			if (!added.has(id)) { sNodes.push({ name: id, itemStyle: { color } }); added.add(id); }
		}
		function addL(src: string, tgt: string, val: number) {
			const key = `${src}→${tgt}`;
			if (linkKeys.has(key)) {
				const link = sLinks.find(l => l.source === src && l.target === tgt);
				if (link) link.value += val;
				return;
			}
			linkKeys.add(key);
			sLinks.push({ source: src, target: tgt, value: val });
		}

		// Storage namespaces — services that use persistent storage
		const storageNs = ['postgres', 'redis', 'rabbitmq', 'minio', 'monitoring', 'meilisearch'];
		const deduped = dedupWorkloads();

		// Layer 1: Longhorn (storage provider)
		addN('longhorn', '#FFAF5F');

		// Layer 2: Storage namespaces
		for (const ns of storageNs) {
			const nsWls = deduped.filter(wl => wl.ns === ns);
			if (nsWls.length === 0) continue;

			addN(`ns: ${ns}`, nsColor(ns));
			const totalMem = nsWls.reduce((s, wl) => s + wl.mem, 0) / (1024 * 1024);
			addL('longhorn', `ns: ${ns}`, Math.max(totalMem, 1));

			// Layer 3: Workloads that likely use storage (StatefulSets, specific deployments)
			for (const wl of nsWls) {
				const isStateful = wl.kind === 'StatefulSet';
				const isStorage = isStateful || ['loki', 'prometheus', 'rabbitmq', 'minio', 'meilisearch'].some(s => wl.name.includes(s));
				if (!isStorage && wl.kind !== 'Deployment') continue;

				const wlId = `wl: ${wl.name}`;
				addN(wlId, nsColor(ns));
				addL(`ns: ${ns}`, wlId, Math.max(wl.mem / (1024 * 1024), 1));

				// Layer 4: Nodes
				for (const nodeName of wl.nodes) {
					const nodeId = `⬡ ${nodeName}`;
					addN(nodeId, nodeColor(nocNodes.find(n => n.id === nodeName)?.arch ?? 'amd64'));
					addL(wlId, nodeId, Math.max(wl.pods, 1));
				}
			}
		}

		// Also add longhorn-system workloads
		const lhWls = deduped.filter(wl => wl.ns === 'longhorn-system');
		if (lhWls.length > 0) {
			addN('ns: longhorn-system', nsColor('longhorn-system'));
			addL('longhorn', 'ns: longhorn-system', lhWls.reduce((s, wl) => s + wl.pods, 0));

			for (const wl of lhWls) {
				if (wl.kind !== 'DaemonSet' && !wl.name.includes('manager')) continue;
				const wlId = `lh: ${wl.name}`;
				addN(wlId, nsColor('longhorn-system'));
				addL('ns: longhorn-system', wlId, wl.pods);

				for (const nodeName of wl.nodes) {
					const nodeId = `⬡ ${nodeName}`;
					addN(nodeId, nodeColor(nocNodes.find(n => n.id === nodeName)?.arch ?? 'amd64'));
					addL(wlId, nodeId, 1);
				}
			}
		}

		chart.setOption({
			backgroundColor: '#0a0a0f',
			tooltip: { ...baseTooltip, trigger: 'item',
				formatter: (p: any) => p.data?.source ? `${p.data.source} → ${p.data.target}` : p.name,
			},
			series: [{ type: 'sankey', data: sNodes, links: sLinks, layoutIterations: 48,
				emphasis: { focus: 'adjacency' },
				lineStyle: { color: 'gradient', opacity: 0.35 },
				label: { color: '#e6edf3', fontSize: 9, fontFamily: 'monospace' },
				itemStyle: { borderWidth: 0 },
				nodeGap: 8, nodeWidth: 15,
			}],
		});
	}

	// ── SANKEY: Network Policy map ──
	function initNetPolMap() {
		const el = charts.netpol.el;
		if (!el) return;
		const chart = echarts.init(el, undefined, { renderer: 'canvas' });
		charts.netpol.instance = chart;

		const sNodes: any[] = [];
		const sLinks: any[] = [];
		const added = new Set<string>();
		const linkKeys = new Set<string>();

		function addN(id: string, color: string) {
			if (!added.has(id)) { sNodes.push({ name: id, itemStyle: { color } }); added.add(id); }
		}
		function addL(src: string, tgt: string, val: number, ports?: number[]) {
			const key = `${src}→${tgt}`;
			if (linkKeys.has(key)) return;
			linkKeys.add(key);
			const label = ports?.length ? ports.join(',') : '';
			sLinks.push({ source: src, target: tgt, value: val, lineStyle: { opacity: 0.4 }, label: label ? { show: true, formatter: label, fontSize: 7, color: '#6e7681' } : undefined });
		}

		if (!netPolicies || netPolicies.length === 0) {
			chart.setOption({
				backgroundColor: '#0a0a0f',
				title: { text: 'No network policy data yet', textStyle: { color: '#6e7681', fontSize: 12, fontFamily: 'monospace' }, left: 'center', top: 'center' },
			});
			return;
		}

		// Build: source namespace → [ALLOW] → target namespace (with ports)
		for (const rule of netPolicies) {
			const nsId = rule.namespace;
			addN(nsId, nsColor(nsId));

			for (const peer of rule.peers) {
				if (peer === '*') continue; // skip wildcard for readability
				const peerId = peer.includes('/') || peer.includes('.') ? peer : peer; // CIDR or namespace
				addN(peerId, peer.includes('.') ? '#6C6C6C' : nsColor(peerId));

				if (rule.direction === 'egress') {
					addL(nsId, peerId, 1, rule.ports);
				} else {
					addL(peerId, nsId, 1, rule.ports);
				}
			}
		}

		chart.setOption({
			backgroundColor: '#0a0a0f',
			tooltip: {
				...baseTooltip, trigger: 'item',
				formatter: (p: any) => {
					if (p.data?.source) {
						const ports = p.data.label?.formatter ?? 'all';
						return `${p.data.source} → ${p.data.target}<br/>ports: ${ports}`;
					}
					return p.name;
				},
			},
			series: [{ type: 'sankey', data: sNodes, links: sLinks, layoutIterations: 48,
				emphasis: { focus: 'adjacency' },
				lineStyle: { color: 'gradient', opacity: 0.35 },
				label: { color: '#e6edf3', fontSize: 10, fontFamily: 'monospace' },
				itemStyle: { borderWidth: 0 },
				nodeGap: 12, nodeWidth: 18,
			}],
		});
	}

	type PanelGroup = { label: string; panels: { id: string; title: string; init: () => void }[] };

	const groups: PanelGroup[] = [
		{ label: 'network', panels: [
			{ id: 'netpol', title: 'network policy map', init: initNetPolMap },
			{ id: 'sankey_ingress', title: 'ingress flow', init: initSankeyIngress },
		]},
		{ label: 'topology', panels: [
			{ id: 'chord', title: 'node ↔ namespace', init: initChord },
			{ id: 'sunburst', title: 'cluster hierarchy (cpu)', init: initSunburst },
		]},
		{ label: 'resources', panels: [
			{ id: 'sankey', title: 'cpu flow', init: initSankey },
			{ id: 'sankey_mem', title: 'memory flow', init: initSankeyMem },
			{ id: 'sankey_storage', title: 'storage flow', init: initSankeyStorage },
		]},
		{ label: 'utilization', panels: [
			{ id: 'treemap', title: 'memory treemap', init: initTreemap },
			{ id: 'heatmap', title: 'node utilization', init: initHeatmap },
		]},
	];

	const panels = groups.flatMap(g => g.panels);
	let activeGroup = $state('network');

	const chartEls: Record<string, HTMLDivElement> = {};

	function syncEls() {
		for (const p of panels) {
			if (!charts[p.id]) charts[p.id] = { el: null, instance: null, init: p.init };
			if (chartEls[p.id]) charts[p.id].el = chartEls[p.id];
		}
	}

	function resizeAll() {
		Object.values(charts).forEach(c => c.instance?.resize());
	}

	$effect(() => {
		if (nocNodes.length > 0) {
			setTimeout(() => {
				syncEls();
				panels.forEach(p => {
					if (charts[p.id]?.el && !charts[p.id].instance) {
						charts[p.id].init();
					}
				});
			}, 200);
		}
	});

	onMount(() => window.addEventListener('resize', resizeAll));
	onDestroy(() => {
		window.removeEventListener('resize', resizeAll);
		Object.values(charts).forEach(c => c.instance?.dispose());
	});
</script>

<div class="overlay">
	<div class="header">
		<h2>cluster analytics</h2>
		<div class="group-tabs">
			{#each groups as group}
				<button class="group-tab" class:active={activeGroup === group.label}
					onclick={() => { activeGroup = group.label; setTimeout(resizeAll, 200); }}>
					{group.label}
				</button>
			{/each}
		</div>
		<span class="sub">{nodes.length} nodes · {workloads.length} workloads</span>
		<button class="close" onclick={onClose}>✕</button>
	</div>

	<div class="body">
		{#each panels as panel}
			{@const isExpanded = expandedPanel === panel.id}
			{@const inGroup = groups.find(g => g.label === activeGroup)?.panels.some(p => p.id === panel.id)}
			{@const isHidden = (expandedPanel && expandedPanel !== panel.id) || (!expandedPanel && !inGroup)}
			<div class="panel" class:expanded={isExpanded} class:hidden={isHidden}>
				<div class="panel-head">
					<span class="panel-title">{panel.title}</span>
					<button class="expand-btn" onclick={() => toggleExpand(panel.id)}>
						{isExpanded ? '⊡' : '⊞'}
					</button>
				</div>
				<div class="chart" bind:this={chartEls[panel.id]}></div>
			</div>
		{/each}
	</div>
</div>

<style>
	.overlay {
		position: fixed; inset: 0; z-index: 50;
		background: var(--bg-canvas);
		display: flex; flex-direction: column;
	}

	.header {
		display: flex; align-items: center; gap: 10px;
		padding: 8px 14px; border-bottom: 1px solid var(--border-default);
		background: var(--bg-card); flex-shrink: 0;
	}

	h2 { font-size: 13px; font-weight: 700; color: var(--neon-pink); text-shadow: var(--glow-pink); text-transform: uppercase; letter-spacing: 1px; margin: 0; }
	.group-tabs { display: flex; gap: 3px; margin: 0 8px; }
	.group-tab {
		font-family: var(--font); font-size: 9px; text-transform: uppercase;
		letter-spacing: 0.8px; padding: 2px 10px;
		border: 1px solid var(--border-default); border-radius: 2px;
		background: transparent; color: var(--text-muted); cursor: pointer;
		transition: all 0.12s;
	}
	.group-tab:hover { border-color: var(--neon-pink); color: var(--neon-pink); }
	.group-tab.active {
		border-color: var(--neon-pink); color: var(--neon-pink);
		background: rgba(215, 135, 215, 0.08);
		text-shadow: 0 0 4px var(--neon-pink);
	}

	.sub { font-size: 9px; color: var(--text-muted); flex: 1; }

	.close {
		font-family: var(--font); font-size: 14px; background: none;
		border: 1px solid var(--border-default); border-radius: 2px;
		color: var(--text-muted); cursor: pointer; padding: 1px 6px;
	}
	.close:hover { border-color: var(--neon-rose); color: var(--neon-rose); }

	.body {
		flex: 1; overflow-y: auto; padding: 8px;
		display: flex; flex-wrap: wrap; gap: 6px;
		align-content: start;
	}

	.panel {
		flex: 1 1 45%;
		min-width: 400px;
		min-height: 350px;
		border: 1px solid var(--border-default);
		border-radius: var(--radius);
		background: var(--bg-card);
		display: flex; flex-direction: column;
		overflow: hidden;
		transition: all 0.2s ease;
	}

	.panel.expanded {
		position: fixed;
		inset: 48px 8px 8px 8px;
		z-index: 55;
		min-height: auto;
	}

	.panel.hidden {
		display: none;
	}

	.panel-head {
		display: flex; align-items: center; justify-content: space-between;
		padding: 5px 10px; border-bottom: 1px solid var(--border-default);
	}

	.panel-title {
		font-size: 9px; font-weight: 600; text-transform: uppercase;
		letter-spacing: 0.8px; color: var(--neon-orange);
		text-shadow: 0 0 4px var(--neon-orange);
	}

	.expand-btn {
		font-family: var(--font); font-size: 12px; background: none;
		border: none; color: var(--text-muted); cursor: pointer;
		padding: 0 4px; transition: color 0.12s;
	}
	.expand-btn:hover { color: var(--neon-pink); }

	.chart { flex: 1; min-height: 300px; }

	@media (max-width: 900px) {
		.panel { flex-basis: 100%; min-width: 0; min-height: 280px; }
	}
</style>
