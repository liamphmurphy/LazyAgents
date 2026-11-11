/**
 * The Slides deck schema (ADR: Slides deck model and export). The server
 * (server/internal/deck) validates the same shape; keep the two in step.
 */

export const W = 1920;
export const H = 1080;

export type Slot = "sans" | "serif" | "mono";
export type FontRef = "heading" | "body" | Slot;
export type ThemeColor = "background" | "text" | "heading" | "accent" | "accent2" | "muted" | "surface";
/** A theme colour name or a literal #RRGGBB. */
export type ColorRef = ThemeColor | string;

export type Run = { text: string; size: number; bold?: boolean; italic?: boolean; underline?: boolean; strike?: boolean; color?: ColorRef; highlight?: ColorRef; font?: FontRef };
/** lineSpacing multiplies the default line height (0.8–3; unset is 1). */
export type Paragraph = { runs: Run[]; align?: "left" | "center" | "right" | "justify"; bullet?: "disc" | "number"; level?: number; spaceBefore?: number; lineSpacing?: number };
export type TextBox = { paragraphs: Paragraph[]; valign?: "top" | "middle" | "bottom" };
export type Frame = { x: number; y: number; w: number; h: number };
export type Crop = { l: number; t: number; r: number; b: number };
export type Placeholder = "title" | "subtitle" | "body" | "body2" | "body3" | "label" | "label2" | "image";

type Common = { id: string; frame: Frame; rotation?: number; placeholder?: Placeholder };
export type TextElement = Common & { type: "text" } & TextBox;
export type ShapeElement = Common & { type: "shape"; shape: "rect" | "roundRect" | "ellipse"; fill?: ColorRef; stroke?: ColorRef; strokeWidth?: number; radius?: number; text?: TextBox };
export type LineElement = Common & { type: "line"; stroke: ColorRef; strokeWidth: number; endArrow?: boolean; startArrow?: boolean };
export type ImageElement = Common & { type: "image"; asset: string; crop?: Crop; alt?: string };
export type Element = TextElement | ShapeElement | LineElement | ImageElement;

export type Layout = "title" | "title-body" | "two-column" | "image-right" | "section" | "blank" | "quote" | "big-number" | "three-column" | "comparison" | "image-full" | "image-left" | "agenda" | "closing";
export type Slide = { id: string; layout: Layout; background?: { color: ColorRef }; notes?: string; hidden?: boolean; elements: Element[] };
export type Theme = { name: string; fonts: { heading: Slot; body: Slot }; colors: Record<ThemeColor, string> };
export type Asset = { mime: "image/png" | "image/jpeg"; w: number; h: number; alt?: string };
export type Deck = { schema: 1; size: { w: number; h: number }; theme: Theme; assets: Record<string, Asset>; slides: Slide[] };

export const THEME_COLORS: ThemeColor[] = ["background", "text", "heading", "accent", "accent2", "muted", "surface"];
export const LAYOUTS: { id: Layout; label: string }[] = [
  { id: "title", label: "Title" },
  { id: "title-body", label: "Title and body" },
  { id: "two-column", label: "Two columns" },
  { id: "image-right", label: "Text and image" },
  { id: "section", label: "Section" },
  { id: "blank", label: "Blank" },
  { id: "quote", label: "Quote" },
  { id: "big-number", label: "Big number" },
  { id: "three-column", label: "Three columns" },
  { id: "comparison", label: "Comparison" },
  { id: "image-full", label: "Full-bleed image" },
  { id: "image-left", label: "Image and text" },
  { id: "agenda", label: "Agenda" },
  { id: "closing", label: "Closing" },
];
/** Layouts added in the second Slides epic; the picker badges them. */
export const NEW_LAYOUTS = new Set<Layout>(["quote", "big-number", "three-column", "comparison", "image-full", "image-left", "agenda", "closing"]);

/** The built-in themes; server/internal/deck/templates.go has the same values. */
export const THEMES: Record<string, Theme> = {
  light: { name: "Light", fonts: { heading: "sans", body: "sans" }, colors: { background: "#FFFFFF", text: "#1F2328", heading: "#111418", accent: "#3B6FE0", accent2: "#E8A33D", muted: "#6B7280", surface: "#F3F5F9" } },
  dark: { name: "Dark", fonts: { heading: "sans", body: "sans" }, colors: { background: "#14151A", text: "#E6E7EB", heading: "#FFFFFF", accent: "#8B9CFF", accent2: "#F2B35E", muted: "#9AA0AE", surface: "#1F2129" } },
  paper: { name: "Paper", fonts: { heading: "serif", body: "serif" }, colors: { background: "#FBF8F1", text: "#2B2723", heading: "#1A1612", accent: "#A2462E", accent2: "#3F6B5C", muted: "#7A7268", surface: "#F1EBDD" } },
  contrast: { name: "Contrast", fonts: { heading: "sans", body: "serif" }, colors: { background: "#FFFFFF", text: "#000000", heading: "#000000", accent: "#D7263D", accent2: "#1B998B", muted: "#4A4A4A", surface: "#F0F0F0" } },
};
export type ThemeKey = keyof typeof THEMES;

