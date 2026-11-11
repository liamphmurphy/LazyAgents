import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { LINE, layoutSlide, layoutText } from "./slides/layout.ts";
import { THEMES } from "./slides/schema.ts";

// Every character is half its font size wide, so lines break predictably.
const measure = (text, _slot, _style, size) => text.length * 0.5 * size;
const theme = THEMES.light;
const frame = { x: 0, y: 0, w: 432, h: 1000 }; // 400 units inside the 16-unit insets: 20 characters at size 40
const box = (p, runs) => ({ paragraphs: [{ ...p, runs: runs ?? [{ text: "aaaa bbbb cccc dddd eeee ffff gggg", size: 40 }] }] });
const texts = (ops) => ops.filter((o) => o.op === "text");

test("line spacing multiplies the line height and the box height", () => {
  const single = layoutText(theme, box({}), frame, measure, "e");
  const double = layoutText(theme, box({ lineSpacing: 2 }), frame, measure, "e");
  assert.ok(Math.abs(single.height - 2 * 40 * LINE) < 1e-9);
  const near = (a, b) => assert.ok(Math.abs(a - b) < 1e-9, `${a} ≉ ${b}`);
  near(double.height, 2 * single.height);
  const ys = (r) => [...new Set(texts(r.ops).map((o) => o.y))];
  near(ys(double)[1] - ys(double)[0], 2 * (ys(single)[1] - ys(single)[0]));
});

test("overflow honours line spacing", () => {
  const tight = { ...frame, h: 2 * 40 * LINE + 32 }; // exactly two single-spaced lines
  assert.equal(layoutText(theme, box({}), tight, measure, "e").overflow, false);
  assert.equal(layoutText(theme, box({ lineSpacing: 1.15 }), tight, measure, "e").overflow, true);
  assert.equal(layoutText(theme, box({ lineSpacing: 0.8 }), tight, measure, "e").overflow, false);
});

test("justified lines fill the box except the paragraph's last line", () => {
  const { ops } = layoutText(theme, box({ align: "justify" }), frame, measure, "e");
  const t = texts(ops);
  const first = t.filter((o) => o.y === t[0].y);
  const last = t.filter((o) => o.y !== t[0].y);
  // First line "aaaa bbbb cccc dddd " (4 words): three gaps share the 20 units of room.
  assert.deepEqual(first.map((o) => o.x), [16, 16 + 100 + 20 / 3, 16 + 200 + 40 / 3, 16 + 300 + 20]);
  const end = first.at(-1);
  assert.ok(Math.abs(end.x + end.w - (frame.w - 16)) < 1e-9, "the first line ends at the right inset");
  // The last line stays left-aligned at its natural spacing.
  assert.deepEqual(last.map((o) => o.x), [16, 116, 216]);
});

test("a justified line broken inside one long word stays left-aligned", () => {
  const { ops } = layoutText(theme, box({ align: "justify" }, [{ text: "x".repeat(30), size: 40 }]), frame, measure, "e");
  assert.deepEqual(texts(ops).map((o) => o.x), [16, 16]);
});

test("strike and highlight ride on text ops; highlights draw first and span the space between highlighted words", () => {
  const { ops } = layoutText(theme, box({}, [{ text: "one two ", size: 40, highlight: "accent", strike: true }, { text: "three", size: 40 }]), frame, measure, "e");
  const hl = ops.filter((o) => o.op === "highlight");
  assert.equal(ops[0].op, "highlight");
  assert.equal(hl.length, 2);
  assert.equal(hl[0].fill, theme.colors.accent);
  assert.equal(hl[0].w, 80); // "one " up to where "two" starts
  assert.equal(hl[1].w, 60); // "two" without its trailing space: "three" isn't highlighted
  const t = texts(ops);
  assert.deepEqual(t.map((o) => o.strike), [true, true, false]);
});

test("the acceptance deck's text slide flags only the double-spaced box", () => {
  const deck = JSON.parse(readFileSync(new URL("../../docs/slides/fixtures/acceptance-deck.json", import.meta.url), "utf8"));
  const slide = deck.slides.find((s) => s.id === "s_text");
  const laid = layoutSlide(deck, slide, measure);
  assert.deepEqual([...laid.overflow], ["e_y3"]);
  const single = structuredClone(slide);
  delete single.elements[2].paragraphs[0].lineSpacing;
  assert.deepEqual([...layoutSlide(deck, single, measure).overflow], []);
  const ops = laid.ops.filter((o) => o.el === "e_y2");
  assert.ok(ops.some((o) => o.op === "highlight") && ops.some((o) => o.op === "text" && o.strike));
});
