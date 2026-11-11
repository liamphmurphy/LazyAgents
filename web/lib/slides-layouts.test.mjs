import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { layoutSlide } from "./slides/layout.ts";
import { NEW_LAYOUTS, isImageSlot, placeholders, relayout } from "./slides/schema.ts";

const deck = JSON.parse(readFileSync(new URL("../../docs/slides/fixtures/acceptance-deck.json", import.meta.url), "utf8"));
const measure = (text, _slot, _style, size) => text.length * 0.5 * size;
const frames = (els) => Object.fromEntries(els.filter((e) => e.placeholder).map((e) => [e.placeholder, e.frame]));

test("the acceptance deck has a slide per new layout, placed where the layout puts its placeholders", () => {
  const used = new Set(deck.slides.map((s) => s.layout));
  for (const l of NEW_LAYOUTS) assert.ok(used.has(l), `no ${l} slide`);
  for (const s of deck.slides.filter((s) => NEW_LAYOUTS.has(s.layout))) {
    assert.deepEqual(frames(s.elements), frames(placeholders(s.layout)), s.layout);
  }
});

test("an empty image slot draws nothing; relayout keeps it only where the layout has one", () => {
  const slide = { id: "s_x", layout: "image-left", elements: placeholders("image-left") };
  const slot = slide.elements.find(isImageSlot);
  assert.ok(slot);
  assert.equal(layoutSlide(deck, slide, measure).ops.filter((o) => o.el === slot.id).length, 0);
  const right = relayout(slide, "image-right");
  assert.deepEqual(right.elements.find(isImageSlot).frame, { x: 1000, y: 120, w: 800, h: 840 });
  assert.equal(relayout(slide, "agenda").elements.some(isImageSlot), false);
  // A filled image is not a slot: it stays and moves with its role.
  const filled = { ...slide, elements: slide.elements.map((e) => (isImageSlot(e) ? { id: e.id, type: "image", asset: "a_landscape", placeholder: "image", frame: e.frame } : e)) };
  assert.deepEqual(relayout(filled, "image-right").elements.find((e) => e.type === "image").frame, { x: 1000, y: 120, w: 800, h: 840 });
});
