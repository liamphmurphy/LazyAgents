// Design export (ADR-designs §3.10): what an export covers, SVG with Inter
// embedded and text kept as text, and PDF with one page per item. PNG comes
// from the server renderer (GET /designs/{id}/render), so it isn't drawn here.
// Pure apart from the loaders it is given, so the browser and the comparison
// tests build byte-identical files.
import { find, labelOf } from "./edit.ts";
import { makeLayout, pageOps, type Box, type LaidPage, type Op, type ResolvedPaint } from "./layout.ts";
import { FAMILY } from "./measure.ts";
import type { Design, DesignNode, Page } from "./schema.ts";
import { toSvg } from "./svg.ts";

export type ExportScope = "selection" | "frame" | "page" | "frames";
export type ExportFormat = "png" | "svg" | "pdf";

/** One picture: a node's box (with its top-level node's ops) or a whole page. */
export type ExportItem = {
  /** Used for file names: the node's or page's name. */
  name: string;
  page: string;
  /** Set for a node; absent for a whole page. */
  node?: string;
  box: Box;
  ops: Op[];
  background?: string;
};

export class ExportError extends Error {}

/** Longest edge of a PNG export, the renderer's limit. */
export const MAX_PNG_EDGE = 4096;
/** PDF points per design px (CSS px are 1/96 in, points 1/72 in). */
export const PT = 0.75;

const walk = (nodes: DesignNode[], f: (n: DesignNode) => void) => {
  for (const n of nodes) {
    f(n);
    if ("children" in n && Array.isArray(n.children)) walk(n.children, f);
  }
};

/** The top-level node of a page that holds id. */
function topOf(page: Page, id: string): DesignNode | undefined {
  return page.children.find((t) => {
    let hit = false;
    walk([t], (n) => void (hit ||= n.id === id));
    return hit;
  });
}

/** Lays pages out once per export. */
function laidPages(design: Design) {
  const lay = makeLayout(design);
  const cache = new Map<string, LaidPage>();
  return (p: Page) => {
    let l = cache.get(p.id);
    if (!l) cache.set(p.id, (l = lay.layoutPage(p)));
    return l;
  };
}

/** A node drawn as its region of the top-level node it sits in, as the server renderer does. */
function nodeItem(page: Page, laid: LaidPage, id: string): ExportItem | undefined {
  const top = topOf(page, id);
  const lt = top && laid.nodes.find((n) => n.id === top.id);
  const box = top?.id === id ? lt?.box : laid.boxes.get(id);
  if (!top || !lt || !box || !(box.w > 0 && box.h > 0)) return undefined;
  let node: DesignNode = top;
  walk([top], (n) => void (n.id === id && (node = n)));
  return { name: labelOf(node), page: page.id, node: id, box, ops: lt.ops, background: laid.background };
}

/** The frame an export of "this frame" means: the top-level frame holding the selection, else the page's first. */
export function frameFor(design: Design, pageId: string | undefined, selected: string[]): { page: Page; frame: DesignNode } | undefined {
  const page = design.pages.find((p) => p.id === pageId) ?? design.pages[0];
  if (!page) return undefined;
  for (const id of selected) {
    const top = topOf(page, id);
    if (top && top.type === "frame" && top.visible !== false) return { page, frame: top };
  }
  const first = page.children.find((n) => n.type === "frame" && n.visible !== false);
  return first ? { page, frame: first } : undefined;
}

/** Every visible top-level frame, in page order and then layer order. */
export const allFrames = (design: Design) => design.pages.flatMap((page) => page.children.filter((n) => n.type === "frame" && n.visible !== false).map((frame) => ({ page, frame })));

/** What an export of the scope draws, one item per picture (and per PDF page). */
export function exportItems(design: Design, scope: ExportScope, at: { page?: string; selected?: string[] }): ExportItem[] {
  const laid = laidPages(design);
  const page = design.pages.find((p) => p.id === at.page) ?? design.pages[0];
  switch (scope) {
    case "selection": {
      const out: ExportItem[] = [];
      for (const id of at.selected ?? []) {
        const f = find(design, id);
        if (!f || f.node.type === "connector") continue;
        const it = nodeItem(f.page, laid(f.page), id);
        if (it) out.push(it);
      }
      return out;
    }
    case "frame": {
      const f = frameFor(design, at.page, at.selected ?? []);
      const it = f && nodeItem(f.page, laid(f.page), f.frame.id);
      return it ? [it] : [];
    }
    case "page": {
      if (!page) return [];
      const l = laid(page);
      return l.box.w > 0 && l.box.h > 0 ? [{ name: page.name, page: page.id, box: l.box, ops: pageOps(l), background: l.background }] : [];
    }
    case "frames":
      return allFrames(design).flatMap(({ page: p, frame }) => nodeItem(p, laid(p), frame.id) ?? []);
  }
}

