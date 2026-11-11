/**
 * Shared slide layout. One function turns a slide into a display list in
 * slide units; the SVG renderer (editor, thumbnails, review, presentation)
 * and the PDF export both draw it, and the PPTX export reuses the constants,
 * so all of them break lines in the same places.
 */
import { type Seg, isPreset, shapeGeometry } from "./geometry.ts";
import { type Deck, type Element, type Paragraph, type Run, type Slide, type Slot, type TextBox, type Theme, resolveColor, slotOf, textOf } from "./schema.ts";

export const INSET = 16; // text inset on every side, in slide units (8 pt)
export const LINE = 1.2; // line height as a multiple of the paragraph's largest run, times its lineSpacing
export const BULLET_INDENT = 60; // hanging indent per bullet level, in slide units (30 pt)

/** A paragraph's line height in slide units: LINE × its largest run × its line spacing. */
export const lineHeight = (p: Paragraph) => Math.max(...p.runs.map((r) => r.size)) * LINE * (p.lineSpacing || 1);

/** Strikeout position above the baseline, as a fraction of the font size (Liberation OS/2 yStrikeoutPosition). */
export const STRIKE = 0.26;

/** Ascent and descent as fractions of the font size (Liberation 2.1.5, hhea). */
export const METRICS: Record<Slot, { asc: number; desc: number }> = {
  sans: { asc: 0.9053, desc: 0.2119 },
  serif: { asc: 0.8911, desc: 0.2163 },
  mono: { asc: 0.8325, desc: 0.3003 },
};

export type Style = "normal" | "bold" | "italic" | "bolditalic";
export const styleOf = (r: Pick<Run, "bold" | "italic">): Style => (r.bold && r.italic ? "bolditalic" : r.bold ? "bold" : r.italic ? "italic" : "normal");

/** Measures text width in slide units at a size; see fonts.ts for the browser one. */
export type Measure = (text: string, slot: Slot, style: Style, size: number) => number;

type Rot = { deg: number; cx: number; cy: number };
export type TextOp = { op: "text"; x: number; y: number; text: string; slot: Slot; style: Style; size: number; color: string; underline: boolean; strike: boolean; w: number; rot?: Rot; el: string };
/** A highlight behind a run of text; drawn before the text it marks. */
export type HighlightOp = { op: "highlight"; x: number; y: number; w: number; h: number; fill: string; rot?: Rot; el: string };
export type Op =
  | { op: "rect"; x: number; y: number; w: number; h: number; r: number; fill?: string; stroke?: string; sw: number; rot?: Rot; el?: string }
  | { op: "ellipse"; x: number; y: number; w: number; h: number; fill?: string; stroke?: string; sw: number; rot?: Rot; el: string }
  | { op: "path"; segs: Seg[]; fill?: string; stroke?: string; sw: number; rot?: Rot; el: string }
  | { op: "line"; x1: number; y1: number; x2: number; y2: number; stroke: string; sw: number; endArrow: boolean; startArrow: boolean; rot?: Rot; el: string }
  | { op: "image"; asset: string; x: number; y: number; w: number; h: number; src: { x: number; y: number; w: number; h: number }; alt: string; rot?: Rot; el: string }
  | HighlightOp
  | TextOp;

export type Laid = { ops: Op[]; overflow: Set<string> };

type Piece = { run: Run; slot: Slot; text: string; x: number; w: number };
type Line = { y: number; lh: number; maxSize: number; left: number; pieces: Piece[]; align: NonNullable<Paragraph["align"]>; last: boolean; bullet?: { glyph: string; x: number; run: Run; slot: Slot } };

/**
 * Greedy word wrap over styled runs. Reports overflow; never shrinks text.
 * Justified lines spread their spaces to fill the line, except a paragraph's
 * last line (and a line broken inside one long word), which stay left-aligned.
 */
