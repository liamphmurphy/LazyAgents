import test from "node:test";
import assert from "node:assert/strict";
import { STYLES, applyStyle, styleOf } from "./slides/styles.ts";
import { THEMES, resolveColor } from "./slides/schema.ts";

const THEME_COLORS = ["background", "text", "heading", "accent", "accent2", "muted", "surface"];
const shape = () => ({
  id: "e_1", type: "shape", shape: "rect", frame: { x: 0, y: 0, w: 10, h: 10 },
  fill: "#123456", stroke: "#654321", strokeWidth: 9,
  text: { valign: "middle", paragraphs: [{ runs: [{ text: "a", size: 40, color: "#FFFFFF" }, { text: "b", size: 40 }] }] },
});
const line = () => ({ id: "e_2", type: "line", frame: { x: 0, y: 0, w: 10, h: 0 }, stroke: "#000000", strokeWidth: 1 });

const want = {
  "filled-accent": { fill: "accent", stroke: undefined, strokeWidth: undefined, text: "background" },
  "filled-accent2": { fill: "accent2", stroke: undefined, strokeWidth: undefined, text: "background" },
  "outline-accent": { fill: undefined, stroke: "accent", strokeWidth: 6, text: "accent" },
  subtle: { fill: "surface", stroke: "muted", strokeWidth: 3, text: "text" },
  emphasis: { fill: "heading", stroke: undefined, strokeWidth: undefined, text: "background" },
  ghost: { fill: undefined, stroke: "muted", strokeWidth: 3, text: "muted" },
};

test("each preset maps to its fill, outline, width and text colour", () => {
  assert.deepEqual(STYLES.map((s) => s.id), Object.keys(want));
  for (const [id, w] of Object.entries(want)) {
    const e = applyStyle(shape(), id);
    assert.equal(e.fill, w.fill, id);
    assert.equal(e.stroke, w.stroke, id);
    assert.equal(e.strokeWidth, w.strokeWidth, id);
    assert.deepEqual(e.text.paragraphs[0].runs.map((r) => r.color), [w.text, w.text], id);
    assert.equal(e.text.valign, "middle");
    assert.equal(styleOf(e), id);
  }
});

test("presets use only theme colours, so they follow the theme", () => {
  for (const s of STYLES) for (const c of [s.fill, s.stroke, s.text, s.line.stroke].filter(Boolean)) assert.ok(THEME_COLORS.includes(c), `${s.id}: ${c}`);
});

test("lines take the line variant; other elements are untouched", () => {
  for (const s of STYLES) {
    const l = applyStyle(line(), s.id);
    assert.deepEqual([l.stroke, l.strokeWidth], [s.line.stroke, s.line.strokeWidth]);
  }
  const t = { id: "e_3", type: "text", frame: { x: 0, y: 0, w: 1, h: 1 }, paragraphs: [] };
  assert.equal(applyStyle(t, "subtle"), t);
  assert.equal(styleOf(t), undefined);
  assert.equal(styleOf(shape()), undefined);
});

test("a shape without text stays without text", () => {
  const { text, ...bare } = shape();
  assert.equal("text" in applyStyle(bare, "ghost"), false);
});

test("a styled shape resolves to different colours after a theme switch", () => {
  for (const s of STYLES) {
    const e = applyStyle(shape(), s.id);
    const paint = (t) => [e.fill, e.stroke, e.text.paragraphs[0].runs[0].color].filter(Boolean).map((c) => resolveColor(t, c));
    assert.notDeepEqual(paint(THEMES.light), paint(THEMES.dark), s.id);
  }
});
