<script lang="ts">
	import { onMount } from 'svelte';
	import type { Service, NodeInfo, ClusterSummary, ClusterEvent, Workload, NetPolicyRule } from './lib/types';
	import { connectWS } from './lib/ws';
	import { buildGraph } from './lib/graph/buildGraph';
	import DashView from './lib/DashView.svelte';

	let GraphComponent: any = $state(null);
	let MermaidComponent: any = $state(null);
	let NocMapComponent: any = $state(null);
	let ChartsComponent: any = $state(null);
	let screensaver = $state(false);
	let showDiagram = $state(false);
	let showMap = $state(false);
	let showCharts = $state(false);

	$effect(() => {
		if (screensaver && !GraphComponent) {
			import('./lib/graph/ClusterGraph.svelte').then(m => { GraphComponent = m.default; });
		}
		if (showDiagram && !MermaidComponent) {
			import('./lib/MermaidViewer.svelte').then(m => { MermaidComponent = m.default; });
		}
		if (showMap && !NocMapComponent) {
			import('./lib/NocFullMap.svelte').then(m => { NocMapComponent = m.default; });
		}
		if (showCharts && !ChartsComponent) {
			import('./lib/charts/ClusterCharts.svelte').then(m => { ChartsComponent = m.default; });
		}
	});

	const commitSha = (import.meta.env.VITE_COMMIT_SHA || 'dev').slice(0, 7);

	let services: Service[] = $state([]);
	let nodes: NodeInfo[] = $state([]);
	let cluster: ClusterSummary | null = $state(null);
	let events: ClusterEvent[] = $state([]);
	let workloads: Workload[] = $state([]);
	let netPolicies: NetPolicyRule[] = $state([]);
	let loading = $state(true);
	let error = $state('');
	let live = $state(false);

	const graphData = $derived(buildGraph(nodes, workloads, services));

	async function fetchAll() {
		try {
			const [svcRes, nodeRes, clusterRes] = await Promise.all([
				fetch('/api/services'),
				fetch('/api/nodes'),
				fetch('/api/cluster'),
			]);
			if (!svcRes.ok || !nodeRes.ok || !clusterRes.ok) throw new Error('API error');
			services = await svcRes.json();
			nodes = await nodeRes.json();
			cluster = await clusterRes.json();
			error = '';
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Could not reach API';
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		let fallback: ReturnType<typeof setInterval> | null = null;

		const cleanup = connectWS({
			onMessage: (data) => {
				services = data.services ?? [];
				nodes = data.nodes ?? [];
				cluster = data.cluster ?? null;
				events = data.events ?? [];
				workloads = data.workloads ?? [];
				netPolicies = data.netPolicies ?? [];
				loading = false;
				error = '';
			},
			onOpen: () => {
				live = true;
				if (fallback) { clearInterval(fallback); fallback = null; }
			},
			onError: () => {
				live = false;
				if (!fallback) { fetchAll(); fallback = setInterval(fetchAll, 15000); }
			},
		});

		return () => { cleanup(); if (fallback) clearInterval(fallback); };
	});
</script>

