/**
 * Pure geometry for the mockup review canvas: the drawing's size, fit and
 * 100% views, zooming about a point, and pin positions. No DOM access.
 */

export type Size = { w: number; h: number };
/** Where the drawing sits in the viewport: its top-left corner (px) and scale. */
export type View = { x: number; y: number; scale: number };
/** A point on the drawing as fractions of its width and height (0..1). */
export type Fraction = { x: number; y: number };

export const MIN_SCALE = 0.05;
export const MAX_SCALE = 8;
/** Gap kept around the drawing at Fit, in px. */
export const FIT_PAD = 32;
/** Zoom step for buttons and +/− keys. */
export const ZOOM_STEP = 1.25;
/** Arrow-key pan distance in px (Shift multiplies it by 5). */
export const PAN_STEP = 48;

const DEFAULT_SIZE: Size = { w: 1280, h: 800 };

function length(v: string | undefined): number {
  // Unitless or px lengths only; %, em and friends fall back to the viewBox.
  return v && /^\s*[\d.]+\s*(px)?\s*$/.test(v) ? parseFloat(v) : NaN;
}

/**
 * The drawing's intrinsic size from the root <svg>'s width/height, else its
 * viewBox, else 1280×800. Read from source so it's known before the image loads.
 */
export function svgSize(source: string): Size {
  const tag = /<svg\b[^>]*>/i.exec(source)?.[0];
  if (!tag) return DEFAULT_SIZE;
  const attr = (name: string) => new RegExp(`\\s${name}\\s*=\\s*("([^"]*)"|'([^']*)')`, "i").exec(tag)?.slice(2).find((s) => s !== undefined);
  const vb = attr("viewBox")?.trim().split(/[\s,]+/).map(Number);
  const vbOk = !!vb && vb.length === 4 && vb.every(Number.isFinite) && vb[2] > 0 && vb[3] > 0;
  let w = length(attr("width"));
  let h = length(attr("height"));
  if (!(w > 0) && !(h > 0)) {
    if (vbOk) return { w: vb![2], h: vb![3] };
    return DEFAULT_SIZE;
  }
  // One side given: keep the viewBox's aspect ratio.
  if (!(w > 0)) w = vbOk ? (h * vb![2]) / vb![3] : DEFAULT_SIZE.w;
  if (!(h > 0)) h = vbOk ? (w * vb![3]) / vb![2] : DEFAULT_SIZE.h;
  return { w, h };
}

export function clampScale(s: number): number {
  return Math.min(MAX_SCALE, Math.max(MIN_SCALE, s));
}

/** The whole drawing, centred, as large as fits inside the viewport's padding. */
export function fitView(drawing: Size, viewport: Size, pad = FIT_PAD): View {
  const scale = clampScale(Math.min((viewport.w - 2 * pad) / drawing.w, (viewport.h - 2 * pad) / drawing.h));
  return { scale, x: (viewport.w - drawing.w * scale) / 2, y: (viewport.h - drawing.h * scale) / 2 };
}

/** 100%: centred when it fits, else its top-left corner at the padding. */
export function actualView(drawing: Size, viewport: Size, pad = FIT_PAD): View {
  const at = (d: number, v: number) => (d <= v - 2 * pad ? (v - d) / 2 : pad);
  return { scale: 1, x: at(drawing.w, viewport.w), y: at(drawing.h, viewport.h) };
}

/** Scale by `factor`, keeping the viewport point (px, py) over the same spot of the drawing. */
export function zoomAt(view: View, factor: number, px: number, py: number): View {
  const scale = clampScale(view.scale * factor);
  const k = scale / view.scale;
  return { scale, x: px - (px - view.x) * k, y: py - (py - view.y) * k };
}

export function panBy(view: View, dx: number, dy: number): View {
  return { ...view, x: view.x + dx, y: view.y + dy };
}

/** The drawing fraction under viewport point (px, py), or null when it's off the drawing. */
export function toFraction(view: View, drawing: Size, px: number, py: number): Fraction | null {
  const x = (px - view.x) / (drawing.w * view.scale);
  const y = (py - view.y) / (drawing.h * view.scale);
  return x >= 0 && x <= 1 && y >= 0 && y <= 1 ? { x, y } : null;
}

/** Viewport position (px) of a point on the drawing. */
export function fromFraction(view: View, drawing: Size, f: Fraction): { left: number; top: number } {
  return { left: view.x + f.x * drawing.w * view.scale, top: view.y + f.y * drawing.h * view.scale };
}

/** Swipe handle position (0..1 of the viewport width) after a key press, or null if the key doesn't move it. */
export function swipeKey(pos: number, key: string, shift = false): number | null {
  const step = shift ? 0.1 : 0.02;
  const clamp = (v: number) => Math.min(1, Math.max(0, v));
  switch (key) {
    case "ArrowLeft":
      return clamp(pos - step);
    case "ArrowRight":
      return clamp(pos + step);
    case "Home":
      return 0;
    case "End":
      return 1;
    default:
      return null;
  }
}
