<script lang="ts">
	import { onMount } from 'svelte';
	import type { NodeInfo, Workload, Service } from './types';
	import { generateOverview, generateNamespace } from './mermaid/generate';

	let { onClose, nodes, workloads, services }: {
		onClose: () => void;
		nodes: NodeInfo[];
		workloads: Workload[];
		services: Service[];
	} = $props();

	let rendered = $state('');
	let loading = $state(true);
	let showSource = $state(false);
	let copied = $state(false);
	let diagramEl: HTMLDivElement;
	let selectedNs = $state('');
	let renderCount = 0;
	let mermaidLib: any = null;

	const namespaces = $derived([...new Set(workloads.map(w => w.namespace))].sort());

	const source = $derived(
		selectedNs
			? generateNamespace(selectedNs, nodes, workloads, services)
			: generateOverview(nodes, workloads, services)
	);

	async function render() {
		loading = true;
		try {
			if (!mermaidLib) {
				mermaidLib = (await import('mermaid')).default;
				mermaidLib.initialize({
					startOnLoad: false,
					theme: 'dark',
					themeVariables: {
						darkMode: true,
						background: '#0a0a0f',
						primaryColor: '#D787D7',
						primaryTextColor: '#e6edf3',
						primaryBorderColor: '#D787D7',
						lineColor: '#484f58',
						secondaryColor: '#00AFAF',
						tertiaryColor: '#12121a',
						fontFamily: 'monospace',
						fontSize: '12px',
						clusterBkg: '#12121a',
						clusterBorder: '#30363d',
					},
					flowchart: {
						padding: 16,
						nodeSpacing: 30,
						rankSpacing: 50,
						curve: 'basis',
						useMaxWidth: true,
					},
				});
			}

			renderCount++;
			const { svg } = await mermaidLib.render(`mermaid-${renderCount}`, source);
			rendered = svg;
		} catch (e) {
			rendered = `<pre style="color:var(--neon-rose)">${e}</pre>`;
		} finally {
			loading = false;
		}
	}

	// Re-render when source changes
	$effect(() => {
		if (source) render();
	});

	async function copySource() {
		await navigator.clipboard.writeText(source);
		flash();
	}

	function copyMarkdown() {
		navigator.clipboard.writeText('```mermaid\n' + source + '\n```');
		flash();
	}

	async function exportPNG() {
		if (!diagramEl) return;
		const svg = diagramEl.querySelector('svg');
		if (!svg) return;

		const svgData = new XMLSerializer().serializeToString(svg);
		const svgBlob = new Blob([svgData], { type: 'image/svg+xml;charset=utf-8' });
		const url = URL.createObjectURL(svgBlob);

		const img = new Image();
		img.onload = () => {
			const scale = 2;
			const canvas = document.createElement('canvas');
			canvas.width = img.naturalWidth * scale;
			canvas.height = img.naturalHeight * scale;
			const ctx = canvas.getContext('2d')!;
			ctx.fillStyle = '#0a0a0f';
			ctx.fillRect(0, 0, canvas.width, canvas.height);
			ctx.scale(scale, scale);
			ctx.drawImage(img, 0, 0);

			canvas.toBlob((blob) => {
				if (!blob) return;
				const a = document.createElement('a');
				a.href = URL.createObjectURL(blob);
				const name = selectedNs || 'cluster';
				a.download = `aegis-${name}-${new Date().toISOString().slice(0, 10)}.png`;
				a.click();
				URL.revokeObjectURL(a.href);
			}, 'image/png');
			URL.revokeObjectURL(url);
		};
		img.src = url;
	}

	function flash() {
		copied = true;
		setTimeout(() => copied = false, 2000);
	}
</script>

<div class="overlay" onclick={onClose}>
	<div class="modal" onclick={(e) => e.stopPropagation()}>
		<div class="modal-header">
			<h2>{selectedNs || 'cluster overview'}</h2>
			<div class="actions">
				<select class="ns-select" bind:value={selectedNs}>
					<option value="">overview</option>
					{#each namespaces as ns}
						<option value={ns}>{ns}</option>
					{/each}
				</select>
				<button class="btn" class:active={showSource} onclick={() => showSource = !showSource}>
					{showSource ? 'preview' : 'source'}
				</button>
				<button class="btn" onclick={copySource}>
					{copied ? '✓ copied' : 'copy raw'}
				</button>
				<button class="btn" onclick={copyMarkdown}>copy md</button>
				<button class="btn" onclick={exportPNG}>export png</button>
				<button class="btn close" onclick={onClose}>✕</button>
			</div>
		</div>

		<div class="modal-body">
			{#if loading}
				<div class="loading">█ rendering...</div>
			{:else if showSource}
				<pre class="source">{source}</pre>
			{:else}
				<div class="diagram" bind:this={diagramEl}>{@html rendered}</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.overlay {
		position: fixed; inset: 0; z-index: 50;
		background: rgba(0, 0, 0, 0.85);
		display: flex; align-items: stretch; justify-content: center;
		padding: 16px;
	}

	.modal {
		background: var(--bg-canvas); border: 1px solid var(--border-default);
		border-radius: var(--radius-lg); width: 100%;
		display: flex; flex-direction: column; overflow: hidden;
	}

	.modal-header {
		display: flex; align-items: center; justify-content: space-between;
		padding: 8px 14px; border-bottom: 1px solid var(--border-default);
		flex-shrink: 0; background: var(--bg-card);
	}

	h2 { font-size: 12px; font-weight: 700; color: var(--neon-pink); text-shadow: var(--glow-pink); text-transform: uppercase; letter-spacing: 1px; }

	.actions { display: flex; gap: 4px; align-items: center; }

	.ns-select {
		font-family: var(--font); font-size: 9px; text-transform: uppercase;
		padding: 2px 6px; border: 1px solid var(--border-default); border-radius: 2px;
		background: var(--bg-inset); color: var(--text-primary); cursor: pointer;
	}
	.ns-select:focus { outline: 1px solid var(--neon-pink); }

	.btn {
		font-family: var(--font); font-size: 9px; text-transform: uppercase;
		letter-spacing: 0.5px; padding: 3px 8px; border: 1px solid var(--border-default);
		border-radius: 2px; background: transparent; color: var(--text-muted);
		cursor: pointer; transition: all 0.12s ease;
	}
	.btn:hover { border-color: var(--neon-pink); color: var(--neon-pink); }
	.btn.active { border-color: var(--neon-green); color: var(--neon-green); }
	.btn.close:hover { border-color: var(--neon-rose); color: var(--neon-rose); }

	.modal-body { flex: 1; overflow: auto; padding: 16px; }

	.loading { color: var(--neon-pink); animation: blink 1s step-end infinite; }
	@keyframes blink { 50% { opacity: 0; } }

	.source { font-size: 11px; color: var(--text-primary); white-space: pre; overflow-x: auto; line-height: 1.5; }

	.diagram { display: flex; justify-content: center; align-items: flex-start; min-height: 100%; }
	.diagram :global(svg) { max-width: 100%; height: auto; }

	@media (max-width: 900px) {
		.overlay { padding: 6px; }
		.actions { flex-wrap: wrap; }
	}
</style>
