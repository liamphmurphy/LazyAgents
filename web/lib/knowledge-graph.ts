// A force layout for the whole-vault graph: springs along links, repulsion
// between nearby nodes (bucketed in a grid, so a few thousand pages lay out
// in well under a second), and a gentle pull to the centre so unlinked
// clusters and orphans stay on screen. Deterministic for the same input.

export type GNode = { id: string; group: string };
export type GLink = { source: string; target: string };
export type Placed = { id: string; x: number; y: number; degree: number };

function seeded(n: number) {
  let s = n >>> 0 || 1;
  return () => {
    s ^= s << 13;
    s ^= s >>> 17;
    s ^= s << 5;
    return ((s >>> 0) % 10000) / 10000;
  };
}

export function layoutGraph(nodes: GNode[], links: GLink[], iterations = 220): Placed[] {
  const n = nodes.length;
  if (!n) return [];
  const index = new Map(nodes.map((v, i) => [v.id, i]));
  const rand = seeded(n * 7919 + links.length);
  // Groups start in their own sector, so folders read as clusters.
  const groups = [...new Set(nodes.map((v) => v.group))];
  const x = new Float64Array(n);
  const y = new Float64Array(n);
  const radius = Math.sqrt(n) * 22;
  nodes.forEach((v, i) => {
    const a = (groups.indexOf(v.group) / Math.max(1, groups.length)) * Math.PI * 2;
    const r = radius * (0.35 + rand() * 0.6);
    x[i] = Math.cos(a) * radius * 0.6 + Math.cos(rand() * 6.283) * r * 0.5;
    y[i] = Math.sin(a) * radius * 0.6 + Math.sin(rand() * 6.283) * r * 0.5;
  });
  const edges = links.flatMap((l) => {
    const a = index.get(l.source);
    const b = index.get(l.target);
    return a === undefined || b === undefined || a === b ? [] : [[a, b] as const];
  });
  const degree = new Int32Array(n);
  for (const [a, b] of edges) {
    degree[a]++;
    degree[b]++;
  }
  const ideal = 34;
  const cell = ideal * 3;
  const vx = new Float64Array(n);
  const vy = new Float64Array(n);
  for (let it = 0; it < iterations; it++) {
    const t = 1 - it / iterations;
    vx.fill(0);
    vy.fill(0);
    const grid = new Map<string, number[]>();
    for (let i = 0; i < n; i++) {
      const k = `${Math.floor(x[i] / cell)},${Math.floor(y[i] / cell)}`;
      (grid.get(k) ?? grid.set(k, []).get(k)!).push(i);
    }
    for (let i = 0; i < n; i++) {
      const cx = Math.floor(x[i] / cell);
      const cy = Math.floor(y[i] / cell);
      for (let gx = cx - 1; gx <= cx + 1; gx++)
        for (let gy = cy - 1; gy <= cy + 1; gy++)
          for (const j of grid.get(`${gx},${gy}`) ?? []) {
            if (j <= i) continue;
            const dx = x[i] - x[j] || 0.01;
            const dy = y[i] - y[j] || 0.01;
            const d2 = dx * dx + dy * dy;
            if (d2 > cell * cell) continue;
            const f = (ideal * ideal) / d2;
            vx[i] += dx * f * 0.05;
            vy[i] += dy * f * 0.05;
            vx[j] -= dx * f * 0.05;
            vy[j] -= dy * f * 0.05;
          }
    }
    for (const [a, b] of edges) {
      const dx = x[b] - x[a];
      const dy = y[b] - y[a];
      const d = Math.sqrt(dx * dx + dy * dy) || 0.01;
      const f = ((d - ideal) / d) * 0.06;
      vx[a] += dx * f;
      vy[a] += dy * f;
      vx[b] -= dx * f;
      vy[b] -= dy * f;
    }
    const step = 6 * t + 0.5;
    for (let i = 0; i < n; i++) {
      vx[i] -= x[i] * 0.004;
      vy[i] -= y[i] * 0.004;
      const m = Math.sqrt(vx[i] * vx[i] + vy[i] * vy[i]);
      const s = m > step ? step / m : 1;
      x[i] += vx[i] * s;
      y[i] += vy[i] * s;
    }
  }
  return nodes.map((v, i) => ({ id: v.id, x: x[i], y: y[i], degree: degree[i] }));
}

/** The box around placed nodes, with a margin, for the SVG viewBox. */
export function bounds(p: Placed[], margin = 60): { x: number; y: number; w: number; h: number } {
  if (!p.length) return { x: -100, y: -100, w: 200, h: 200 };
  let x0 = Infinity,
    y0 = Infinity,
    x1 = -Infinity,
    y1 = -Infinity;
  for (const q of p) {
    x0 = Math.min(x0, q.x);
    y0 = Math.min(y0, q.y);
    x1 = Math.max(x1, q.x);
    y1 = Math.max(y1, q.y);
  }
  return { x: x0 - margin, y: y0 - margin, w: x1 - x0 + margin * 2, h: y1 - y0 + margin * 2 };
}
