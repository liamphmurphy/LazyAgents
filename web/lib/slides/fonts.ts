"use client";

/**
 * The fonts every slide renderer measures and draws with: the Liberation 2.x
 * faces in public/fonts/slides (SIL OFL 1.1). They're registered under their
 * own family names so a system copy of a different Liberation version can't
 * change where lines break.
 */
import type { Slot } from "./schema";
import type { Measure, Style } from "./layout";

export const FAMILY: Record<Slot, string> = { sans: "Vesper Slides Sans", serif: "Vesper Slides Serif", mono: "Vesper Slides Mono" };
/** What PPTX names, metric-compatible with each slot (ADR §3.4). */
export const OFFICE: Record<Slot, string> = { sans: "Arial", serif: "Times New Roman", mono: "Courier New" };
const FILE: Record<Slot, string> = { sans: "LiberationSans", serif: "LiberationSerif", mono: "LiberationMono" };
const FILE_STYLE: Record<Style, string> = { normal: "Regular", bold: "Bold", italic: "Italic", bolditalic: "BoldItalic" };

export const fontUrl = (slot: Slot, style: Style) => `/fonts/slides/${FILE[slot]}-${FILE_STYLE[style]}.ttf`;
export const SLOTS: Slot[] = ["sans", "serif", "mono"];
export const STYLES: Style[] = ["normal", "bold", "italic", "bolditalic"];

let loading: Promise<void> | undefined;

/** Registers and loads every face once; later calls share the promise. */
export function loadSlideFonts(): Promise<void> {
  if (loading) return loading;
  if (typeof document === "undefined" || typeof FontFace === "undefined") return Promise.resolve();
  loading = Promise.all(
    SLOTS.flatMap((slot) =>
      STYLES.map(async (style) => {
        const face = new FontFace(FAMILY[slot], `url(${fontUrl(slot, style)})`, { weight: style.includes("bold") ? "700" : "400", style: style.includes("italic") ? "italic" : "normal" });
        document.fonts.add(await face.load());
      }),
    ),
  ).then(() => undefined);
  loading.catch(() => (loading = undefined));
  return loading;
}

/** The CSS font shorthand for a run, at a pixel size. */
export const cssFont = (slot: Slot, style: Style, px: number) => `${style.includes("italic") ? "italic " : ""}${style.includes("bold") ? "700" : "400"} ${px}px "${FAMILY[slot]}"`;

let ctx: CanvasRenderingContext2D | null | undefined;
const cache = new Map<string, number>();

/**
 * Measures at a fixed 100px and scales, so results don't depend on the
 * size's rounding; kerning is off here and wherever the text is drawn.
 */
export const measure: Measure = (text, slot, style, size) => {
  if (!text) return 0;
  const key = `${slot}|${style}|${text}`;
  let w = cache.get(key);
  if (w === undefined) {
    if (ctx === undefined) ctx = typeof document === "undefined" ? null : document.createElement("canvas").getContext("2d");
    if (!ctx) return text.length * 0.5 * size;
    ctx.font = cssFont(slot, style, 100);
    if ("fontKerning" in ctx) ctx.fontKerning = "none";
    w = ctx.measureText(text).width / 100;
    if (cache.size > 20000) cache.clear();
    cache.set(key, w);
  }
  return w * size;
};

/** Forget measurements taken before the fonts loaded. */
export const resetMeasureCache = () => cache.clear();
