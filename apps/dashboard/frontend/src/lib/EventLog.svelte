<script lang="ts">
	import { timeAgo } from './format';
	import type { ClusterEvent } from './types';

	let { events }: { events: ClusterEvent[] } = $props();

	const reasonColors: Record<string, string> = {
		CrashLoopBackOff: 'var(--neon-rose)',
		OOMKilled: 'var(--neon-rose)',
		BackOff: 'var(--neon-orange)',
		Failed: 'var(--neon-orange)',
		FailedScheduling: 'var(--neon-orange)',
		FailedMount: 'var(--neon-orange)',
		FailedAttachVolume: 'var(--neon-orange)',
		Unhealthy: 'var(--neon-peach)',
		Killing: 'var(--neon-dusty)',
		Evicted: 'var(--neon-rose)',
	};

	function reasonColor(reason: string): string {
		return reasonColors[reason] ?? 'var(--neon-orange)';
	}
</script>

<div class="log-scroll">
	{#each events as ev}
		<div class="log-row">
			<span class="log-time">{timeAgo(ev.timestamp)}</span>
			<span class="log-reason" style="color: {reasonColor(ev.reason)}; border-color: {reasonColor(ev.reason)}">{ev.reason}</span>
			<span class="log-obj">{ev.namespace}<span class="log-sep">/</span>{ev.object}</span>
			<span class="log-msg">{ev.message}</span>
		</div>
	{/each}
	{#if events.length === 0}
		<div class="log-empty">no issues</div>
	{/if}
</div>

<style>
	.log-scroll {
		max-height: 200px;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.log-row {
		display: flex;
		align-items: baseline;
		gap: 8px;
		padding: 3px 0;
		font-size: 11px;
		line-height: 1.4;
		border-bottom: 1px solid rgba(48, 54, 61, 0.3);
	}

	.log-time {
		color: var(--text-muted);
		font-size: 10px;
		flex-shrink: 0;
		min-width: 48px;
	}

	.log-reason {
		font-size: 9px;
		font-weight: 600;
		padding: 0 4px;
		border: 1px solid;
		border-radius: 2px;
		text-transform: uppercase;
		letter-spacing: 0.3px;
		flex-shrink: 0;
		white-space: nowrap;
	}

	.log-obj {
		color: var(--neon-cyan);
		font-size: 10px;
		flex-shrink: 0;
		white-space: nowrap;
	}

	.log-sep {
		color: var(--text-muted);
	}

	.log-msg {
		color: var(--text-secondary);
		font-size: 10px;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		flex: 1;
		min-width: 0;
	}

	.log-empty {
		color: var(--neon-green);
		font-size: 11px;
		padding: 8px 0;
	}

	@media (max-width: 900px) {
		.log-scroll {
			max-height: 300px;
		}

		.log-row {
			flex-wrap: wrap;
			gap: 4px;
		}

		.log-msg {
			flex-basis: 100%;
			white-space: normal;
		}
	}
</style>