export function layoutText(theme: Theme, box: TextBox, frame: Element["frame"], measure: Measure, el: string): { ops: (TextOp | HighlightOp)[]; overflow: boolean; height: number } {
  const innerW = frame.w - 2 * INSET;
  const lines: Line[] = [];
  let y = 0;
  let number = 0;
  for (const p of box.paragraphs) {
    const maxSize = Math.max(...p.runs.map((r) => r.size));
    const lh = lineHeight(p);
    const level = p.level ?? 0;
    const left = p.bullet ? BULLET_INDENT * (level + 1) : 0;
    number = p.bullet === "number" ? number + 1 : 0;
    y += p.spaceBefore ?? 0;
    const tokens: { run: Run; slot: Slot; text: string }[] = [];
    for (const r of p.runs) for (const t of r.text.split(/(?<= )/)) if (t) tokens.push({ run: r, slot: slotOf(theme, r.font), text: t });
    const first = lines.length;
    let cur: Piece[] = [];
    let w = 0;
    const flush = () => {
      lines.push({ y, lh, maxSize, left, pieces: cur, align: p.align ?? "left", last: false });
      cur = [];
      w = 0;
      y += lh;
    };
    for (const t of tokens) {
      const style = styleOf(t.run);
      const tw = measure(t.text, t.slot, style, t.run.size);
      const trimmed = measure(t.text.trimEnd(), t.slot, style, t.run.size);
      if (cur.length && w + trimmed > innerW - left) flush();
      // A word longer than the line breaks between characters.
      if (!cur.length && trimmed > innerW - left) {
        let chunk = "";
        for (const ch of t.text) {
          if (chunk && measure(chunk + ch, t.slot, style, t.run.size) > innerW - left) {
            cur.push({ ...t, text: chunk, x: 0, w: measure(chunk, t.slot, style, t.run.size) });
            flush();
            chunk = "";
          }
          chunk += ch;
        }
        cur.push({ ...t, text: chunk, x: 0, w: measure(chunk, t.slot, style, t.run.size) });
        w = cur[0].w;
        continue;
      }
      cur.push({ ...t, x: w, w: tw });
      w += tw;
    }
    flush();
    lines[lines.length - 1].last = true;
    if (p.bullet) {
      const r = p.runs[0];
      lines[first].bullet = { glyph: p.bullet === "number" ? `${number}.` : "•", x: BULLET_INDENT * level, run: r, slot: slotOf(theme, r.font) };
    }
  }
  const height = y;
  const avail = frame.h - 2 * INSET;
  const shift = box.valign === "middle" ? (avail - height) / 2 : box.valign === "bottom" ? avail - height : 0;
  const highlights: HighlightOp[] = [];
  const ops: TextOp[] = [];
  for (const l of lines) {
    const m = METRICS[l.pieces[0]?.slot ?? l.bullet?.slot ?? "sans"];
    // Extra line spacing splits evenly above and below the text, like CSS half-leading.
    const baseline = frame.y + INSET + shift + l.y + (l.lh - (m.asc + m.desc) * l.maxSize) / 2 + m.asc * l.maxSize;
    const last = l.pieces.at(-1);
    const used = last ? last.x + measure(last.text.trimEnd(), last.slot, styleOf(last.run), last.run.size) : 0;
    const room = innerW - l.left - used;
    const dx = l.align === "center" ? room / 2 : l.align === "right" ? room : 0;
    // Justify: the room goes to the spaces between pieces (a piece ending in a space is a word end).
    const gaps = l.align === "justify" && !l.last && room > 0 ? l.pieces.slice(0, -1).filter((pc) => pc.text.endsWith(" ")).length : 0;
    const per = gaps ? room / gaps : 0;
    const placed: { pc: Piece; x: number; w: number }[] = [];
    let spread = 0;
    for (const pc of l.pieces) {
      const x = frame.x + INSET + l.left + dx + pc.x + spread;
      if (per && pc.text.endsWith(" ")) spread += per;
      if (pc.text) placed.push({ pc, x, w: measure(pc.text.trimEnd(), pc.slot, styleOf(pc.run), pc.run.size) });
    }
    if (l.bullet && l.pieces.some((p) => p.text.trim()))
      ops.push({ op: "text", x: frame.x + INSET + l.bullet.x, y: baseline, text: l.bullet.glyph, slot: l.bullet.slot, style: "normal", size: l.bullet.run.size, color: resolveColor(theme, l.bullet.run.color), underline: false, strike: false, w: 0, el });
    placed.forEach(({ pc, x, w }, i) => {
      ops.push({ op: "text", x, y: baseline, text: pc.text, slot: pc.slot, style: styleOf(pc.run), size: pc.run.size, color: resolveColor(theme, pc.run.color), underline: !!pc.run.underline, strike: !!pc.run.strike, w, el });
      if (!pc.run.highlight) return;
      // A highlight runs on through the space to the next piece when that piece is highlighted in the same colour.
      const fill = resolveColor(theme, pc.run.highlight);
      const next = placed[i + 1];
      const end = next?.pc.run.highlight && resolveColor(theme, next.pc.run.highlight) === fill ? next.x : x + w;
      const pm = METRICS[pc.slot];
      highlights.push({ op: "highlight", x, y: baseline - pm.asc * pc.run.size, w: end - x, h: (pm.asc + pm.desc) * pc.run.size, fill, el });
    });
  }
  return { ops: [...highlights, ...ops], overflow: height > avail + 0.5, height };
}