/** A file name without characters file systems refuse. */
export function fileName(...parts: string[]): string {
  const s = parts.filter(Boolean).join(" – ").replace(/[\\/:*?"<>|\u0000-\u001f]+/g, " ").replace(/\s+/g, " ").trim();
  return s.slice(0, 120) || "Design";
}

/** The query for GET /designs/{id}/render that draws the item (the same region the worker draws). */
export function renderQuery(item: ExportItem, scale: number): string {
  const q = new URLSearchParams(item.node ? { node: item.node } : { page: item.page });
  q.set("scale", String(scale));
  return q.toString();
}

/** Why a PNG of the item at this scale can't be made, or undefined. */
export function pngTooLarge(item: ExportItem, scale: number): string | undefined {
  const w = Math.round(item.box.w * scale), h = Math.round(item.box.h * scale);
  return Math.max(w, h) > MAX_PNG_EDGE ? `${item.name} would be ${w}×${h} px at ${scale}×; a PNG is at most ${MAX_PNG_EDGE} px on its longest edge. Try a smaller scale, or SVG or PDF.` : undefined;
}

// ---- Fonts and images

/** "Regular", "SemiBoldItalic"…: the file name part of an Inter face. */
export function faceName(weight: number, italic: boolean): string {
  const base = ({ 400: "Regular", 500: "Medium", 600: "SemiBold", 700: "Bold" } as Record<number, string>)[weight] ?? "Regular";
  return italic ? (base === "Regular" ? "Italic" : `${base}Italic`) : base;
}

/** The faces the items' text uses, as [weight, italic]. */
export function usedFaces(items: ExportItem[]): [number, boolean][] {
  const seen = new Map<string, [number, boolean]>();
  for (const it of items) for (const o of it.ops) if (o.op === "text") seen.set(`${o.weight}${o.italic ? "i" : ""}`, [o.weight, o.italic]);
  return [...seen.values()];
}

/** The image assets the items draw. */
export const usedAssets = (items: ExportItem[]) => [...new Set(items.flatMap((it) => it.ops.flatMap((o) => (o.op === "image" ? [o.asset] : []))))];

/** Loads font and image bytes: from the app's URLs in the browser, from disk in tests. */
export type Loaders = {
  /** An Inter face file, e.g. ("SemiBold", "woff2"). */
  font: (face: string, kind: "woff2" | "ttf") => Promise<Uint8Array>;
  /** Asset id -> data URI (PNG or JPEG); undefined draws the image empty, as the renderer does. */
  assets: Record<string, string>;
};

export function base64(b: Uint8Array): string {
  if (typeof Buffer !== "undefined") return Buffer.from(b.buffer, b.byteOffset, b.byteLength).toString("base64");
  let s = "";
  for (let i = 0; i < b.length; i += 0x8000) s += String.fromCharCode(...b.subarray(i, i + 0x8000));
  return btoa(s);
}

// ---- SVG

/**
 * One self-contained SVG per item: text stays <text>, the Inter faces it uses
 * are embedded as WOFF2 data URIs (so it opens offline) with a font-family
 * fallback list, and images are inline data URIs.
 */
export async function exportSvg(item: ExportItem, load: Loaders): Promise<string> {
  const faces = usedFaces([item]);
  const css = (
    await Promise.all(
      faces.map(async ([w, italic]) => `@font-face{font-family:"${FAMILY}";src:url(data:font/woff2;base64,${base64(await load.font(faceName(w, italic), "woff2"))}) format("woff2");font-weight:${w};font-style:${italic ? "italic" : "normal"}}`),
    )
  ).join("");
  const assets = Object.fromEntries(usedAssets([item]).flatMap((a) => (load.assets[a] ? [[a, load.assets[a]]] : [])));
  return `<?xml version="1.0" encoding="UTF-8"?>\n${toSvg(item.ops, { box: item.box, background: item.background, assets, css })}\n`;
}

// ---- PDF

type Seg = { op: "m" | "l" | "c" | "h"; c: number[] };

/** SVG path data (M L C Q Z, absolute or relative) as jsPDF path operators. */
export function pathSegs(d: string): Seg[] {
  const tokens = d.match(/[MLCQZmlcqz]|-?(?:\d+\.?\d*|\.\d+)(?:e-?\d+)?/g) ?? [];
  const out: Seg[] = [];
  let i = 0, cmd = "", x = 0, y = 0, sx = 0, sy = 0;
  const num = () => Number(tokens[i++]);
  while (i < tokens.length) {
    if (/[A-Za-z]/.test(tokens[i])) cmd = tokens[i++];
    else if (!cmd) break;
    const rel = cmd === cmd.toLowerCase();
    const ox = rel ? x : 0, oy = rel ? y : 0;
    switch (cmd.toUpperCase()) {
      case "M":
        x = ox + num(); y = oy + num();
        sx = x; sy = y;
        out.push({ op: "m", c: [x, y] });
        cmd = rel ? "l" : "L"; // further pairs are lines
        break;
      case "L":
        x = ox + num(); y = oy + num();
        out.push({ op: "l", c: [x, y] });
        break;
      case "C": {
        const c = [ox + num(), oy + num(), ox + num(), oy + num(), ox + num(), oy + num()];
        x = c[4]; y = c[5];
        out.push({ op: "c", c });
        break;
      }
      case "Q": {
        const qx = ox + num(), qy = oy + num(), ex = ox + num(), ey = oy + num();
        out.push({ op: "c", c: [x + (2 / 3) * (qx - x), y + (2 / 3) * (qy - y), ex + (2 / 3) * (qx - ex), ey + (2 / 3) * (qy - ey), ex, ey] });
        x = ex; y = ey;
        break;
      }
      case "Z":
        out.push({ op: "h", c: [] });
        x = sx; y = sy;
        cmd = "";
        break;
    }
  }
  return out;
}

const hex7 = (c: string) => c.slice(0, 7);
const alphaOf = (c: string | undefined) => (c && c.length === 9 ? parseInt(c.slice(7), 16) / 255 : 1);

/**
 * One PDF page per item at its size (1 px = 0.75 pt), in the items' order,
 * drawn from the display list with the Inter faces embedded. Builds the whole
 * file in memory: nothing is handed out until it's complete.
 */
export async function exportPdf(items: ExportItem[], load: Loaders, signal?: AbortSignal): Promise<Uint8Array> {
  if (!items.length) throw new ExportError("there is nothing to export");
  const { jsPDF } = await import("jspdf");
  const size = (b: Box): [number, number] => [b.w * PT, b.h * PT];
  const orient = (b: Box) => (b.w > b.h ? "landscape" : "portrait");
  const pdf = new jsPDF({ unit: "pt", format: size(items[0].box), orientation: orient(items[0].box), compress: true });
  const fontId = (w: number, italic: boolean) => `DesignsInter-${faceName(w, italic)}`;
  await Promise.all(
    usedFaces(items).map(async ([w, italic]) => {
      const file = `Inter-${faceName(w, italic)}.ttf`;
      pdf.addFileToVFS(file, base64(await load.font(faceName(w, italic), "ttf")));
      pdf.addFont(file, fontId(w, italic), "normal");
    }),
  );

  let shadings = 0;
  items.forEach((item, n) => {
    if (signal?.aborted) throw new DOMException("cancelled", "AbortError");
    if (n) pdf.addPage(size(item.box), orient(item.box));
    const { box } = item;
    const H = box.h * PT;
    const X = (v: number) => (v - box.x) * PT;
    const Y = (v: number) => (v - box.y) * PT;
    const L = (v: number) => v * PT;
    if (item.background) {
      pdf.setFillColor(hex7(item.background));
      pdf.rect(0, 0, box.w * PT, H, "F");
    }
    const gstate = (fill: number, stroke: number) => {
      if (fill < 1 || stroke < 1) pdf.setGState(pdf.GState({ opacity: fill, "stroke-opacity": stroke }));
    };
    const strokeStyle = (o: { stroke?: string; sw: number; dash?: string; cap?: string; join?: string }) => {
      pdf.setDrawColor(hex7(o.stroke!));
      pdf.setLineWidth(L(o.sw));
      pdf.setLineDashPattern(o.dash ? o.dash.split(/[ ,]+/).map((v) => L(Number(v))) : [], 0);
      pdf.setLineCap(o.cap === "round" ? "round" : o.cap === "square" ? "square" : "butt");
      pdf.setLineJoin(o.join === "round" ? "round" : o.join === "bevel" ? "bevel" : "miter");
    };
    /** Fills the path drawn by path() with a native axial shading across b (SVG's objectBoundingBox line). */
    const shade = (p: Exclude<ResolvedPaint, string>, b: Box, path: () => void) => {
      const key = `grad${++shadings}`;
      const a = (p.angle * Math.PI) / 180;
      const at = (t: number, c: number) => 0.5 + (t * c) / 2;
      const coords = [X(b.x + at(-1, Math.cos(a)) * b.w), Y(b.y + at(-1, Math.sin(a)) * b.h), X(b.x + at(1, Math.cos(a)) * b.w), Y(b.y + at(1, Math.sin(a)) * b.h)];
      const rgb = (c: string) => [1, 3, 5].map((i) => parseInt(c.slice(i, i + 2), 16));
      pdf.advancedAPI((api) => {
        api.addShadingPattern(key, api.ShadingPattern("axial", coords, [{ offset: 0, color: rgb(p.stops[0]) }, { offset: 1, color: rgb(p.stops[1]) }]));
        path();
        api.fill({ key, matrix: api.unitMatrix });
      });
    };
    const roundRect = (x: number, y: number, w: number, h: number, r: number, style: "F" | "S" | "FD" | null) => {
      const rr = Math.min(r, w / 2, h / 2);
      if (rr > 0) pdf.roundedRect(X(x), Y(y), L(w), L(h), L(rr), L(rr), style);
      else pdf.rect(X(x), Y(y), L(w), L(h), style);
    };

    for (const o of item.ops) {
      switch (o.op) {
        case "clip":
          pdf.saveGraphicsState();
          roundRect(o.x, o.y, o.w, o.h, o.r, null);
          pdf.clip();
          pdf.discardPath();
          continue;
        case "rotate": {
          pdf.saveGraphicsState();
          // PDF space is y-up, so a clockwise rotation is negative there.
          const r = (o.deg * Math.PI) / 180, cx = X(o.cx), cy = H - Y(o.cy), c = Math.cos(r), s = Math.sin(r);
          pdf.setCurrentTransformationMatrix(pdf.Matrix(c, -s, s, c, cx - c * cx - s * cy, cy + s * cx - c * cy));
          continue;
        }
        case "unclip":
        case "unrotate":
          pdf.restoreGraphicsState();
          continue;
      }
      const opacity = "opacity" in o && o.opacity !== undefined ? o.opacity : 1;
      pdf.saveGraphicsState();
      if (o.op === "rect" || o.op === "ellipse" || o.op === "path" || o.op === "polyline") {
        const fill = o.op === "polyline" ? undefined : o.fill;
        const solid = typeof fill === "string" ? fill : undefined;
        const stroked = !!(o.stroke && o.sw);
        gstate(opacity * alphaOf(solid), opacity * alphaOf(o.stroke));
        if (solid) pdf.setFillColor(hex7(solid));
        if (stroked) strokeStyle(o);
        const draw = (style: "F" | "S" | "FD" | null) => {
          if (o.op === "rect") roundRect(o.x, o.y, Math.max(0, o.w), Math.max(0, o.h), o.r, style);
          else if (o.op === "ellipse") pdf.ellipse(X(o.x + o.w / 2), Y(o.y + o.h / 2), L(Math.max(0, o.w / 2)), L(Math.max(0, o.h / 2)), style);
          else {
            const segs = o.op === "path" ? pathSegs(o.d) : o.points.map(([x, y], i) => ({ op: i ? "l" : "m", c: [x, y] }) as Seg);
            // jsPDF's path takes absolute operators in page units and paints nothing itself.
            pdf.path(segs.map((g) => ({ op: g.op, c: g.c.map((v, i) => (i % 2 ? Y(v) : X(v))) })));
            if (style === "FD") pdf.fillStroke();
            else if (style === "F") pdf.fill();
            else if (style === "S") pdf.stroke();
          }
        };
        if (fill && typeof fill !== "string") {
          const b = o.op === "path" ? o.box : o.op === "polyline" ? box : o;
          shade(fill, b, () => draw(null));
          if (stroked) draw("S");
        } else {
          const style = solid && stroked ? "FD" : solid ? "F" : stroked ? "S" : null;
          if (style) draw(style);
        }
      } else if (o.op === "image") {
        const uri = load.assets[o.asset];
        if (uri) {
          gstate(opacity, opacity);
          roundRect(o.x, o.y, o.w, o.h, o.r, null);
          pdf.clip();
          pdf.discardPath();
          const sx = o.w / o.src.w, sy = o.h / o.src.h;
          pdf.addImage(uri, uri.startsWith("data:image/png") ? "PNG" : "JPEG", X(o.x - o.src.x * sx), Y(o.y - o.src.y * sy), L(o.iw * sx), L(o.ih * sy), o.asset);
        }
      } else if (o.op === "text") {
        gstate(opacity * alphaOf(o.fill), 1);
        pdf.setFont(fontId(o.weight, o.italic), "normal");
        pdf.setFontSize(L(o.size));
        pdf.setTextColor(hex7(o.fill));
        pdf.text(o.text, X(o.x), Y(o.y), { baseline: "alphabetic" });
      }
      pdf.restoreGraphicsState();
    }
  });
  return new Uint8Array(pdf.output("arraybuffer"));
}
