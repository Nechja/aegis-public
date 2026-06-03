import * as THREE from 'three';
import type { GraphNode } from './types';
import { colorFor, sizeFor } from './theme';
import { iconPathFor } from './icons';

const textureCache = new Map<string, THREE.CanvasTexture>();

export function makeNodeObject(n: GraphNode): THREE.Object3D {
	const color = new THREE.Color(colorFor(n));
	const size = sizeFor(n);

	if (n.kind === 'node') return makeAnchorNode(n, color, size);
	return makeWorkloadNode(n, color, size);
}

// Machine node — a glowing diamond with a big label. Acts as a gravity anchor.
function makeAnchorNode(n: GraphNode, color: THREE.Color, size: number): THREE.Group {
	const group = new THREE.Group();

	// Diamond shape
	const geo = new THREE.OctahedronGeometry(size, 0);
	const mesh = new THREE.Mesh(geo, new THREE.MeshBasicMaterial({
		color, transparent: true, opacity: 0.7,
	}));
	group.add(mesh);

	// Glow
	group.add(new THREE.PointLight(color, 0.6, 80));

	// Big label
	group.add(makeLabel(n.label, color, size + 4, 28, true));

	// Arch tag below
	group.add(makeLabel(n.meta.arch, color, -(size + 3), 16, false));

	return group;
}

// Workload — icon sprite with name label
function makeWorkloadNode(n: GraphNode, color: THREE.Color, size: number): THREE.Group {
	const group = new THREE.Group();

	group.add(renderIconSprite(n, color, size));
	group.add(makeLabel(n.label, color, -(size * 1.6), 13, false));
	group.add(new THREE.PointLight(color, 0.15, size * 5));

	// Service ring
	if (n.meta.service) {
		const ring = new THREE.Mesh(
			new THREE.TorusGeometry(size * 2, 0.12, 8, 32),
			new THREE.MeshBasicMaterial({ color, transparent: true, opacity: 0.4 }),
		);
		group.add(ring);
	}

	return group;
}

function renderIconSprite(n: GraphNode, color: THREE.Color, size: number): THREE.Sprite {
	const iconPath = iconPathFor(n);
	const hex = color.getHexString();
	const cacheKey = `${iconPath}:${hex}`;

	let tex = textureCache.get(cacheKey);
	if (!tex) {
		const res = 128;
		const canvas = document.createElement('canvas');
		canvas.width = res;
		canvas.height = res;
		const ctx = canvas.getContext('2d')!;

		ctx.beginPath();
		ctx.arc(res / 2, res / 2, res / 2 - 4, 0, Math.PI * 2);
		ctx.fillStyle = `#${hex}15`;
		ctx.fill();
		ctx.strokeStyle = `#${hex}50`;
		ctx.lineWidth = 1.5;
		ctx.stroke();

		ctx.save();
		const iconSize = res * 0.45;
		const offset = (res - iconSize) / 2;
		ctx.translate(offset, offset);
		ctx.scale(iconSize / 24, iconSize / 24);
		ctx.fillStyle = `#${hex}`;
		ctx.shadowColor = `#${hex}`;
		ctx.shadowBlur = 3;
		ctx.fill(new Path2D(iconPath));
		ctx.restore();

		tex = new THREE.CanvasTexture(canvas);
		tex.needsUpdate = true;
		textureCache.set(cacheKey, tex);
	}

	const sprite = new THREE.Sprite(new THREE.SpriteMaterial({
		map: tex, transparent: true, opacity: 0.9, depthWrite: false,
	}));
	const scale = size * 2;
	sprite.scale.set(scale, scale, 1);
	return sprite;
}

function makeLabel(text: string, color: THREE.Color, offset: number, fontSize: number, bold: boolean): THREE.Sprite {
	const display = text.length > 20 ? text.slice(0, 18) + '..' : text;
	const canvas = document.createElement('canvas');
	const ctx = canvas.getContext('2d')!;
	canvas.width = 256;
	canvas.height = 64;
	ctx.font = `${bold ? 'bold ' : ''}${fontSize}px monospace`;
	ctx.fillStyle = `#${color.getHexString()}`;
	ctx.textAlign = 'center';
	ctx.textBaseline = 'middle';
	if (bold) {
		ctx.shadowColor = `#${color.getHexString()}`;
		ctx.shadowBlur = 6;
	}
	ctx.fillText(display, 128, 32);

	const tex = new THREE.CanvasTexture(canvas);
	const sprite = new THREE.Sprite(new THREE.SpriteMaterial({
		map: tex, transparent: true, opacity: bold ? 0.9 : 0.6,
	}));
	const scale = bold ? 20 : 10;
	sprite.scale.set(scale, scale * 0.25, 1);
	sprite.position.set(0, offset, 0);
	return sprite;
}
