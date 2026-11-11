/**
 * Arranging a slide's elements: z-order, duplicating, aligning,
 * distributing, group resizing and snap guides. Pure functions over slide
 * units; the editor wires them to the selection. Stacking order is the
 * slide's `elements` array (first is drawn first, so it's at the back).
 *
 * Boxes are the unrotated frames, normalised so lines with a negative width
 * or height still have l ≤ r and t ≤ b.
 */
import type { Element, Frame } from "./schema";

export type Box = { l: number; t: number; r: number; b: number };
export type ZOrder = "forward" | "backward" | "front" | "back";
export type AlignOp = "left" | "center" | "right" | "top" | "middle" | "bottom";

export const boxOf = (f: Frame): Box => ({ l: Math.min(f.x, f.x + f.w), t: Math.min(f.y, f.y + f.h), r: Math.max(f.x, f.x + f.w), b: Math.max(f.y, f.y + f.h) });

export function unionBox(frames: Frame[]): Box | undefined {
  if (!frames.length) return undefined;
  const bs = frames.map(boxOf);
  return { l: Math.min(...bs.map((b) => b.l)), t: Math.min(...bs.map((b) => b.t)), r: Math.max(...bs.map((b) => b.r)), b: Math.max(...bs.map((b) => b.b)) };
}

export const frameOf = (b: Box): Frame => ({ x: b.l, y: b.t, w: b.r - b.l, h: b.b - b.t });

/** Whether `inner` lies wholly inside `outer`. */
export const contains = (outer: Box, inner: Box) => inner.l >= outer.l && inner.r <= outer.r && inner.t >= outer.t && inner.b <= outer.b;

/** The elements reordered for a z-order command on the ids; the same array when nothing moves. */
export function reorder(elements: Element[], ids: ReadonlySet<string>, op: ZOrder): Element[] {
  const sel = (e: Element) => ids.has(e.id);
  let out: Element[];
  if (op === "front") out = [...elements.filter((e) => !sel(e)), ...elements.filter(sel)];
  else if (op === "back") out = [...elements.filter(sel), ...elements.filter((e) => !sel(e))];
  else {
    // Each selected element swaps with the unselected neighbour above (or
    // below) it, so a selection moves one step as a block and keeps its order.
    out = [...elements];
    if (op === "forward") {
      for (let i = out.length - 2; i >= 0; i--) if (sel(out[i]) && !sel(out[i + 1])) [out[i], out[i + 1]] = [out[i + 1], out[i]];
    } else {
      for (let i = 1; i < out.length; i++) if (sel(out[i]) && !sel(out[i - 1])) [out[i], out[i - 1]] = [out[i - 1], out[i]];
    }
  }
  return out.every((e, i) => e === elements[i]) ? elements : out;
}

/**
 * Copies of the ids' elements with fresh ids, offset by `offset`, placed
 * above the topmost original in the originals' order. Copies drop their
 * layout placeholder role: a slide has one title.
 */
export function duplicate(elements: Element[], ids: ReadonlySet<string>, newId: () => string, offset = 40): { elements: Element[]; ids: string[] } {
  const src = elements.filter((e) => ids.has(e.id));
  if (!src.length) return { elements, ids: [] };
  const copies = src.map((e) => ({ ...structuredClone(e), id: newId(), frame: { ...e.frame, x: e.frame.x + offset, y: e.frame.y + offset }, placeholder: undefined }) as Element);
  const top = elements.reduce((at, e, i) => (ids.has(e.id) ? i : at), -1);
  return { elements: [...elements.slice(0, top + 1), ...copies, ...elements.slice(top + 1)], ids: copies.map((c) => c.id) };
}

/** Frames moved so each box lines up with `to` on one side or centre line. */
export function align(frames: Record<string, Frame>, op: AlignOp, to: Box): Record<string, Frame> {
  const out: Record<string, Frame> = {};
  for (const [id, f] of Object.entries(frames)) {
    const b = boxOf(f);
    let dx = 0, dy = 0;
    if (op === "left") dx = to.l - b.l;
    if (op === "right") dx = to.r - b.r;
    if (op === "center") dx = (to.l + to.r) / 2 - (b.l + b.r) / 2;
    if (op === "top") dy = to.t - b.t;
    if (op === "bottom") dy = to.b - b.b;
    if (op === "middle") dy = (to.t + to.b) / 2 - (b.t + b.b) / 2;
    out[id] = { ...f, x: Math.round(f.x + dx), y: Math.round(f.y + dy) };
  }
  return out;
}

/**
 * Equal gaps between three or more boxes along an axis. The box reaching
 * furthest to the start and the one reaching furthest to the end stay put and
 * the rest move between them, in the order of their centres. When the boxes
 * are too big to fit between those two without overlapping (or one box spans
 * them all), their centres are spaced evenly between the outermost centres
 * instead, so no gap goes negative and no centre leaves the selection.
 */
