/**
 * Shape style presets: named bundles of theme colour tokens. They're applied,
 * not referenced: the element keeps the resulting fill, stroke, strokeWidth
 * and run colours, which are theme names, so it recolours with the theme.
 * server/internal/deck/styles.go has the same table; keep the two in step.
 */
import type { Element, ThemeColor } from "./schema.ts";

export type StyleId = "filled-accent" | "filled-accent2" | "outline-accent" | "subtle" | "emphasis" | "ghost";
export type ShapeStyle = {
  id: StyleId;
  label: string;
  fill?: ThemeColor;
  stroke?: ThemeColor;
  strokeWidth?: number;
  text: ThemeColor;
  line: { stroke: ThemeColor; strokeWidth: number };
};

/** In gallery order. Ghost would use a dashed outline; it ships solid until dashes land. */
export const STYLES: ShapeStyle[] = [
  { id: "filled-accent", label: "Filled accent", fill: "accent", text: "background", line: { stroke: "accent", strokeWidth: 10 } },
  { id: "filled-accent2", label: "Filled accent 2", fill: "accent2", text: "background", line: { stroke: "accent2", strokeWidth: 10 } },
  { id: "outline-accent", label: "Outline accent", stroke: "accent", strokeWidth: 6, text: "accent", line: { stroke: "accent", strokeWidth: 5 } },
  { id: "subtle", label: "Subtle", fill: "surface", stroke: "muted", strokeWidth: 3, text: "text", line: { stroke: "muted", strokeWidth: 4 } },
  { id: "emphasis", label: "Emphasis", fill: "heading", text: "background", line: { stroke: "heading", strokeWidth: 10 } },
  { id: "ghost", label: "Ghost", stroke: "muted", strokeWidth: 3, text: "muted", line: { stroke: "muted", strokeWidth: 3 } },
];

const byId = (id: StyleId) => STYLES.find((s) => s.id === id);

/** The element with a style's properties set; elements other than shapes and lines are returned as they are. */
export function applyStyle(e: Element, id: StyleId): Element {
  const s = byId(id);
  if (!s) return e;
  if (e.type === "line") return { ...e, stroke: s.line.stroke, strokeWidth: s.line.strokeWidth };
  if (e.type !== "shape") return e;
  const text = e.text && { ...e.text, paragraphs: e.text.paragraphs.map((p) => ({ ...p, runs: p.runs.map((r) => ({ ...r, color: s.text })) })) };
  return { ...e, fill: s.fill, stroke: s.stroke, strokeWidth: s.stroke ? s.strokeWidth : undefined, ...(text ? { text } : {}) };
}

/** The style an element matches exactly, if any, for the gallery's current tile. */
export function styleOf(e: Element): StyleId | undefined {
  return STYLES.find((s) => {
    if (e.type === "line") return e.stroke === s.line.stroke && e.strokeWidth === s.line.strokeWidth;
    if (e.type !== "shape") return false;
    const runs = e.text?.paragraphs.flatMap((p) => p.runs) ?? [];
    return e.fill === s.fill && e.stroke === s.stroke && (s.stroke ? e.strokeWidth === s.strokeWidth : true) && runs.every((r) => r.color === s.text);
  })?.id;
}
