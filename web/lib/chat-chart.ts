// Charts: a ```chart fence or lesson block holds a JSON spec of real numbers,
// and components/domain/chart-visual draws it, so scales can't be hand-plotted
// wrong. The server applies the same limits to lessons (runs/visuals.go).

export const isChartFence = (info: string | undefined) => (info ?? "").trim().split(/\s+/)[0]?.toLowerCase() === "chart";

export type ChartType = "bar" | "line" | "scatter";
export type ChartSeries = { name: string; values: (number | null)[]; points: [number, number][] };
export type ChartSpec = {
  type: ChartType;
  title?: string;
  subtitle?: string;
  x: { label?: string };
  y: { label?: string; unit?: string };
  categories: string[];
  series: ChartSeries[];
};

export const CHART_LIMITS = { series: 8, scatterSeries: 3, categories: 60, points: 500 };

const text = (v: unknown) => (typeof v === "string" && v.trim() ? v.trim() : undefined);
const finite = (v: unknown): v is number => typeof v === "number" && Number.isFinite(v);
const record = (v: unknown): Record<string, unknown> => (v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : {});

/** The chart a fence describes, or why it can't be drawn. */
export function parseChart(code: string): { spec: ChartSpec } | { error: string } {
  let raw: unknown;
  try {
    raw = JSON.parse(code);
  } catch {
    return { error: "This chart isn't valid JSON." };
  }
  const r = record(raw);
  const type = r.type;
  if (type !== "bar" && type !== "line" && type !== "scatter") return { error: "A chart's type must be bar, line or scatter." };
  const list = Array.isArray(r.series) ? r.series.map(record) : [];
  const maxSeries = type === "scatter" ? CHART_LIMITS.scatterSeries : CHART_LIMITS.series;
  if (list.length === 0 || list.length > maxSeries) return { error: `A ${type} chart needs 1 to ${maxSeries} series.` };
  const categories = Array.isArray(r.categories) ? r.categories.map((c) => String(c)) : [];
  if (type !== "scatter" && (categories.length === 0 || categories.length > CHART_LIMITS.categories)) return { error: `A ${type} chart needs 1 to ${CHART_LIMITS.categories} categories.` };
  const series: ChartSeries[] = [];
  for (const s of list) {
    const name = text(s.name) ?? "";
    if (!name && list.length > 1) return { error: "Each series needs a name." };
    if (type === "scatter") {
      const points = (Array.isArray(s.points) ? s.points : []).filter((p): p is [number, number] => Array.isArray(p) && p.length === 2 && finite(p[0]) && finite(p[1]));
      if (points.length === 0 || points.length > CHART_LIMITS.points) return { error: `Each series needs 1 to ${CHART_LIMITS.points} points.` };
      series.push({ name, values: [], points });
    } else {
      const values = (Array.isArray(s.values) ? s.values : []).map((v) => (finite(v) ? v : null));
      if (values.length !== categories.length || values.every((v) => v === null)) return { error: `Series "${name}" needs one value per category.` };
      series.push({ name, values, points: [] });
    }
  }
  const x = record(r.x);
  const y = record(r.y);
  return { spec: { type, title: text(r.title), subtitle: text(r.subtitle), x: { label: text(x.label) }, y: { label: text(y.label), unit: text(y.unit) }, categories, series } };
}

/** Evenly spaced round numbers covering min to max, with min and max widened to reach them. */
export function niceTicks(min: number, max: number, count = 5): number[] {
  if (min === max) {
    const pad = Math.abs(min) || 1;
    return niceTicks(min - pad, max + pad, count);
  }
  const rough = (max - min) / Math.max(1, count - 1);
  const mag = 10 ** Math.floor(Math.log10(rough));
  const step = [1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => s >= rough) ?? 10 * mag;
  const first = Math.floor(min / step);
  const last = Math.ceil(max / step);
  // Rounded from whole multiples so floats like 0.30000000000000004 can't show.
  return Array.from({ length: last - first + 1 }, (_, i) => Number(((first + i) * step).toPrecision(12)));
}

/** Compact from 1,000 up: 1K, 2.5M. */
export function compactNumber(v: number): string {
  const abs = Math.abs(v);
  for (const [limit, suffix] of [[1e12, "T"], [1e9, "B"], [1e6, "M"], [1e3, "K"]] as const) {
    if (abs >= limit) return `${Number((v / limit).toPrecision(3))}${suffix}`;
  }
  return String(Number(v.toPrecision(4)));
}

