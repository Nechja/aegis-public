<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import type { GraphData } from './types';
	import ForceGraph3D from '3d-force-graph';
	import * as THREE from 'three';
	import { EffectComposer } from 'three/addons/postprocessing/EffectComposer.js';
	import { RenderPass } from 'three/addons/postprocessing/RenderPass.js';
	import { UnrealBloomPass } from 'three/addons/postprocessing/UnrealBloomPass.js';
	import { COLORS, tooltip } from './theme';
	import { makeNodeObject } from './objects';
	import { flyTo } from './camera';

	let { data }: { data: GraphData } = $props();
	let container: HTMLDivElement;
	let graph: any = null;
	let searchOpen = $state(false);
	let searchQuery = $state('');
	let layerOpen = $state(false);

	const namespaces = $derived(
		[...new Set(data.nodes.filter(n => n.kind === 'workload').map(n => n.group))].sort()
	);
	let hiddenLayers: Set<string> = $state(new Set());

	const visibleData = $derived({
		nodes: data.nodes.filter(n => {
			if (n.kind === 'node') return true;
			return !hiddenLayers.has(n.group);
		}),
		links: data.links.filter(l => {
			const src = data.nodes.find(n => n.id === l.source);
			const tgt = data.nodes.find(n => n.id === l.target);
			if (!src || !tgt) return true;
			if (src.kind === 'workload' && hiddenLayers.has(src.group)) return false;
			if (tgt.kind === 'workload' && hiddenLayers.has(tgt.group)) return false;
			return true;
		}),
	});

	const targets = $derived(
		visibleData.nodes
			.map(n => ({ id: n.id, label: n.label, kind: n.kind, group: n.kind === 'node' ? n.meta.arch : n.meta.namespace }))
			.sort((a, b) => {
				if (a.kind !== b.kind) return a.kind === 'node' ? -1 : 1;
				return a.label.localeCompare(b.label);
			})
	);

	const filtered = $derived(
		searchQuery
			? targets.filter(t => t.label.toLowerCase().includes(searchQuery.toLowerCase()) || t.group.toLowerCase().includes(searchQuery.toLowerCase()))
			: targets
	);

	function handleFlyTo(id: string) {
		if (!graph) return;
		flyTo(graph, id);
		searchOpen = false;
		layerOpen = false;
		searchQuery = '';
	}

	function toggleLayer(ns: string) {
		const next = new Set(hiddenLayers);
		if (next.has(ns)) next.delete(ns);
		else next.add(ns);
		hiddenLayers = next;
	}

	function initGraph() {
		if (!container) return;

		graph = ForceGraph3D()(container)
			.backgroundColor(COLORS.bg)
			.showNavInfo(false)
			.nodeThreeObject((n: any) => makeNodeObject(n))
			.nodeLabel((n: any) => tooltip(n))
			.onNodeClick((n: any) => handleFlyTo(n.id))
			.onNodeHover((n: any) => { container.style.cursor = n ? 'pointer' : 'default'; })
			.linkColor(() => COLORS.link)
			.linkOpacity(0.2)
			.linkWidth((l: any) => 0.2 + l.value * 0.3)
			.linkDirectionalParticles((l: any) => Math.min(l.value * 2, 6))
			.linkDirectionalParticleWidth(1)
			.linkDirectionalParticleSpeed(0.005)
			.linkDirectionalParticleColor(() => COLORS.particle)
			.cooldownTime(4000)
			.warmupTicks(80);

		// Forces: nodes repel each other, workloads cluster tight to their node
		graph.d3Force('charge')?.strength((n: any) => n.kind === 'node' ? -600 : -8);
		graph.d3Force('link')
			?.distance((l: any) => {
				const hasNode = l.source?.kind === 'node' || l.target?.kind === 'node';
				return hasNode ? 25 : 15;
			})
			.strength((l: any) => {
				const hasNode = l.source?.kind === 'node' || l.target?.kind === 'node';
				return hasNode ? 1.0 : 0.3;
			});

		// Bloom
		const renderer = graph.renderer();
		const scene = graph.scene();
		const camera = graph.camera();
		const composer = new EffectComposer(renderer);
		composer.addPass(new RenderPass(scene, camera));
		composer.addPass(new UnrealBloomPass(
			new THREE.Vector2(container.clientWidth, container.clientHeight),
			0.8, 0.4, 0.2,
		));
		graph.onAfterRender(() => composer.render());

		// Scene
		scene.add(new THREE.AmbientLight(COLORS.ambient, 1));
		const grid = new THREE.GridHelper(600, 40, COLORS.grid, COLORS.grid);
		grid.position.y = -60;
		(grid.material as THREE.Material).transparent = true;
		(grid.material as THREE.Material).opacity = 0.25;
		scene.add(grid);

		graph.cameraPosition({ x: 0, y: 80, z: 300 });
		updateData();
	}

	function updateData() {
		if (!graph) return;
		const oldNodes = new Map<string, { x: number; y: number; z: number; vx?: number; vy?: number; vz?: number }>();
		for (const n of graph.graphData().nodes) {
			if (n.x !== undefined) oldNodes.set(n.id, { x: n.x, y: n.y, z: n.z, vx: n.vx, vy: n.vy, vz: n.vz });
		}
		graph.graphData({
			nodes: visibleData.nodes.map(n => {
				const pos = oldNodes.get(n.id);
				return pos ? { ...n, ...pos } : { ...n };
			}),
			links: visibleData.links.map(l => ({ ...l })),
		});
	}

	$effect(() => { if (visibleData && graph) updateData(); });

	function handleResize() {
		if (graph && container) {
			graph.width(container.clientWidth);
			graph.height(container.clientHeight);
		}
	}

	onMount(() => {
		initGraph();
		window.addEventListener('resize', handleResize);
	});

	onDestroy(() => {
		window.removeEventListener('resize', handleResize);
		graph?._destructor?.();
	});
