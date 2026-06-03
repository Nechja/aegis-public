import type { DashboardState } from './types';

export interface WSCallbacks {
	onMessage: (data: DashboardState) => void;
	onOpen?: () => void;
	onError?: () => void;
}

export function connectWS(callbacks: WSCallbacks): () => void {
	let ws: WebSocket | null = null;
	let backoff = 1000;
	let timer: ReturnType<typeof setTimeout> | null = null;
	let closed = false;

	function connect() {
		if (closed) return;

		const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
		ws = new WebSocket(`${proto}//${location.host}/ws`);

		ws.onopen = () => {
			backoff = 1000;
			callbacks.onOpen?.();
		};

		ws.onmessage = (ev) => {
			try {
				const data: DashboardState = JSON.parse(ev.data);
				callbacks.onMessage(data);
			} catch {
				// ignore malformed frames
			}
		};

		ws.onclose = () => {
			callbacks.onError?.();
			reconnect();
		};

		ws.onerror = () => {
			ws?.close();
		};
	}

	function reconnect() {
		if (closed) return;
		timer = setTimeout(() => {
			backoff = Math.min(backoff * 2, 30000);
			connect();
		}, backoff);
	}

	connect();

	return () => {
		closed = true;
		if (timer) clearTimeout(timer);
		ws?.close();
	};
}