/** A currency symbol leads the number; any other unit follows it. */
export function withUnit(value: string, unit: string | undefined): string {
  if (!unit) return value;
  if (/^[$€£¥]$/.test(unit)) return value.startsWith("-") ? `-${unit}${value.slice(1)}` : `${unit}${value}`;
  return /^[%°]/.test(unit) ? `${value}${unit}` : `${value} ${unit}`;
}

// Layout: pure pixels from a spec and a width, so it can be tested without a browser.

const BAR_MAX = 24;
const BAR_ROW_MAX = 18;
const BAR_GAP = 2;
const BAR_RADIUS = 4;
const MARKER_MAX_POINTS = 24;
const LABEL_MIN_WIDTH = 420;
const SCATTER_HIT_PX = 48;

const textWidth = (s: string) => s.length * 6.2;
const clip = (s: string, max: number) => (s.length > max ? `${s.slice(0, Math.max(1, max - 1))}…` : s);

type End = "top" | "bottom" | "left" | "right";

/** A bar: 4px rounded at its data end, square at the baseline. */
function barPath(x: number, y: number, w: number, h: number, end: End): string {
  const r = Math.min(BAR_RADIUS, w / 2, h / 2);
  const [X, Y, R, W, H] = [x, y, r, x + w, y + h].map((n) => Number(n.toFixed(2)));
  if (end === "top") return `M${X} ${H}V${Y + R}Q${X} ${Y} ${X + R} ${Y}H${W - R}Q${W} ${Y} ${W} ${Y + R}V${H}Z`;
  if (end === "bottom") return `M${X} ${Y}V${H - R}Q${X} ${H} ${X + R} ${H}H${W - R}Q${W} ${H} ${W} ${H - R}V${Y}Z`;
  if (end === "right") return `M${X} ${Y}H${W - R}Q${W} ${Y} ${W} ${Y + R}V${H - R}Q${W} ${H} ${W - R} ${H}H${X}Z`;
  return `M${W} ${Y}H${X + R}Q${X} ${Y} ${X} ${Y + R}V${H - R}Q${X} ${H} ${X + R} ${H}H${W}Z`;
}

export type ChartTick = { pos: number; label: string };
export type ChartBar = { d: string; series: number; index: number };
export type ChartLine = { series: number; d: string; dots: { x: number; y: number; index: number }[] };
export type ChartPoint = { x: number; y: number; series: number; point: number };
export type ChartText = { x: number; y: number; label: string; anchor: "start" | "middle" | "end" };
export type ChartLayout = {
  width: number;
  height: number;
  plot: { left: number; top: number; right: number; bottom: number };
  horizontal: boolean;
  /** Gridlines across the value axis; on the x axis of a horizontal chart. */
  valueTicks: ChartTick[];
  /** Numbers along x, on a scatter. */
  xTicks: ChartTick[];
  categoryLabels: ChartText[];
  /** Where value zero sits, or null when the axis doesn't reach it. */
  baseline: number | null;
  bars: ChartBar[];
  lines: ChartLine[];
  points: ChartPoint[];
  labels: ChartText[];
  /** How many positions a pointer or arrow key can land on. */
  positions: number;
  nearest: (x: number, y: number) => number | null;
  anchor: (position: number) => { x: number; y: number };
};

