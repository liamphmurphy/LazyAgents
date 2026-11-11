"use client";

/**
 * PDF and PPTX export, built in the browser (ADR §3.8, §4). Both libraries
 * load on first use. The PDF draws the shared display list with the slide
 * fonts embedded; the PPTX maps elements to native, editable PowerPoint
 * objects using the same layout constants.
 */
import type PptxGenJS from "pptxgenjs";
import { FAMILY, OFFICE, SLOTS, STYLES, fontUrl, measure } from "./fonts";
import { BULLET_INDENT, INSET, STRIKE, imageSource, layoutSlide, lineHeight } from "./layout";
import { isImageSlot, resolveColor, slotOf, textOf, type Deck, type Paragraph, type TextBox } from "./schema";

const K = 0.5; // slide units → points
const IN = (u: number) => u / 144; // slide units → inches (1920 units = 13.333 in)

async function bytes(url: string): Promise<Uint8Array> {
  const r = await fetch(url, { credentials: "same-origin" });
  if (!r.ok) throw new Error(`couldn't load ${url.includes("/assets/") ? "an image" : "a font"} (${r.status})`);
  return new Uint8Array(await r.arrayBuffer());
}

function base64(b: Uint8Array): string {
  let s = "";
  for (let i = 0; i < b.length; i += 0x8000) s += String.fromCharCode(...b.subarray(i, i + 0x8000));
  return btoa(s);
}

/** Every image the shown slides use, fetched once. */
async function images(deck: Deck, assetUrl: (id: string) => string) {
  const used = new Set(deck.slides.filter((s) => !s.hidden).flatMap((s) => s.elements.flatMap((e) => (e.type === "image" && deck.assets[e.asset] ? [e.asset] : []))));
  const out = new Map<string, Uint8Array>();
  await Promise.all([...used].map(async (id) => out.set(id, await bytes(assetUrl(id)))));
  return out;
}

export type Progress = (done: number, total: number) => void;

