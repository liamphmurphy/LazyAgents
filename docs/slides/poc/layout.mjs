// Shared slide layout for the LAZYAGENTS-103 proof of concept.
//
// One function turns a deck slide into a display list in slide units
// (1920 × 1080). The SVG preview (editor and presentation) and the PDF export
// draw that list; the PPTX export maps the same elements to native shapes
// and reuses only the constants below (insets, line height, bullet indent)
// so PowerPoint wraps the text the way we did.
import opentype from "opentype.js";
import { readFileSync } from "node:fs";

export const FONT_DIR = "/usr/share/fonts/truetype/liberation";

// A theme font is a slot, never a free-form family: each slot has the face we
// ship (and embed in PDF) and the metric-compatible face PPTX names, so
// PowerPoint wraps lines where we did even without our fonts installed.
export const FAMILIES = {
  sans: { file: "LiberationSans", web: "Liberation Sans", office: "Arial" },
  serif: { file: "LiberationSerif", web: "Liberation Serif", office: "Times New Roman" },
  mono: { file: "LiberationMono", web: "Liberation Mono", office: "Courier New" },
};
const STYLE_FILE = { normal: "Regular", bold: "Bold", italic: "Italic", bolditalic: "BoldItalic" };

export const INSET = 16; // text inset on every side, slide units (= 8 pt)
export const LINE = 1.2; // line height as a multiple of the paragraph's largest run
export const BULLET_INDENT = 60; // hanging indent per bullet level, slide units (= 30 pt)

const cache = new Map();
export function fontFile(family, style) {
  return `${FONT_DIR}/${FAMILIES[family].file}-${STYLE_FILE[style]}.ttf`;
}
export function font(family, style) {
  const k = `${family}/${style}`;
  if (!cache.has(k)) {
    const b = readFileSync(fontFile(family, style));
    cache.set(k, opentype.parse(b.buffer.slice(b.byteOffset, b.byteOffset + b.byteLength)));
  }
  return cache.get(k);
}
export const styleOf = (r) => (r.bold && r.italic ? "bolditalic" : r.bold ? "bold" : r.italic ? "italic" : "normal");

/** A colour token from the theme, or a literal #RRGGBB. */
export const color = (theme, c, fallback = "text") => {
  const v = c ?? fallback;
  return v.startsWith("#") ? v.toUpperCase() : theme.colors[v];
};

function familyOf(theme, run) {
  const f = run.font ?? "body";
  return f === "heading" || f === "body" ? theme.fonts[f] : f;
}

const measure = (run, family, text) => {
  const f = font(family, styleOf(run));
  return (f.getAdvanceWidth(text, run.size, { kerning: false }));
};

/**
 * Greedy word wrap over styled runs. Returns lines of positioned pieces and
 * whether the text overflows the frame (we report overflow; we never shrink).
 */
