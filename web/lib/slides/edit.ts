/**
 * Pure helpers behind the editor's toolbar controls: the schema's limits
 * (server/internal/deck/deck.go), parsing what a person types, and the element
 * edits each control makes. No schema fields here, only the existing ones.
 */
import type { Element, Paragraph } from "./schema.ts";

/** The limits server/internal/deck enforces, in slide units. */
export const LIMITS = {
  /** Run size in slide units; the toolbar shows half of it as pt. */
  fontSize: { min: 8, max: 400 },
  /** Outline and line width; 0 means no outline, so the controls start at 1. */
  strokeWidth: { min: 1, max: 100 },
  /** 0 is omitted on save and reads back as the default 24, so the controls start at 1. */
  radius: { min: 1, max: 1000 },
  level: { min: 0, max: 4 },
  /** Paragraph line spacing, a multiple of the default; the server allows the same range. */
  lineSpacing: { min: 0.8, max: 3 },
} as const;

export const RECENT_COLORS = 8;

/** "#abc", "abc", "#aabbcc" or "AABBCC" as "#AABBCC"; anything else is undefined. */
export function parseHex(raw: string): string | undefined {
  const s = raw.trim().replace(/^#/, "");
  if (/^[0-9a-f]{3}$/i.test(s)) return `#${[...s].map((c) => c + c).join("")}`.toUpperCase();
  if (/^[0-9a-f]{6}$/i.test(s)) return `#${s}`.toUpperCase();
  return undefined;
}

/** The recent colours with c first, without duplicates, at most RECENT_COLORS. */
export function pushRecent(recent: readonly string[], c: string): string[] {
  const hex = parseHex(c);
  if (!hex) return [...recent];
  return [hex, ...recent.filter((x) => x.toUpperCase() !== hex)].slice(0, RECENT_COLORS);
}

/**
 * A number a person typed, if it's within [min, max]; otherwise why not.
 * Values are refused rather than clamped so nothing changes by surprise.
 */
export function parseInRange(raw: string, min: number, max: number, unit = ""): { value: number } | { error: string } {
  const v = Number(raw.trim());
  if (raw.trim() === "" || !Number.isFinite(v)) return { error: `Enter a number from ${min} to ${max}${unit}.` };
  if (v < min || v > max) return { error: `Enter a number from ${min} to ${max}${unit}.` };
  return { value: v };
}

/** A typed pt size (shown as half the slide-unit size) as a run size, or why not. */
export function parseFontSize(raw: string): { value: number } | { error: string } {
  const r = parseInRange(raw, LIMITS.fontSize.min / 2, LIMITS.fontSize.max / 2, " pt");
  return "error" in r ? r : { value: Math.round(r.value * 2) };
}

/** Moves bulleted paragraphs in or out a level, within 0 to 4; others stay put. */
export function shiftLevel(paragraphs: Paragraph[], delta: number): Paragraph[] {
  let changed = false;
  const out = paragraphs.map((p) => {
    if (!p.bullet) return p;
    const level = Math.max(LIMITS.level.min, Math.min(LIMITS.level.max, (p.level ?? 0) + delta));
    if (level === (p.level ?? 0)) return p;
    changed = true;
    return { ...p, level };
  });
  return changed ? out : paragraphs;
}

const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, Math.round(v * 100) / 100));

/** Sets a shape's outline or a line's width. A shape without an outline gets one in the text colour. */
export function withStrokeWidth(e: Element, w: number): Element {
  const sw = clamp(w, LIMITS.strokeWidth.min, LIMITS.strokeWidth.max);
  if (e.type === "line") return e.strokeWidth === sw ? e : { ...e, strokeWidth: sw };
  if (e.type === "shape") return e.strokeWidth === sw && e.stroke ? e : { ...e, stroke: e.stroke ?? "text", strokeWidth: sw };
  return e;
}

/** Sets a rounded rectangle's corner radius. */
export function withRadius(e: Element, r: number): Element {
  if (e.type !== "shape" || e.shape !== "roundRect") return e;
  const radius = clamp(r, LIMITS.radius.min, LIMITS.radius.max);
  return e.radius === radius ? e : { ...e, radius };
}

/** Turns a line's start or end arrowhead on or off. */
export function toggleArrow(e: Element, end: "start" | "end"): Element {
  if (e.type !== "line") return e;
  const k = end === "start" ? "startArrow" : "endArrow";
  return { ...e, [k]: e[k] ? undefined : true };
}