/** One 960 × 540 pt page per shown slide, text real and fonts embedded. */
export async function deckToPdf(deck: Deck, assetUrl: (id: string) => string, progress?: Progress, signal?: AbortSignal): Promise<Blob> {
  const [{ jsPDF }, imgs] = await Promise.all([import("jspdf"), images(deck, assetUrl)]);
  const usedSlots = new Set(deck.slides.flatMap((s) => s.elements.flatMap((e) => (textOf(e)?.paragraphs ?? []).flatMap((p) => p.runs.map((r) => slotOf(deck.theme, r.font))))));
  const pdf = new jsPDF({ unit: "pt", format: [deck.size.w * K, deck.size.h * K], orientation: "landscape", compress: true });
  await Promise.all(
    SLOTS.filter((s) => usedSlots.has(s)).flatMap((slot) =>
      STYLES.map(async (style) => {
        const name = `${slot}-${style}.ttf`;
        pdf.addFileToVFS(name, base64(await bytes(fontUrl(slot, style))));
        pdf.addFont(name, FAMILY[slot], style);
      }),
    ),
  );
  const shown = deck.slides.filter((s) => !s.hidden);
  shown.forEach((slide, i) => {
    if (signal?.aborted) throw new DOMException("cancelled", "AbortError");
    if (i) pdf.addPage([deck.size.w * K, deck.size.h * K], "landscape");
    for (const o of layoutSlide(deck, slide, measure).ops) {
      pdf.saveGraphicsState();
      if (o.rot) {
        // Rotate about the element's centre; PDF space is y-up, so a clockwise slide rotation is negative here.
        const r = (o.rot.deg * Math.PI) / 180, cx = o.rot.cx * K, cy = deck.size.h * K - o.rot.cy * K, c = Math.cos(r), s = Math.sin(r);
        pdf.setCurrentTransformationMatrix(pdf.Matrix(c, -s, s, c, cx - c * cx - s * cy, cy + s * cx - c * cy));
      }
      if (o.op === "rect" || o.op === "ellipse" || o.op === "path") {
        const style = o.fill && o.stroke && o.sw ? "FD" : o.fill ? "F" : o.stroke && o.sw ? "S" : null;
        if (o.fill) pdf.setFillColor(o.fill);
        if (o.stroke) {
          pdf.setDrawColor(o.stroke);
          pdf.setLineWidth(o.sw * K);
        }
        if (style && o.op === "path") {
          // jsPDF's path takes absolute m/l/c/h operators in page units.
          pdf.path(o.segs.map((g) => (g[0] === "Z" ? { op: "h", c: [] } : { op: g[0].toLowerCase(), c: g.slice(1).map((v) => (v as number) * K) })));
          if (style === "FD") pdf.fillStroke();
          else if (style === "F") pdf.fill();
          else pdf.stroke();
        } else if (style && o.op === "rect") {
          if (o.r) pdf.roundedRect(o.x * K, o.y * K, o.w * K, o.h * K, o.r * K, o.r * K, style);
          else pdf.rect(o.x * K, o.y * K, o.w * K, o.h * K, style);
        } else if (style && o.op === "ellipse") pdf.ellipse((o.x + o.w / 2) * K, (o.y + o.h / 2) * K, (o.w / 2) * K, (o.h / 2) * K, style);
      } else if (o.op === "line") {
        pdf.setDrawColor(o.stroke);
        pdf.setFillColor(o.stroke);
        pdf.setLineWidth(o.sw * K);
        const len = Math.hypot(o.x2 - o.x1, o.y2 - o.y1) || 1;
        const ux = (o.x2 - o.x1) / len, uy = (o.y2 - o.y1) / len, head = o.sw * 4;
        const tip = (x: number, y: number, dir: number) => {
          const bx = x - dir * ux * head, by = y - dir * uy * head, px = -uy * head * 0.6, py = ux * head * 0.6;
          pdf.triangle(x * K, y * K, (bx + px) * K, (by + py) * K, (bx - px) * K, (by - py) * K, "F");
        };
        const x1 = o.startArrow ? o.x1 + ux * head : o.x1, y1 = o.startArrow ? o.y1 + uy * head : o.y1;
        const x2 = o.endArrow ? o.x2 - ux * head : o.x2, y2 = o.endArrow ? o.y2 - uy * head : o.y2;
        pdf.line(x1 * K, y1 * K, x2 * K, y2 * K);
        if (o.endArrow) tip(o.x2, o.y2, 1);
        if (o.startArrow) tip(o.x1, o.y1, -1);
      } else if (o.op === "image") {
        const a = deck.assets[o.asset], data = imgs.get(o.asset);
        if (data) {
          const s = o.w / o.src.w;
          pdf.rect(o.x * K, o.y * K, o.w * K, o.h * K, null);
          pdf.clip();
          pdf.discardPath();
          pdf.addImage(data, a.mime === "image/png" ? "PNG" : "JPEG", (o.x - o.src.x * s) * K, (o.y - o.src.y * s) * K, a.w * s * K, a.h * s * K, o.asset);
        }
      } else if (o.op === "highlight") {
        pdf.setFillColor(o.fill);
        pdf.rect(o.x * K, o.y * K, o.w * K, o.h * K, "F");
      } else if (o.op === "text") {
        pdf.setFont(FAMILY[o.slot], o.style);
        pdf.setFontSize(o.size * K);
        pdf.setTextColor(o.color);
        pdf.text(o.text, o.x * K, o.y * K, { baseline: "alphabetic" });
        if (o.underline) {
          pdf.setDrawColor(o.color);
          pdf.setLineWidth(o.size * 0.06 * K);
          const u = (o.y + o.size * 0.12) * K;
          pdf.line(o.x * K, u, (o.x + o.w) * K, u);
        }
        if (o.strike) {
          pdf.setDrawColor(o.color);
          pdf.setLineWidth(o.size * 0.06 * K);
          const s = (o.y - o.size * STRIKE) * K;
          pdf.line(o.x * K, s, (o.x + o.w) * K, s);
        }
      }
      pdf.restoreGraphicsState();
    }
    progress?.(i + 1, shown.length);
  });
  return pdf.output("blob");
}

