<script lang="ts">
	import { mdiArrowRight } from '@mdi/js';
	import Icon from './Icon.svelte';
	import StatusDot from './StatusDot.svelte';
	import type { Service } from './types';

	let { service }: { service: Service } = $props();

	const displayName = service.name
		.replace(/-/g, ' ')
		.replace(/\b\w/g, (c) => c.toUpperCase());

	const nsColors: Record<string, string> = {
		monitoring: 'var(--neon-green)',
		argocd: 'var(--neon-pink)',
		workloads: 'var(--neon-cyan)',
		'linkerd-viz': 'var(--neon-orange)',
		'longhorn-system': 'var(--neon-peach)',
	};

	const nsColor = nsColors[service.namespace] ?? 'var(--text-muted)';
</script>

<a href={service.url} class="svc" target="_blank" rel="noopener">
	<StatusDot color={nsColor} size={5} />
	<div class="svc-info">
		<span class="svc-name">{displayName}</span>
		<span class="svc-ns" style="color: {nsColor}">{service.namespace}</span>
	</div>
	<span class="svc-arrow">
		<Icon path={mdiArrowRight} size={12} />
	</span>
</a>

<style>
	.svc {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 8px 10px;
		color: var(--text-primary);
		text-decoration: none;
		border-radius: var(--radius);
		transition: background 0.12s ease;
	}

	.svc:hover {
		background: var(--bg-card);
	}

	.svc-info {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 1px;
	}

	.svc-name {
		font-size: 12px;
		font-weight: 600;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.svc-ns {
		font-size: 10px;
	}

	.svc-arrow {
		color: var(--text-muted);
		flex-shrink: 0;
		opacity: 0;
		transition: opacity 0.12s ease, transform 0.12s ease;
	}

	.svc:hover .svc-arrow {
		opacity: 1;
		color: var(--neon-cyan);
		transform: translateX(2px);
	}

	@media (max-width: 900px) {
		.svc-arrow {
			opacity: 0.5;
		}
	}
</style>