export function layoutText(theme, box, frame) {
  const out = [];
  const innerW = frame.w - 2 * INSET;
  let y = 0;
  const lines = [];
  let number = 0;
  for (const p of box.paragraphs) {
    const maxSize = Math.max(...p.runs.map((r) => r.size));
    const lh = maxSize * LINE;
    const level = p.level ?? 0;
    const left = p.bullet ? BULLET_INDENT * (level + 1) : 0;
    number = p.bullet === "number" ? number + 1 : 0;
    y += p.spaceBefore ?? 0;
    // tokens: words and the spaces after them, each carrying its run
    const tokens = [];
    for (const r of p.runs) for (const t of r.text.split(/(?<= )/)) if (t) tokens.push({ run: r, text: t, family: familyOf(theme, r) });
    let cur = [];
    let w = 0;
    const flush = () => {
      lines.push({ y, lh, maxSize, left, pieces: cur, width: w, align: p.align ?? "left", bullet: null });
      cur = [];
      w = 0;
      y += lh;
    };
    const first = lines.length;
    for (const t of tokens) {
      const tw = measure(t.run, t.family, t.text);
      const trimmed = measure(t.run, t.family, t.text.trimEnd());
      if (cur.length && w + trimmed > innerW - left) flush();
      cur.push({ ...t, x: w, w: tw });
      w += tw;
    }
    flush();
    if (p.bullet) lines[first].bullet = { glyph: p.bullet === "number" ? `${number}.` : "•", x: BULLET_INDENT * level, run: p.runs[0], family: familyOf(theme, p.runs[0]) };
  }
  const height = y;
  const avail = frame.h - 2 * INSET;
  const shift = box.valign === "middle" ? (avail - height) / 2 : box.valign === "bottom" ? avail - height : 0;
  for (const l of lines) {
    const asc = font(l.pieces[0]?.family ?? "sans", "normal").ascender / font(l.pieces[0]?.family ?? "sans", "normal").unitsPerEm;
    const desc = -font(l.pieces[0]?.family ?? "sans", "normal").descender / font(l.pieces[0]?.family ?? "sans", "normal").unitsPerEm;
    const baseline = frame.y + INSET + shift + l.y + (l.lh - (asc + desc) * l.maxSize) / 2 + asc * l.maxSize;
    // trailing spaces don't count when aligning
    const last = l.pieces.at(-1);
    const used = last ? last.x + measure(last.run, last.family, last.text.trimEnd()) : 0;
    const room = innerW - l.left - used;
    const dx = l.align === "center" ? room / 2 : l.align === "right" ? room : 0;
    if (l.bullet) out.push({ op: "text", x: frame.x + INSET + l.bullet.x, y: baseline, text: l.bullet.glyph, family: l.bullet.family, style: "normal", size: l.bullet.run.size, color: color(theme, l.bullet.run.color) });
    for (const pc of l.pieces) {
      out.push({ op: "text", x: frame.x + INSET + l.left + dx + pc.x, y: baseline, text: pc.text, family: pc.family, style: styleOf(pc.run), size: pc.run.size, color: color(theme, pc.run.color), underline: !!pc.run.underline, w: pc.w });
    }
  }
  return { ops: out, overflow: height > avail + 0.5, height };
}

/** Everything a slide draws, in paint order, in slide units. */
export function layoutSlide(deck, slide) {
  const { theme } = deck;
  const ops = [{ op: "rect", x: 0, y: 0, w: deck.size.w, h: deck.size.h, fill: color(theme, slide.background?.color, "background") }];
  const warnings = [];
  for (const e of slide.elements) {
    const f = e.frame;
    const rot = e.rotation ? { deg: e.rotation, cx: f.x + f.w / 2, cy: f.y + f.h / 2 } : undefined;
    const group = [];
    if (e.type === "shape") {
      group.push({ op: e.shape === "ellipse" ? "ellipse" : "rect", x: f.x, y: f.y, w: f.w, h: f.h, r: e.shape === "roundRect" ? e.radius ?? 24 : 0,
        fill: e.fill && color(theme, e.fill), stroke: e.stroke && color(theme, e.stroke), sw: e.strokeWidth ?? 0 });
    }
    if (e.type === "line") group.push({ op: "line", x1: f.x, y1: f.y, x2: f.x + f.w, y2: f.y + f.h, stroke: color(theme, e.stroke), sw: e.strokeWidth, arrow: !!e.endArrow });
    if (e.type === "image") {
      const a = deck.assets[e.asset];
      const c = { l: 0, t: 0, r: 0, b: 0, ...e.crop };
      let sx = c.l * a.w, sy = c.t * a.h, sw = (1 - c.l - c.r) * a.w, sh = (1 - c.t - c.b) * a.h;
      // cover: centre-trim whatever is left of the crop to the frame's aspect
      const fa = f.w / f.h;
      if (sw / sh > fa) { const nw = sh * fa; sx += (sw - nw) / 2; sw = nw; } else { const nh = sw / fa; sy += (sh - nh) / 2; sh = nh; }
      group.push({ op: "image", asset: e.asset, x: f.x, y: f.y, w: f.w, h: f.h, src: { x: sx, y: sy, w: sw, h: sh }, alt: a.alt });
      e._src = { x: sx, y: sy, w: sw, h: sh }; // the PPTX export crops the same way
    }
    const box = e.type === "text" ? e : e.text;
    if (box) {
      const t = layoutText(theme, box, f);
      if (t.overflow) warnings.push(`${slide.id}/${e.id}: text overflows its box by ${Math.round(t.height - (f.h - 2 * INSET))} units`);
      group.push(...t.ops);
    }
    ops.push(...group.map((o) => (rot ? { ...o, rot } : o)));
  }
  return { ops, warnings };
}