/** A PPTX whose text boxes, shapes and lines stay editable; images are cropped pictures. */
export async function deckToPptx(deck: Deck, assetUrl: (id: string) => string): Promise<Blob> {
  const [{ default: PptxGenJS }, imgs] = await Promise.all([import("pptxgenjs"), images(deck, assetUrl)]);
  const pptx = new PptxGenJS();
  pptx.layout = "LAYOUT_WIDE";
  const strip = (c: string) => c.replace("#", "");
  const color = (c?: string, fb: "text" | "background" = "text") => strip(resolveColor(deck.theme, c, fb));
  const runs = (box: TextBox) =>
    box.paragraphs.flatMap((p: Paragraph) =>
      p.runs.map((r, i) => ({
        text: r.text,
        options: {
          fontFace: OFFICE[slotOf(deck.theme, r.font)],
          fontSize: r.size * K,
          bold: !!r.bold,
          italic: !!r.italic,
          underline: r.underline ? { style: "sng" as const } : undefined,
          strike: r.strike ? ("sngStrike" as const) : undefined,
          highlight: r.highlight ? color(r.highlight) : undefined,
          color: color(r.color),
          // Paragraph properties ride on every run of the paragraph.
          align: p.align ?? "left",
          // Exact points (a:spcPts), not a percentage: PowerPoint's 100% is the font's own
          // line gap, not the layout's LINE, so a percentage would wrap and overflow differently.
          lineSpacing: lineHeight(p) * K,
          paraSpaceBefore: (p.spaceBefore ?? 0) * K,
          bullet: p.bullet ? (p.bullet === "number" ? { type: "number" as const, indent: BULLET_INDENT * K } : { indent: BULLET_INDENT * K }) : false,
          indentLevel: p.bullet ? (p.level ?? 0) : 0,
          breakLine: i === p.runs.length - 1,
        },
      })),
    );
  for (const s of deck.slides) {
    const slide = pptx.addSlide();
    slide.background = { color: color(s.background?.color, "background") };
    if (s.hidden) slide.hidden = true;
    if (s.notes) slide.addNotes(s.notes);
    for (const e of s.elements) {
      // An unfilled image placeholder draws nothing; don't leave an empty box in the file.
      if (isImageSlot(e)) continue;
      const f = { x: IN(e.frame.x), y: IN(e.frame.y), w: IN(e.frame.w), h: IN(e.frame.h), rotate: e.rotation ?? 0, objectName: e.id };
      const textOpts = (box: TextBox) => ({ margin: INSET * K, valign: box.valign ?? "top", fit: "none" as const, wrap: true });
      if (e.type === "text") slide.addText(runs(e), { ...f, ...textOpts(e) });
      else if (e.type === "shape") {
        // Our shape names are prstGeom names, so the shape stays native and editable.
        const shape = e.shape as PptxGenJS.SHAPE_NAME;
        const style = {
          fill: e.fill ? { color: color(e.fill) } : { type: "none" as const },
          line: e.stroke && e.strokeWidth ? { color: color(e.stroke), width: e.strokeWidth * K } : { type: "none" as const },
          rectRadius: e.shape === "roundRect" ? IN(Math.min(e.radius ?? 24, e.frame.w / 2, e.frame.h / 2)) : undefined,
        };
        if (e.text) slide.addText(runs(e.text), { shape, ...f, ...textOpts(e.text), ...style });
        else slide.addShape(shape, { ...f, ...style });
      } else if (e.type === "line") {
        slide.addShape(pptx.ShapeType.line, { ...f, line: { color: color(e.stroke), width: e.strokeWidth * K, endArrowType: e.endArrow ? "triangle" : undefined, beginArrowType: e.startArrow ? "triangle" : undefined } });
      } else if (e.type === "image") {
        const a = deck.assets[e.asset], data = imgs.get(e.asset);
        if (!a || !data) continue;
        const src = imageSource(e, a);
        // pptxgenjs measures the crop against the picture's own w × h.
        slide.addImage({ data: `${a.mime};base64,${base64(data)}`, ...f, altText: e.alt ?? a.alt ?? "", sizing: { type: "crop", x: (src.x / a.w) * f.w, y: (src.y / a.h) * f.h, w: (src.w / a.w) * f.w, h: (src.h / a.h) * f.h } });
      }
    }
  }
  return (await pptx.write({ outputType: "blob" })) as Blob;
}