/** A fresh id: prefix ("s", "e" or "a") and 10 random base32 characters, like the server's. */
export function newId(prefix: "s" | "e" | "a"): string {
  const alphabet = "abcdefghijklmnopqrstuvwxyz234567";
  const b = crypto.getRandomValues(new Uint8Array(10));
  return `${prefix}_${Array.from(b, (n) => alphabet[n % 32]).join("")}`;
}

/** A theme colour name resolved to #RRGGBB; literals pass through. */
export function resolveColor(theme: Theme, c: ColorRef | undefined, fallback: ThemeColor = "text"): string {
  const v = c ?? fallback;
  return v.startsWith("#") ? v.toUpperCase() : (theme.colors[v as ThemeColor] ?? theme.colors[fallback]);
}

/** The slot a run's font resolves to. */
export const slotOf = (theme: Theme, f: FontRef | undefined): Slot => (f === undefined || f === "body" ? theme.fonts.body : f === "heading" ? theme.fonts.heading : f);

const para = (text: string, size: number, opts: Partial<Run> = {}, p: Partial<Paragraph> = {}): Paragraph => ({ ...p, runs: [{ text, size, ...opts }] });
const text = (placeholder: Placeholder, frame: Frame, valign: TextBox["valign"], ...paragraphs: Paragraph[]): TextElement => ({ id: newId("e"), type: "text", placeholder, frame, valign, paragraphs });

/**
 * An empty image placeholder: a text element with the image role and no text,
 * so it draws nothing in presentation, PDF and PPTX. The editor shows it as a
 * "click to add image" box and swaps in an image element with the same frame
 * and role. server/internal/deck imageSlot makes the same.
 */
const imageSlot = (frame: Frame): TextElement => text("image", frame, "middle", para("", 40, { font: "body", color: "muted" }));

/** An image placeholder nobody has filled yet. */
export const isImageSlot = (e: Element): boolean => e.type === "text" && e.placeholder === "image";

