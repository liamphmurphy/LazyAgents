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
export type Placeholder = "title" | "subtitle" | "body" | "body2" | "image";

type Common = { id: string; frame: Frame; rotation?: number; placeholder?: Placeholder };
export type TextElement = Common & { type: "text" } & TextBox;
export type ShapeElement = Common & { type: "shape"; shape: "rect" | "roundRect" | "ellipse"; fill?: ColorRef; stroke?: ColorRef; strokeWidth?: number; radius?: number; text?: TextBox };
export type LineElement = Common & { type: "line"; stroke: ColorRef; strokeWidth: number; endArrow?: boolean; startArrow?: boolean };
export type ImageElement = Common & { type: "image"; asset: string; crop?: Crop; alt?: string };
export type Element = TextElement | ShapeElement | LineElement | ImageElement;

export type Layout = "title" | "title-body" | "two-column" | "image-right" | "section" | "blank";
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
];

/** The built-in themes; server/internal/deck/templates.go has the same values. */
export const THEMES: Record<string, Theme> = {
  light: { name: "Light", fonts: { heading: "sans", body: "sans" }, colors: { background: "#FFFFFF", text: "#1F2328", heading: "#111418", accent: "#3B6FE0", accent2: "#E8A33D", muted: "#5F6673", surface: "#F3F5F9" } },
  dark: { name: "Dark", fonts: { heading: "sans", body: "sans" }, colors: { background: "#14151A", text: "#E6E7EB", heading: "#FFFFFF", accent: "#8B9CFF", accent2: "#F2B35E", muted: "#9AA0AE", surface: "#1F2129" } },
  paper: { name: "Paper", fonts: { heading: "serif", body: "serif" }, colors: { background: "#FBF8F1", text: "#2B2723", heading: "#1A1612", accent: "#A2462E", accent2: "#3F6B5C", muted: "#655D53", surface: "#F1EBDD" } },
  contrast: { name: "Contrast", fonts: { heading: "sans", body: "serif" }, colors: { background: "#FFFFFF", text: "#000000", heading: "#000000", accent: "#D7263D", accent2: "#1B998B", muted: "#4A4A4A", surface: "#F0F0F0" } },
  dusk: { name: "Dusk", fonts: { heading: "sans", body: "sans" }, colors: { background: "#1B1830", text: "#E4E0F5", heading: "#FFFFFF", accent: "#B79CFF", accent2: "#F4A261", muted: "#A9A3C4", surface: "#262240" } },
  ocean: { name: "Ocean", fonts: { heading: "sans", body: "sans" }, colors: { background: "#0B2540", text: "#D6E4F0", heading: "#FFFFFF", accent: "#4CC9F0", accent2: "#FFD166", muted: "#9DB4CB", surface: "#13355A" } },
  forest: { name: "Forest", fonts: { heading: "serif", body: "sans" }, colors: { background: "#F3F6EF", text: "#1E2A20", heading: "#14301B", accent: "#2F6B3A", accent2: "#B5651D", muted: "#56635A", surface: "#E3EADC" } },
  sunset: { name: "Sunset", fonts: { heading: "sans", body: "sans" }, colors: { background: "#FFF4EC", text: "#3A1F2B", heading: "#5A1A33", accent: "#D1495B", accent2: "#7B4FA0", muted: "#7A5560", surface: "#FBE3D3" } },
  mono: { name: "Mono", fonts: { heading: "mono", body: "sans" }, colors: { background: "#FAFAFA", text: "#1A1A1A", heading: "#000000", accent: "#111111", accent2: "#777777", muted: "#5C5C5C", surface: "#EDEDED" } },
  blueprint: { name: "Blueprint", fonts: { heading: "mono", body: "sans" }, colors: { background: "#123A6B", text: "#E8F1FB", heading: "#FFFFFF", accent: "#9BD1FF", accent2: "#FFE08A", muted: "#B4CBE6", surface: "#1B4A82" } },
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
      return [heading("", 72, { x: 120, y: 120, w: 800, h: 300 }, "top"), text("body", { x: 120, y: 460, w: 800, h: 480 }, "top", para("", 40, { font: "body", color: "text" }))];
    case "section":
      return [heading("", 128, { x: 160, y: 380, w: 1600, h: 320 }, "middle", "center")];
    default:
      return [];
  }
}

export const newSlide = (layout: Layout): Slide => ({ id: newId("s"), layout, elements: placeholders(layout) });

/** Where a layout puts each placeholder role, for re-laying out a slide. */
export function relayout(slide: Slide, layout: Layout): Slide {
  const fresh = placeholders(layout);
  const used = new Set<string>();
  const elements = slide.elements.map((e) => {
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