export function distribute(frames: Record<string, Frame>, axis: "x" | "y"): Record<string, Frame> {
  const items = Object.entries(frames).map(([id, f]) => ({ id, f, b: boxOf(f) }));
  if (items.length < 3) return { ...frames };
  const lo = (b: Box) => (axis === "x" ? b.l : b.t);
  const hi = (b: Box) => (axis === "x" ? b.r : b.b);
  const mid = (b: Box) => (lo(b) + hi(b)) / 2;
  const first = items.reduce((m, i) => (lo(i.b) < lo(m.b) ? i : m));
  const last = items.reduce((m, i) => (i !== first && (m === first || hi(i.b) > hi(m.b)) ? i : m));
  const middle = items.filter((i) => i !== first && i !== last).sort((p, q) => mid(p.b) - mid(q.b));
  const gap = (hi(last.b) - lo(first.b) - items.reduce((s, i) => s + hi(i.b) - lo(i.b), 0)) / (items.length - 1);
  const shift: Record<string, number> = {};
  if (hi(last.b) >= hi(first.b) && gap >= 0) {
    let at = hi(first.b) + gap;
    for (const it of middle) {
      shift[it.id] = Math.round(at - lo(it.b));
      at += hi(it.b) - lo(it.b) + gap;
    }
  } else {
    // Even centres between the outermost centres.
    const byMid = [...items].sort((p, q) => mid(p.b) - mid(q.b));
    const c0 = mid(byMid[0].b), step = (mid(byMid[byMid.length - 1].b) - c0) / (byMid.length - 1);
    byMid.forEach((it, k) => (shift[it.id] = k === 0 || k === byMid.length - 1 ? 0 : Math.round(c0 + k * step - mid(it.b))));
  }
  const out: Record<string, Frame> = {};
  for (const it of items) {
    const d = shift[it.id] ?? 0;
    out[it.id] = axis === "x" ? { ...it.f, x: it.f.x + d } : { ...it.f, y: it.f.y + d };
  }
  return out;
}

/** A frame mapped from the box `from` onto the box `to`, scaling its position and size. */
export function scaleFrame(f: Frame, from: Box, to: Box): Frame {
  const sx = from.r > from.l ? (to.r - to.l) / (from.r - from.l) : 1;
  const sy = from.b > from.t ? (to.b - to.t) / (from.b - from.t) : 1;
  return { x: Math.round(to.l + (f.x - from.l) * sx), y: Math.round(to.t + (f.y - from.t) * sy), w: Math.round(f.w * sx), h: Math.round(f.h * sy) };
}

/** Lines things snap to on each axis: slide edges and centre, and other elements' edges and centres. */
export type SnapTargets = { x: number[]; y: number[] };

export function snapTargets(width: number, height: number, others: Frame[]): SnapTargets {
  const x = [0, width / 2, width], y = [0, height / 2, height];
  for (const f of others) {
    const b = boxOf(f);
    x.push(b.l, (b.l + b.r) / 2, b.r);
    y.push(b.t, (b.t + b.b) / 2, b.b);
  }
  return { x, y };
}

/**
 * The smallest shift (within `threshold`) that puts one of `values` on one
 * of `targets`, and the line it snapped to; or none.
 */
export function snapAxis(values: number[], targets: number[], threshold: number): { d: number; at: number } | undefined {
  let best: { d: number; at: number } | undefined;
  for (const v of values)
    for (const t of targets) {
      const d = t - v;
      if (Math.abs(d) <= threshold && (!best || Math.abs(d) < Math.abs(best.d))) best = { d, at: t };
    }
  return best;
}

/**
 * Snapping a box being moved: its edges and centre on each axis. Returns the
 * corrected offset and the guide lines to draw.
 */
export function snapMove(b: Box, targets: SnapTargets, threshold: number): { dx: number; dy: number; guides: { x: number[]; y: number[] } } {
  const sx = snapAxis([b.l, (b.l + b.r) / 2, b.r], targets.x, threshold);
  const sy = snapAxis([b.t, (b.t + b.b) / 2, b.b], targets.y, threshold);
  const moved = { l: b.l + (sx?.d ?? 0), r: b.r + (sx?.d ?? 0), t: b.t + (sy?.d ?? 0), b: b.b + (sy?.d ?? 0) };
  return { dx: sx?.d ?? 0, dy: sy?.d ?? 0, guides: guidesFor(moved, targets) };
}

/** Every target line the box's edges or centre now sit on (to within half a unit). */
export function guidesFor(b: Box, targets: SnapTargets): { x: number[]; y: number[] } {
  const on = (vs: number[], ts: number[]) => [...new Set(ts.filter((t) => vs.some((v) => Math.abs(v - t) < 0.5)))];
  return { x: on([b.l, (b.l + b.r) / 2, b.r], targets.x), y: on([b.t, (b.t + b.b) / 2, b.b], targets.y) };
}
