/**
 * Shape style presets: named bundles of theme colour tokens. They're applied,
 * not referenced: the element keeps the resulting fill, stroke, strokeWidth
 * dash and run colours (dropping any opacity), which are theme names, so it recolours with the theme.
 * server/internal/deck/styles.go has the same table; keep the two in step.
 */
import type { Dash, Element, ThemeColor } from "./schema.ts";

export type StyleId = "filled-accent" | "filled-accent2" | "outline-accent" | "subtle" | "emphasis" | "ghost";
export type ShapeStyle = {
  id: StyleId;
  label: string;
  fill?: ThemeColor;
  stroke?: ThemeColor;
  strokeWidth?: number;
  text: ThemeColor;
  line: { stroke: ThemeColor; strokeWidth: number };
  /** The outline's and line's dash; absent is solid. */
  dash?: Dash;
};

/** In gallery order. */
export const STYLES: ShapeStyle[] = [
  { id: "filled-accent", label: "Filled accent", fill: "accent", text: "background", line: { stroke: "accent", strokeWidth: 10 } },
  { id: "filled-accent2", label: "Filled accent 2", fill: "accent2", text: "background", line: { stroke: "accent2", strokeWidth: 10 } },
  { id: "outline-accent", label: "Outline accent", stroke: "accent", strokeWidth: 6, text: "accent", line: { stroke: "accent", strokeWidth: 5 } },
  { id: "subtle", label: "Subtle", fill: "surface", stroke: "muted", strokeWidth: 3, text: "text", line: { stroke: "muted", strokeWidth: 4 } },
  { id: "emphasis", label: "Emphasis", fill: "heading", text: "background", line: { stroke: "heading", strokeWidth: 10 } },
  { id: "ghost", label: "Ghost", stroke: "muted", strokeWidth: 3, text: "muted", line: { stroke: "muted", strokeWidth: 3 }, dash: "dash" },
];

const byId = (id: StyleId) => STYLES.find((s) => s.id === id);

/** A copy of e without the given keys, so a preset leaves no opacity or dash behind. */
function without<T extends object>(e: T, ...keys: string[]): T {
  const o = { ...e } as Record<string, unknown>;
  for (const k of keys) delete o[k];
  return o as T;
}

/** The element with a style's properties set; elements other than shapes and lines are returned as they are. */
export function applyStyle(e: Element, id: StyleId): Element {
  const s = byId(id);
  if (!s) return e;
  if (e.type === "line") {
    return { ...without(e, "strokeOpacity", "dash"), stroke: s.line.stroke, strokeWidth: s.line.strokeWidth, ...(s.dash ? { dash: s.dash } : {}) };
  }
  if (e.type !== "shape") return e;
  const text = e.text && { ...e.text, paragraphs: e.text.paragraphs.map((p) => ({ ...p, runs: p.runs.map((r) => ({ ...r, color: s.text })) })) };
  return { ...without(e, "fillOpacity", "strokeOpacity", "dash"), fill: s.fill, stroke: s.stroke, strokeWidth: s.stroke ? s.strokeWidth : undefined, ...(s.dash ? { dash: s.dash } : {}), ...(text ? { text } : {}) };
}

/** The style an element matches exactly, if any, for the gallery's current tile. */
export function styleOf(e: Element): StyleId | undefined {
  return STYLES.find((s) => {
    if ((e.type === "line" || e.type === "shape") && (e.dash ?? "solid") !== (s.dash ?? "solid")) return false;
    if (e.type === "line") return e.stroke === s.line.stroke && e.strokeWidth === s.line.strokeWidth && e.strokeOpacity === undefined;
    if (e.type !== "shape") return false;
    const runs = e.text?.paragraphs.flatMap((p) => p.runs) ?? [];
    return e.fill === s.fill && e.fillOpacity === undefined && e.strokeOpacity === undefined && e.stroke === s.stroke && (s.stroke ? e.strokeWidth === s.strokeWidth : true) && runs.every((r) => r.color === s.text);
  })?.id;
}