{#if screensaver}
	<div class="screensaver">
		{#if GraphComponent}
			<GraphComponent data={graphData} />
		{:else}
			<div class="blink">█</div>
		{/if}
		<button class="exit-ss" onclick={() => screensaver = false}>esc</button>
	</div>
{:else}
	<div class="shell">
		<div class="titlebar">
			<span class="title-text">aegis</span>
			<span class="title-sep">—</span>
			<span class="title-sub">cluster dashboard</span>
			<span class="live" class:connected={live}>{live ? 'live' : 'poll'}</span>
			<span class="sha">{commitSha}</span>
			<button class="ss-btn" onclick={() => showCharts = true} title="Cluster analytics">◉</button>
			<button class="ss-btn" onclick={() => showMap = true} title="Cluster map">◫</button>
			<button class="ss-btn" onclick={() => showDiagram = true} title="Mermaid diagram">◱</button>
			<button class="ss-btn" onclick={() => screensaver = true} title="3D screensaver">▣</button>
		</div>

		{#if loading}
			<div class="blink">█</div>
		{:else if error}
			<div class="error"><span class="error-text">✗ {error}</span></div>
		{:else}
			<DashView {nodes} {workloads} {services} {cluster} {events} />
		{/if}
	</div>
{/if}

{#if showCharts && ChartsComponent}
	<ChartsComponent onClose={() => showCharts = false} {nodes} {workloads} {services} {netPolicies} />
{/if}

{#if showMap && NocMapComponent}
	<NocMapComponent onClose={() => showMap = false} {nodes} {workloads} {services} />
{/if}

{#if showDiagram && MermaidComponent}
	<MermaidComponent onClose={() => showDiagram = false} {nodes} {workloads} {services} />
{/if}

<style>
	.shell {
		height: 100vh;
		display: flex;
		flex-direction: column;
		padding: 12px 16px;
		overflow: hidden;
	}

	.titlebar {
		display: flex;
		align-items: baseline;
		gap: 8px;
		padding: 8px 0 12px;
		border-bottom: 1px solid var(--border-default);
		margin-bottom: 12px;
		flex-shrink: 0;
	}

	.title-text {
		font-size: 22px;
		font-weight: 700;
		color: var(--neon-pink);
		text-shadow: var(--glow-pink);
		letter-spacing: 1px;
	}

	.title-sep { color: var(--text-muted); }
	.title-sub { font-size: 13px; color: var(--text-muted); }

	.live {
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 1px;
		color: var(--neon-orange);
		border: 1px solid var(--neon-orange);
		padding: 1px 6px;
		border-radius: 2px;
	}

	.live.connected {
		color: var(--neon-green);
		border-color: var(--neon-green);
		box-shadow: 0 0 4px var(--neon-green);
		text-shadow: 0 0 3px var(--neon-green);
	}

	.sha { margin-left: auto; font-size: 11px; color: var(--neon-dusty); }

	.ss-btn {
		font-family: var(--font);
		font-size: 12px;
		background: none;
		border: 1px solid var(--border-default);
		border-radius: 2px;
		color: var(--text-muted);
		cursor: pointer;
		padding: 1px 6px;
		transition: all 0.15s ease;
	}

	.ss-btn:hover {
		border-color: var(--neon-cyan);
		color: var(--neon-cyan);
		box-shadow: 0 0 4px var(--neon-cyan);
	}

	.screensaver {
		position: fixed;
		inset: 0;
		z-index: 100;
		background: var(--bg-canvas);
	}

	.exit-ss {
		position: fixed;
		top: 12px;
		right: 12px;
		z-index: 101;
		font-family: var(--font);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 1px;
		padding: 4px 10px;
		border: 1px solid var(--border-default);
		border-radius: 2px;
		background: rgba(10, 10, 15, 0.8);
		color: var(--text-muted);
		cursor: pointer;
		transition: all 0.15s ease;
	}

	.exit-ss:hover {
		border-color: var(--neon-pink);
		color: var(--neon-pink);
	}

	.error { border-left: 2px solid var(--neon-rose); padding-left: 12px; }
	.error-text { color: var(--neon-rose); font-size: 13px; text-shadow: var(--glow-rose); }

	.blink {
		color: var(--neon-pink);
		animation: blink 1s step-end infinite;
		font-size: 14px;
		padding: 8px 0;
	}

	@keyframes blink { 50% { opacity: 0; } }

	@media (max-width: 900px) {
		.shell { height: auto; min-height: 100vh; overflow-y: auto; padding: 8px 12px; }
		.titlebar { flex-wrap: wrap; }
		.ss-btn { display: none; }
	}

	@media (max-width: 480px) {
		.shell { padding: 6px 8px; }
		.title-text { font-size: 18px; }
		.title-sub { display: none; }
	}
</style>
