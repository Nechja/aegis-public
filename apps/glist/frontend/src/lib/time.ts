export function relative(iso: string): string {
  const then = new Date(iso).getTime();
  const now = Date.now();
  const s = Math.max(0, Math.floor((now - then) / 1000));
  if (s < 45) return 'now';
  if (s < 90) return '1m';
  const m = Math.floor(s / 60);
  if (m < 45) return `${m}m`;
  if (m < 90) return '1h';
  const h = Math.floor(m / 60);
  if (h < 22) return `${h}h`;
  const d = Math.floor(h / 24);
  if (d < 7) return `${d}d`;
  const wk = Math.floor(d / 7);
  if (wk < 5) return `${wk}w`;
  return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}