export function chartLayout(spec: ChartSpec, width: number, tall = false): ChartLayout {
  const narrow = width < LABEL_MIN_WIDTH;
  const n = spec.categories.length;
  const m = spec.series.length;
  const unit = spec.y.unit;
  const compact = (v: number) => withUnit(compactNumber(v), unit);
  const numbers = spec.series.flatMap((s) => (spec.type === "scatter" ? s.points.map((p) => p[1]) : s.values.filter((v): v is number => v !== null)));
  const low = spec.type === "bar" ? Math.min(0, ...numbers) : Math.min(...numbers);
  const high = spec.type === "bar" ? Math.max(0, ...numbers) : Math.max(...numbers);
  const ticks = niceTicks(low, high, narrow ? 4 : 5);
  const [d0, d1] = [ticks[0], ticks[ticks.length - 1]];
  const portion = (v: number) => (v - d0) / (d1 - d0);
  const tickLabels = ticks.map(compact);
  const longest = Math.max(...spec.categories.map(textWidth), 0);
  const singleValues = m === 1 && spec.type === "bar" && n <= 12;
  const valueWidth = Math.max(...numbers.map((v) => textWidth(compact(v))), 0);
  const horizontal = spec.type === "bar" && longest + 10 > (width - 60) / n;
  const base = { width, valueTicks: [] as ChartTick[], xTicks: [] as ChartTick[], categoryLabels: [] as ChartText[], baseline: null as number | null, bars: [] as ChartBar[], lines: [] as ChartLine[], points: [] as ChartPoint[], labels: [] as ChartText[] };

  if (horizontal) {
    const row = Math.max(26, m * BAR_ROW_MAX + (m - 1) * BAR_GAP + 8);
    const left = Math.min(longest + 12, width * 0.42);
    const right = 12 + (singleValues ? valueWidth : 0);
    const top = 6;
    const plotH = row * n;
    const bottom = top + plotH;
    const plotRight = width - right;
    const sx = (v: number) => left + (plotRight - left) * portion(v);
    const zero = sx(0);
    const group = Math.min(row * 0.76, m * BAR_ROW_MAX + (m - 1) * BAR_GAP);
    const thick = (group - (m - 1) * BAR_GAP) / m;
    const bars: ChartBar[] = [];
    const labels: ChartText[] = [];
    spec.series.forEach((s, si) =>
      s.values.forEach((v, i) => {
        if (v === null || v === 0) return;
        const y = top + row * (i + 0.5) - group / 2 + si * (thick + BAR_GAP);
        const x = Math.min(zero, sx(v));
        bars.push({ d: barPath(x, y, Math.abs(sx(v) - zero), thick, v > 0 ? "right" : "left"), series: si, index: i });
        if (singleValues) labels.push({ x: sx(v) + (v > 0 ? 4 : -4), y: y + thick / 2 + 4, label: compact(v), anchor: v > 0 ? "start" : "end" });
      }),
    );
    const maxChars = Math.floor((left - 12) / 6.2);
    return {
      ...base, height: bottom + 24, plot: { left, top, right: plotRight, bottom }, horizontal,
      valueTicks: ticks.map((v, i) => ({ pos: sx(v), label: tickLabels[i] })),
      categoryLabels: spec.categories.map((c, i) => ({ x: left - 8, y: top + row * (i + 0.5) + 4, label: clip(c, maxChars), anchor: "end" })),
      baseline: zero, bars, labels, positions: n,
      nearest: (_x, y) => (y < top || y > bottom ? null : Math.min(n - 1, Math.max(0, Math.floor((y - top) / row)))),
      anchor: (p) => ({ x: left + (plotRight - left) / 2, y: top + row * (p + 0.5) }),
    };
  }

  const height = tall ? 440 : narrow ? 220 : 280;
  const left = Math.max(...tickLabels.map(textWidth)) + 10;
  const top = 12;
  const bottom = height - 24;

  if (spec.type === "scatter") {
    const xs = spec.series.flatMap((s) => s.points.map((p) => p[0]));
    const xt = niceTicks(Math.min(...xs), Math.max(...xs), narrow ? 4 : 6);
    const [x0, x1] = [xt[0], xt[xt.length - 1]];
    const right = width - 16;
    const sx = (v: number) => left + ((right - left) * (v - x0)) / (x1 - x0);
    const sy = (v: number) => bottom - (bottom - top) * portion(v);
    const points = spec.series.flatMap((s, series) => s.points.map((p, point) => ({ x: sx(p[0]), y: sy(p[1]), series, point })));
    return {
      ...base, height, plot: { left, top, right, bottom }, horizontal,
      valueTicks: ticks.map((v, i) => ({ pos: sy(v), label: tickLabels[i] })),
      xTicks: xt.map((v) => ({ pos: sx(v), label: compactNumber(v) })),
      points, positions: points.length,
      nearest: (x, y) => {
        let best: number | null = null;
        let bestD = SCATTER_HIT_PX ** 2;
        points.forEach((p, i) => {
          const d = (p.x - x) ** 2 + (p.y - y) ** 2;
          if (d < bestD) [best, bestD] = [i, d];
        });
        return best;
      },
      anchor: (p) => ({ x: points[p].x, y: points[p].y }),
    };
  }

  // Bars and lines share one category axis, with a point at each band's centre.
  const endNames = spec.type === "line" && m >= 2 && m <= 4 && !narrow && spec.series.every((s) => s.name.length <= 16);
  const endValues = spec.type === "line" && m === 1 && !narrow;
  const endWidth = endNames ? Math.max(...spec.series.map((s) => textWidth(s.name))) : endValues ? valueWidth : 0;
  const right = width - 12 - (endWidth ? endWidth + 6 : 0);
  const band = (right - left) / n;
  const center = (i: number) => left + band * (i + 0.5);
  const sy = (v: number) => bottom - (bottom - top) * portion(v);
  const every = Math.max(1, Math.ceil((Math.min(longest, 96) + 10) / band));
  const maxChars = Math.max(4, Math.floor((band * every - 8) / 6.2));
  const shared = {
    ...base, height, plot: { left, top, right, bottom }, horizontal,
    valueTicks: ticks.map((v, i) => ({ pos: sy(v), label: tickLabels[i] })),
    categoryLabels: spec.categories.flatMap((c, i): ChartText[] => (i % every ? [] : [{ x: center(i), y: bottom + 16, label: clip(c, maxChars), anchor: "middle" }])),
    positions: n,
    nearest: (x: number) => (x < left || x > right ? null : Math.min(n - 1, Math.max(0, Math.floor((x - left) / band)))),
  };

  if (spec.type === "bar") {
    const zero = sy(0);
    const group = Math.min(band * 0.76, m * BAR_MAX + (m - 1) * BAR_GAP);
    const thick = Math.max(2, (group - (m - 1) * BAR_GAP) / m);
    const bars: ChartBar[] = [];
    const labels: ChartText[] = [];
    const fit = singleValues && valueWidth + 2 <= band;
    spec.series.forEach((s, si) =>
      s.values.forEach((v, i) => {
        if (v === null || v === 0) return;
        const x = center(i) - group / 2 + si * (thick + BAR_GAP);
        bars.push({ d: barPath(x, Math.min(zero, sy(v)), thick, Math.abs(sy(v) - zero), v > 0 ? "top" : "bottom"), series: si, index: i });
        if (fit) labels.push({ x: x + thick / 2, y: v > 0 ? sy(v) - 5 : sy(v) + 13, label: compact(v), anchor: "middle" });
      }),
    );
    return { ...shared, baseline: zero, bars, labels, anchor: (p) => ({ x: center(p), y: Math.min(...spec.series.map((s) => sy(Math.max(0, s.values[p] ?? 0)))) }) };
  }

  const lines = spec.series.map((s, series): ChartLine => {
    let d = "";
    let pen = false;
    const dots: ChartLine["dots"] = [];
    s.values.forEach((v, index) => {
      if (v === null) {
        pen = false;
        return;
      }
      const [x, y] = [center(index), sy(v)].map((c) => Number(c.toFixed(2)));
      d += `${pen ? "L" : "M"}${x} ${y}`;
      pen = true;
      dots.push({ x, y, index });
    });
    return { series, d, dots: n <= MARKER_MAX_POINTS ? dots : dots.slice(-1) };
  });
  const ends = spec.series.flatMap((s, si) => {
    let last = s.values.length - 1;
    while (last >= 0 && s.values[last] === null) last--;
    return last < 0 ? [] : [{ x: center(last), y: sy(s.values[last]!), label: endValues ? compact(s.values[last]!) : s.name, si }];
  });
  // Nudging colliding labels detaches them from their lines: the legend covers those.
  const ys = ends.map((e) => e.y).sort((a, b) => a - b);
  const collide = ys.some((y, i) => i > 0 && y - ys[i - 1] < 13);
  const labels: ChartText[] = endWidth && !collide ? ends.map((e) => ({ x: e.x + 8, y: e.y + 4, label: e.label, anchor: "start" })) : [];
  return { ...shared, baseline: d0 <= 0 && d1 >= 0 ? sy(0) : null, lines, labels, anchor: (p) => ({ x: center(p), y: Math.min(...spec.series.map((s) => sy(s.values[p] ?? d1))) }) };
}
