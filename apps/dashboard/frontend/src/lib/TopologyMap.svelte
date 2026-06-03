<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import * as echarts from 'echarts';
	import type { NodeInfo, Workload, Service } from './types';
	import { buildNocNodes } from './noc/buildNoc';
	import { COLORS } from './graph/theme';
	import type { NocNode } from './noc/types';

	let { nodes, workloads, services }: {
		nodes: NodeInfo[];
		workloads: Workload[];
		services: Service[];
	} = $props();

	let container: HTMLDivElement;
	let chart: echarts.ECharts | null = null;

	const nocNodes = $derived(buildNocNodes(nodes, workloads, services));
	const namespaces = $derived([...new Set(workloads.map(w => w.namespace))].sort());

	// Build ECharts graph data from our domain model
	function buildChartData() {
		const categories = [
			{ name: 'node' },
			...namespaces.map(ns => ({ name: ns })),
		];

		const catIndex = new Map<string, number>();
		catIndex.set('node', 0);
		namespaces.forEach((ns, i) => catIndex.set(ns, i + 1));

		const chartNodes: any[] = [];
		const chartLinks: any[] = [];

		// Add machine nodes
		for (const node of nocNodes) {
			const nc = node.arch === 'arm64' ? '#D787D7' : '#00AFAF';
			const pCpu = node.cpuPct;

			chartNodes.push({
				id: `node:${node.id}`,
				name: node.label,
				category: 0,
				symbolSize: 40 + Math.sqrt(node.workloads.length) * 8,
				itemStyle: {
					color: `${nc}30`,
					borderColor: nc,
					borderWidth: 2,
					shadowColor: nc,
					shadowBlur: 10,
				},
				label: {
					show: true,
					color: nc,
					fontSize: 13,
					fontWeight: 'bold',
					fontFamily: 'monospace',
					position: 'top',
					distance: 8,
					formatter: `{name|${node.label}}\n{sub|${node.arch} · ${pCpu.toFixed(0)}%${node.gpu ? ' · GPU' : ''}}`,
					rich: {
						name: { color: nc, fontSize: 13, fontWeight: 'bold', fontFamily: 'monospace' },
						sub: { color: `${nc}99`, fontSize: 9, fontFamily: 'monospace' },
					},
				},
				tooltip: {
					formatter: () => {
						const memPct = node.memPct.toFixed(0);
						return `<div style="font-family:monospace;font-size:12px">
							<strong style="color:${nc}">${node.label}</strong><br/>
							${node.arch} · ${node.roles.join(' · ')}<br/>
							<span style="color:#8b949e">CPU ${pCpu.toFixed(0)}% · MEM ${memPct}%</span><br/>
							<span style="color:#8b949e">${node.workloads.length} workloads</span>
							${node.gpu ? '<br/><span style="color:#5FAF00">GPU</span>' : ''}
						</div>`;
					},
				},
				x: undefined,
				y: undefined,
			});

			// Add workload nodes
			for (const wl of node.workloads) {
				const wlId = `wl:${wl.namespace}/${wl.label}:${node.id}`;
				const ci = catIndex.get(wl.namespace) ?? 0;

				chartNodes.push({
					id: wlId,
					name: wl.label,
					category: ci,
					symbolSize: 8 + wl.pods * 2,
					itemStyle: {
						color: wl.color,
						borderColor: wl.isService ? wl.color : 'transparent',
						borderWidth: wl.isService ? 2 : 0,
					},
					label: { show: false },
					tooltip: {
						formatter: () => `<div style="font-family:monospace;font-size:11px">
							<strong>${wl.label}</strong><br/>
							<span style="color:${wl.color}">${wl.namespace}</span> · ${wl.kind}<br/>
							<span style="color:#8b949e">${wl.pods} pod${wl.pods !== 1 ? 's' : ''}</span>
							${wl.isService ? '<br/><span style="color:#00AFAF">tailscale service</span>' : ''}
						</div>`,
					},
				});

				// Link workload to its node
				chartLinks.push({
					source: wlId,
					target: `node:${node.id}`,
					lineStyle: {
						color: `${wl.color}20`,
						width: 0.5,
					},
				});
			}
		}

		// DaemonSet cross-node links
		const wlByName = new Map<string, string[]>();
		for (const node of nocNodes) {
			for (const wl of node.workloads) {
				const key = `${wl.namespace}/${wl.label}`;
				const existing = wlByName.get(key) ?? [];
				const wlId = `wl:${key}:${node.id}`;
				existing.push(wlId);
				wlByName.set(key, existing);
			}
		}

		for (const [, ids] of wlByName) {
			if (ids.length < 2) continue;
			for (let i = 0; i < ids.length - 1; i++) {
				chartLinks.push({
					source: ids[i],
					target: ids[i + 1],
					lineStyle: {
						color: '#D787D740',
						width: 1,
						type: 'dashed',
					},
				});
			}
		}

		return { chartNodes, chartLinks, categories };
	}

	function initChart() {
		if (!container) return;

		chart = echarts.init(container, undefined, { renderer: 'canvas' });

		const { chartNodes, chartLinks, categories } = buildChartData();

		// Category colors
		const catColors = ['#00AFAF', ...namespaces.map(ns => COLORS.namespace.get(ns) ?? COLORS.fallback)];

		chart.setOption({
			backgroundColor: '#0a0a0f',
			tooltip: {
				trigger: 'item',
				backgroundColor: '#12121aee',
				borderColor: '#30363d',
				textStyle: { color: '#e6edf3', fontSize: 11, fontFamily: 'monospace' },
			},
			legend: {
				show: true,
				type: 'scroll',
				bottom: 10,
				left: 'center',
				textStyle: { color: '#6e7681', fontSize: 9, fontFamily: 'monospace' },
				itemWidth: 10,
				itemHeight: 10,
				data: categories.map((c, i) => ({
					name: c.name,
					itemStyle: { color: catColors[i] },
				})),
			},
			series: [{
				type: 'graph',
				layout: 'force',
				roam: true,
				draggable: true,
				categories: categories.map((c, i) => ({
					name: c.name,
					itemStyle: { color: catColors[i] },
				})),
				nodes: chartNodes,
				links: chartLinks,
				force: {
					repulsion: [80, 300],
					gravity: 0.05,
					edgeLength: [20, 80],
					layoutAnimation: true,
					friction: 0.6,
				},
				emphasis: {
					focus: 'adjacency',
					lineStyle: { width: 2 },
					itemStyle: { shadowBlur: 15 },
				},
				lineStyle: { curveness: 0.1 },
				scaleLimit: { min: 0.3, max: 5 },
			}],
		});
	}

	function updateChart() {
		if (!chart) return;
		const { chartNodes, chartLinks, categories } = buildChartData();
		const catColors = ['#00AFAF', ...namespaces.map(ns => COLORS.namespace.get(ns) ?? COLORS.fallback)];

		chart.setOption({
			series: [{
				categories: categories.map((c, i) => ({
					name: c.name,
					itemStyle: { color: catColors[i] },
				})),
				nodes: chartNodes,
				links: chartLinks,
			}],
		});
	}

	$effect(() => {
		if (nocNodes && chart) updateChart();
	});

	function handleResize() {
		chart?.resize();
	}

	onMount(() => {
		initChart();
		window.addEventListener('resize', handleResize);
	});

	onDestroy(() => {
		window.removeEventListener('resize', handleResize);
		chart?.dispose();
	});
</script>

<div class="chart-wrap" bind:this={container}></div>

<style>
	.chart-wrap {
		width: 100%;
		height: 100%;
		min-height: 400px;
	}
</style>