</script>

<div class="graph-wrap">
	<div class="controls">
		<div class="search-wrap">
			<button class="ctrl" onclick={() => { searchOpen = !searchOpen; layerOpen = false; searchQuery = ''; }}>
				⌕ find
			</button>
			{#if searchOpen}
				<div class="dropdown">
					<input class="search-input" type="text" placeholder="search..." bind:value={searchQuery} autofocus />
					<div class="search-list">
						{#each filtered as t}
							<button class="search-item" onclick={() => handleFlyTo(t.id)}>
								<span class="kind-icon" class:is-node={t.kind === 'node'}>{t.kind === 'node' ? '◆' : '●'}</span>
								<span class="item-label">{t.label}</span>
								<span class="item-group">{t.group}</span>
							</button>
						{/each}
					</div>
				</div>
			{/if}
		</div>
		<div class="search-wrap">
			<button class="ctrl" onclick={() => { layerOpen = !layerOpen; searchOpen = false; }}>
				◫ layers
			</button>
			{#if layerOpen}
				<div class="dropdown">
					<div class="search-list">
						{#each namespaces as ns}
							<button class="search-item" onclick={() => toggleLayer(ns)}>
								<span class="layer-dot" style="background: {COLORS.namespace.get(ns) ?? COLORS.fallback}; opacity: {hiddenLayers.has(ns) ? 0.2 : 1}"></span>
								<span class="item-label" style="opacity: {hiddenLayers.has(ns) ? 0.4 : 1}">{ns}</span>
								<span class="layer-toggle">{hiddenLayers.has(ns) ? 'off' : 'on'}</span>
							</button>
						{/each}
					</div>
				</div>
			{/if}
		</div>
	</div>
	<div class="canvas" bind:this={container}></div>
</div>

<style>
	.graph-wrap {
		width: 100%;
		height: 100%;
		min-height: 400px;
		border: 1px solid var(--border-default);
		border-radius: var(--radius);
		overflow: hidden;
		position: relative;
	}

	.canvas { width: 100%; height: 100%; }
	.canvas :global(canvas) { outline: none; }

	.controls {
		position: absolute;
		top: 10px;
		left: 10px;
		z-index: 10;
		display: flex;
		gap: 4px;
		align-items: flex-start;
	}

	.ctrl {
		font-family: var(--font);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.8px;
		padding: 4px 10px;
		border: 1px solid var(--border-default);
		border-radius: 2px;
		background: rgba(10, 10, 15, 0.85);
		color: var(--text-muted);
		cursor: pointer;
		transition: all 0.15s ease;
		white-space: nowrap;
	}

	.ctrl:hover { border-color: var(--neon-pink); color: var(--neon-pink); }

	.search-wrap { position: relative; }

	.dropdown {
		position: absolute;
		top: 100%;
		left: 0;
		margin-top: 4px;
		width: 220px;
		background: rgba(10, 10, 15, 0.95);
		border: 1px solid var(--border-default);
		border-radius: 3px;
		overflow: hidden;
		box-shadow: 0 8px 24px rgba(0, 0, 0, 0.6);
	}

	.search-input {
		width: 100%;
		padding: 6px 10px;
		font-family: var(--font);
		font-size: 11px;
		background: transparent;
		border: none;
		border-bottom: 1px solid var(--border-default);
		color: var(--text-primary);
		outline: none;
	}

	.search-input::placeholder { color: var(--text-muted); }
	.search-list { max-height: 280px; overflow-y: auto; }

	.search-item {
		display: flex;
		align-items: center;
		gap: 6px;
		width: 100%;
		padding: 5px 10px;
		background: transparent;
		border: none;
		cursor: pointer;
		font-family: var(--font);
		font-size: 11px;
		color: var(--text-secondary);
		text-align: left;
		transition: background 0.1s ease;
	}

	.search-item:hover { background: rgba(215, 135, 215, 0.08); color: var(--text-primary); }

	.kind-icon { font-size: 8px; color: var(--neon-cyan); }
	.kind-icon.is-node { color: var(--neon-pink); }
	.item-label { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.item-group { font-size: 9px; color: var(--text-muted); }

	.layer-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
		transition: opacity 0.15s ease;
	}

	.layer-toggle {
		font-size: 9px;
		color: var(--text-muted);
		text-transform: uppercase;
	}
</style>
