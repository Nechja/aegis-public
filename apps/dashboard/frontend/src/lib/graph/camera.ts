// Camera fly-to for the cluster graph.

export function flyTo(graph: any, nodeId: string, duration = 1500) {
	const gd = graph.graphData();
	const target = gd.nodes.find((n: any) => n.id === nodeId);
	if (!target || target.x === undefined) return;

	const dist = target.kind === 'node' ? 100 : 50;
	graph.cameraPosition(
		{
			x: target.x + dist * 0.6,
			y: target.y + dist * 0.4,
			z: target.z + dist * 0.8,
		},
		{ x: target.x, y: target.y, z: target.z },
		duration,
	);
}