/** Where an image's source pixels land: the crop, then centre-trimmed to cover the frame. */
export function imageSource(e: Extract<Element, { type: "image" }>, a: { w: number; h: number }) {
  const c = { l: 0, t: 0, r: 0, b: 0, ...e.crop };
  let x = c.l * a.w, y = c.t * a.h, w = (1 - c.l - c.r) * a.w, h = (1 - c.t - c.b) * a.h;
  const fa = e.frame.w / e.frame.h;
  if (w / h > fa) {
    const nw = h * fa;
    x += (w - nw) / 2;
    w = nw;
  } else {
    const nh = w / fa;
    y += (h - nh) / 2;
    h = nh;
  }
  return { x, y, w, h };
}

/** The box an element's text wraps in: a preset shape's text rectangle, else the frame. */
export function textFrame(e: Element): Element["frame"] {
  return e.type === "shape" && isPreset(e.shape) ? shapeGeometry(e.shape, e.frame).text : e.frame;
}

/** Everything a slide draws, in paint order, in slide units. */
export function layoutSlide(deck: Deck, slide: Slide, measure: Measure): Laid {
  const { theme } = deck;
  const ops: Op[] = [{ op: "rect", x: 0, y: 0, w: deck.size.w, h: deck.size.h, r: 0, fill: resolveColor(theme, slide.background?.color, "background"), sw: 0 }];
  const overflow = new Set<string>();
  for (const e of slide.elements) {
    const f = e.frame;
    const rot = e.rotation ? { deg: e.rotation, cx: f.x + f.w / 2, cy: f.y + f.h / 2 } : undefined;
    const group: Op[] = [];
    if (e.type === "shape") {
      const paint = { fill: e.fill ? resolveColor(theme, e.fill) : undefined, stroke: e.stroke ? resolveColor(theme, e.stroke) : undefined, sw: e.strokeWidth ?? 0 };
      if (isPreset(e.shape)) group.push({ op: "path", segs: shapeGeometry(e.shape, f).segs, ...paint, el: e.id });
      else if (e.shape === "ellipse") group.push({ op: "ellipse", x: f.x, y: f.y, w: f.w, h: f.h, ...paint, el: e.id });
      else group.push({ op: "rect", x: f.x, y: f.y, w: f.w, h: f.h, r: e.shape === "roundRect" ? Math.min(e.radius ?? 24, f.w / 2, f.h / 2) : 0, ...paint, el: e.id });
    }
    if (e.type === "line") group.push({ op: "line", x1: f.x, y1: f.y, x2: f.x + f.w, y2: f.y + f.h, stroke: resolveColor(theme, e.stroke), sw: e.strokeWidth, endArrow: !!e.endArrow, startArrow: !!e.startArrow, el: e.id });
    if (e.type === "image") {
      const a = deck.assets[e.asset];
      if (a) group.push({ op: "image", asset: e.asset, x: f.x, y: f.y, w: f.w, h: f.h, src: imageSource(e, a), alt: e.alt ?? a.alt ?? "", el: e.id });
      else group.push({ op: "rect", x: f.x, y: f.y, w: f.w, h: f.h, r: 0, fill: "#E5E7EB", sw: 0, el: e.id });
    }
    const box = textOf(e);
    if (box) {
      const t = layoutText(theme, box, textFrame(e), measure, e.id);
      if (t.overflow) overflow.add(e.id);
      group.push(...t.ops);
    }
    ops.push(...group.map((o) => (rot ? { ...o, rot } : o)));
  }
  return { ops, overflow };
}
