export function formatBytes(bytes: number): string {
	const gb = bytes / (1024 ** 3);
	return gb >= 1 ? `${gb.toFixed(1)}G` : `${(bytes / (1024 ** 2)).toFixed(0)}M`;
}

export function formatCPU(milli: number): string {
	return milli >= 1000 ? `${(milli / 1000).toFixed(1)}` : `${milli}m`;
}

export function barColor(pct: number): string {
	if (pct > 85) return 'var(--neon-rose)';
	if (pct > 60) return 'var(--neon-orange)';
	return 'var(--neon-green)';
}

export function blockBar(pct: number, width: number = 24): string {
	const filled = Math.round((pct / 100) * width);
	const empty = width - filled;
	return '█'.repeat(filled) + '░'.repeat(empty);
}

export function timeAgo(iso: string): string {
	const s = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
	if (s < 60) return `${s}s ago`;
	if (s < 3600) return `${Math.floor(s / 60)}m ago`;
	if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
	return `${Math.floor(s / 86400)}d ago`;
}