/** The elements a layout starts with; server/internal/deck Placeholders makes the same. */
export function placeholders(layout: Layout, title = ""): Element[] {
  const heading = (t: string, size: number, frame: Frame, valign: TextBox["valign"], align?: Paragraph["align"]) =>
    text("title", frame, valign, para(t, size, { bold: true, font: "heading", color: "heading" }, align ? { align } : {}));
  const body = (p: Placeholder, frame: Frame) => text(p, frame, "top", para("", 44, { font: "body", color: "text" }, { bullet: "disc" }));
  switch (layout) {
    case "title":
      return [heading(title || "Title", 112, { x: 160, y: 300, w: 1600, h: 280 }, "bottom"), text("subtitle", { x: 160, y: 600, w: 1600, h: 120 }, "top", para("", 44, { font: "body", color: "muted" }))];
    case "title-body":
      return [heading("", 80, { x: 120, y: 80, w: 1680, h: 160 }, "middle"), body("body", { x: 120, y: 280, w: 1680, h: 700 })];
    case "two-column":
      return [heading("", 80, { x: 120, y: 80, w: 1680, h: 160 }, "middle"), body("body", { x: 120, y: 280, w: 810, h: 700 }), body("body2", { x: 990, y: 280, w: 810, h: 700 })];
    case "image-right":
      return [heading("", 72, { x: 120, y: 120, w: 800, h: 300 }, "top"), text("body", { x: 120, y: 460, w: 800, h: 480 }, "top", para("", 40, { font: "body", color: "text" })), imageSlot({ x: 1000, y: 120, w: 800, h: 840 })];
    case "section":
      return [heading("", 128, { x: 160, y: 380, w: 1600, h: 320 }, "middle", "center")];
    case "quote":
      return [text("body", { x: 200, y: 220, w: 1520, h: 480 }, "bottom", para("", 72, { italic: true, font: "heading", color: "heading" })), text("subtitle", { x: 200, y: 740, w: 1520, h: 100 }, "top", para("", 44, { font: "body", color: "muted" }))];
    case "big-number":
      return [heading("", 280, { x: 160, y: 220, w: 1600, h: 420 }, "bottom", "center"), text("subtitle", { x: 160, y: 680, w: 1600, h: 160 }, "top", para("", 56, { font: "body", color: "muted" }, { align: "center" }))];
    case "three-column":
      return [heading("", 80, { x: 120, y: 80, w: 1680, h: 160 }, "middle"), body("body", { x: 120, y: 280, w: 520, h: 700 }), body("body2", { x: 700, y: 280, w: 520, h: 700 }), body("body3", { x: 1280, y: 280, w: 520, h: 700 })];
    case "comparison": {
      const label = (p: Placeholder, x: number) => text(p, { x, y: 280, w: 810, h: 90 }, "middle", para("", 52, { bold: true, font: "heading", color: "accent" }));
      return [heading("", 80, { x: 120, y: 80, w: 1680, h: 160 }, "middle"), label("label", 120), label("label2", 990), body("body", { x: 120, y: 390, w: 810, h: 590 }), body("body2", { x: 990, y: 390, w: 810, h: 590 })];
    }
    case "image-full":
      return [imageSlot({ x: 0, y: 0, w: 1920, h: 1080 }), text("title", { x: 120, y: 780, w: 1680, h: 200 }, "bottom", para("", 96, { bold: true, font: "heading", color: "#FFFFFF" }))];
    case "image-left":
      return [imageSlot({ x: 120, y: 120, w: 800, h: 840 }), heading("", 72, { x: 1000, y: 120, w: 800, h: 300 }, "top"), text("body", { x: 1000, y: 460, w: 800, h: 480 }, "top", para("", 40, { font: "body", color: "text" }))];
    case "agenda":
      return [heading("", 80, { x: 120, y: 80, w: 1680, h: 160 }, "middle"), text("body", { x: 120, y: 280, w: 1680, h: 700 }, "top", para("", 52, { font: "body", color: "text" }, { bullet: "number" }))];
    case "closing":
      return [heading(title || "Thank you", 128, { x: 160, y: 300, w: 1600, h: 280 }, "bottom", "center"), text("subtitle", { x: 160, y: 620, w: 1600, h: 140 }, "top", para("", 44, { font: "body", color: "muted" }, { align: "center" }))];
    default:
      return [];
  }
}

export const newSlide = (layout: Layout): Slide => ({ id: newId("s"), layout, elements: placeholders(layout) });

/** Where a layout puts each placeholder role, for re-laying out a slide. */
export function relayout(slide: Slide, layout: Layout): Slide {
  const fresh = placeholders(layout);
  const used = new Set<string>();
  // An unfilled image placeholder has nothing to keep: drop it when the new
  // layout has no image slot, rather than leave an invisible box.
  const keep = fresh.some((f) => f.placeholder === "image") ? slide.elements : slide.elements.filter((e) => !isImageSlot(e));
  const elements = keep.map((e) => {
    if (!e.placeholder) return e;
    const f = fresh.find((x) => x.placeholder === e.placeholder && !used.has(x.placeholder!));
    if (!f) return e;
    used.add(e.placeholder);
    return { ...e, frame: f.frame };
  });
  for (const f of fresh) {
    if (used.has(f.placeholder!)) continue;
    elements.push(f);
    used.add(f.placeholder!);
  }
  return { ...slide, layout, elements };
}

/** The text an element carries, if any: a text box, or a shape's text. */
export function textOf(e: Element): TextBox | undefined {
  if (e.type === "text") return e;
  if (e.type === "shape") return e.text;
  return undefined;
}

/** Every run's text in a slide, for accessible labels and change summaries. */
export function slideText(s: Slide): string {
  return s.elements
    .map((e) => (e.type === "image" ? e.alt ?? "" : (textOf(e)?.paragraphs ?? []).map((p) => p.runs.map((r) => r.text).join("")).join(" ")))
    .filter(Boolean)
    .join(" · ");
}

/** A slide's title: its title placeholder's text, else its first text. */
export function slideTitle(s: Slide): string {
  const t = s.elements.find((e) => e.placeholder === "title") ?? s.elements.find((e) => textOf(e));
  const words = t ? (textOf(t)?.paragraphs ?? []).map((p) => p.runs.map((r) => r.text).join("")).join(" ").trim() : "";
  return words || "Untitled slide";
}

export function parseDeck(body: string): Deck {
  return JSON.parse(body) as Deck;
}

/** The deck as the server stores it (keys in the server's order isn't required: it normalises). */
export const encodeDeck = (d: Deck): string => JSON.stringify(d);
