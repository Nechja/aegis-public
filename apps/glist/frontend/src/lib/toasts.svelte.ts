export type Tone = 'spark' | 'milestone' | 'cheer' | 'sub' | 'error';

type CheerKind =
  | 'first'
  | 'milestone25'
  | 'milestone50'
  | 'milestone75'
  | 'complete'
  | 'missing';

type ToastOpts = {
  tone?: Tone;
  ttl?: number;
  source?: 'shoppy-fren' | null;
  face?: string;
};

type Toast = {
  id: number;
  text: string;
  tone: Tone;
  ttl: number;
  source: 'shoppy-fren' | null;
  face: string;
};

export type Particle = {
  id: number;
  color: 'pink' | 'orange' | 'cyan' | 'green' | 'peach';
  x: number;
  y: number;
  rot: number;
  size: number;
  delay: number;
};

export type Celebration = {
  id: number;
  kind: CheerKind;
  particles: Particle[];
  duration: number;
  hero: string | null;
};

let nextId = 0;

export const store = $state<{ toasts: Toast[] }>({ toasts: [] });
export const celebrations = $state<{ active: Celebration[] }>({ active: [] });

const mascotFor: Record<Tone, string> = {
  spark: '(˃ᴗ˂)',
  milestone: '(✿˃ᴗ˂)',
  cheer: '٩(˃ᴗ˂)۶',
  sub: '(´-ω-`)',
  error: '(´•_•`)'
};

export function toast(text: string, opts: ToastOpts = {}): number {
  const tone = opts.tone ?? 'spark';
  const t: Toast = {
    id: ++nextId,
    text,
    tone,
    ttl: opts.ttl ?? 4000,
    source: opts.source ?? null,
    face: opts.face ?? mascotFor[tone]
  };
  store.toasts = [...store.toasts, t];
  setTimeout(() => dismiss(t.id), t.ttl);
  return t.id;
}

export function dismiss(id: number) {
  store.toasts = store.toasts.filter((t) => t.id !== id);
}

// ---- celebrations ----

type CelebConfig = {
  count: number;
  colors: Particle['color'][];
  size: [number, number];
  duration: number;
  spreadX: number;
  spreadY: number;
  hero: string | null;
};

const celebConfig: Record<CheerKind, CelebConfig> = {
  first: {
    count: 10, colors: ['pink', 'peach'], size: [3, 6], duration: 1200,
    spreadX: 70, spreadY: 50, hero: null
  },
  milestone25: {
    count: 16, colors: ['cyan', 'pink'], size: [3, 6], duration: 1500,
    spreadX: 90, spreadY: 70, hero: null
  },
  milestone50: {
    count: 22, colors: ['pink', 'orange'], size: [4, 7], duration: 1800,
    spreadX: 110, spreadY: 90, hero: null
  },
  milestone75: {
    count: 26, colors: ['orange', 'peach', 'pink'], size: [4, 7], duration: 1900,
    spreadX: 120, spreadY: 100, hero: null
  },
  complete: {
    count: 50, colors: ['pink', 'orange', 'cyan', 'green', 'peach'], size: [4, 9],
    duration: 2800, spreadX: 140, spreadY: 140, hero: '٩(˃ᴗ˂)۶'
  },
  missing: {
    count: 0, colors: [], size: [0, 0], duration: 0, spreadX: 0, spreadY: 0, hero: null
  }
};

function rand(a: number, b: number): number {
  return a + Math.random() * (b - a);
}

function makeParticles(c: CelebConfig): Particle[] {
  const p: Particle[] = [];
  for (let i = 0; i < c.count; i++) {
    p.push({
      id: i,
      color: c.colors[i % c.colors.length],
      x: (Math.random() - 0.5) * c.spreadX,
      y: (Math.random() - 0.5) * c.spreadY - 20,
      rot: rand(-540, 540),
      size: rand(c.size[0], c.size[1]),
      delay: Math.random() * 300
    });
  }
  return p;
}

export function celebrate(kind: CheerKind) {
  const cfg = celebConfig[kind];
  if (cfg.count === 0 && !cfg.hero) return;
  const c: Celebration = {
    id: ++nextId,
    kind,
    particles: makeParticles(cfg),
    duration: cfg.duration,
    hero: cfg.hero
  };
  celebrations.active = [...celebrations.active, c];
  setTimeout(() => dismissCelebration(c.id), cfg.duration + 200);
}

export function dismissCelebration(id: number) {
  celebrations.active = celebrations.active.filter((c) => c.id !== id);
}

// ---- cheer via SSE streaming ----

async function fallbackCheer(kind: CheerKind, listId?: number): Promise<string> {
  try {
    const qs = listId ? `&list=${listId}` : '';
    const r = await fetch(`/api/ai/cheer?kind=${kind}${qs}`);
    if (r.ok) {
      const j = (await r.json()) as { text?: string };
      if (j.text && j.text.trim()) return j.text.trim();
    }
  } catch {
    // ignore
  }
  return 'nice';
}

type CheerOpts = { listId?: number };

// Pool-only cheer. If the list has a personalized pool (via `/api/ai/cheer?list=X`),
// the server returns from there; otherwise it falls back to the generic seeded
// pool. Instant either way — no SSE, no thinking state.
export async function cheer(kind: CheerKind, opts: CheerOpts = {}) {
  const tone: Tone =
    kind === 'complete' ? 'cheer' :
    kind === 'missing' ? 'sub' :
    kind.startsWith('milestone') ? 'milestone' : 'spark';
  const ttl = kind === 'complete' ? 7000 : 4200;

  celebrate(kind);

  const text = await fallbackCheer(kind, opts.listId);
  toast(text, { tone, ttl, source: 'shoppy-fren', face: mascotFor[tone] });
}
