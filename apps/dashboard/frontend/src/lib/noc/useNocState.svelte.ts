// Composable: workload/namespace selection state.

import type { NocWorkload, NocNode } from './types';

export function useSelection() {
	let selectedWl: string | null = $state(null);
	let selectedNs: string | null = $state(null);

	function selectWl(id: string) {
		selectedWl = selectedWl === id ? null : id;
		selectedNs = null;
	}

	function selectNs(ns: string) {
		selectedNs = selectedNs === ns ? null : ns;
		selectedWl = null;
	}

	function clear() {
		selectedWl = null;
		selectedNs = null;
	}

	return {
		get selectedWl() { return selectedWl; },
		get selectedNs() { return selectedNs; },
		selectWl,
		selectNs,
		clear,
	};
}

export function findWorkload(nodes: NocNode[], wlId: string): NocWorkload | null {
	for (const n of nodes) {
		const wl = n.workloads.find(w => w.id === wlId);
		if (wl) return wl;
	}
	return null;
}
