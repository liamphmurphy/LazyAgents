import assert from "node:assert/strict";
import test from "node:test";
import { align, boxOf, contains, distribute, duplicate, guidesFor, reorder, scaleFrame, snapAxis, snapMove, snapTargets, unionBox } from "./slides/arrange.ts";

const el = (id, x = 0, y = 0, w = 100, h = 100) => ({ id, type: "shape", shape: "rect", frame: { x, y, w, h } });
const ids = (els) => els.map((e) => e.id).join("");

test("boxOf normalises lines drawn right to left or upwards", () => {
  assert.deepEqual(boxOf({ x: 100, y: 50, w: -60, h: -20 }), { l: 40, t: 30, r: 100, b: 50 });
  assert.deepEqual(unionBox([{ x: 0, y: 0, w: 10, h: 10 }, { x: 20, y: -5, w: 5, h: 5 }]), { l: 0, t: -5, r: 25, b: 10 });
  assert.equal(unionBox([]), undefined);
  assert.equal(contains({ l: 0, t: 0, r: 100, b: 100 }, { l: 10, t: 10, r: 100, b: 50 }), true);
  assert.equal(contains({ l: 0, t: 0, r: 100, b: 100 }, { l: 10, t: 10, r: 101, b: 50 }), false);
});

test("reorder moves a selection one step or to an end as a block", () => {
  const els = ["a", "b", "c", "d", "e"].map((i) => el(i));
  assert.equal(ids(reorder(els, new Set(["b"]), "forward")), "acbde");
  assert.equal(ids(reorder(els, new Set(["b", "c"]), "forward")), "adbce");
  assert.equal(ids(reorder(els, new Set(["b", "d"]), "forward")), "acbed");
  assert.equal(ids(reorder(els, new Set(["c", "d"]), "backward")), "acdbe");
  assert.equal(ids(reorder(els, new Set(["b", "d"]), "front")), "acebd");
  assert.equal(ids(reorder(els, new Set(["b", "d"]), "back")), "bdace");
  // Already at the top or bottom: the same array, so it isn't an undo step.
  assert.equal(reorder(els, new Set(["e"]), "forward"), els);
  assert.equal(reorder(els, new Set(["d", "e"]), "front"), els);
  assert.equal(reorder(els, new Set(["a"]), "back"), els);
  assert.equal(reorder(els, new Set(["a"]), "backward"), els);
});

test("duplicate places offset copies with fresh ids above the topmost original", () => {
  const els = [el("a"), { ...el("b", 10, 20), placeholder: "title" }, el("c"), el("d")];
  let n = 0;
  const r = duplicate(els, new Set(["a", "b"]), () => `e_new${++n}`);
  assert.deepEqual(r.ids, ["e_new1", "e_new2"]);
  assert.equal(r.elements.map((e) => e.id).join(","), "a,b,e_new1,e_new2,c,d");
  assert.deepEqual(r.elements[3].frame, { x: 50, y: 60, w: 100, h: 100 });
  assert.equal(r.elements[3].placeholder, undefined);
  // The copy is deep: changing it leaves the original alone.
  r.elements[3].frame.x = 0;
  assert.equal(els[1].frame.x, 10);
});

test("align lines boxes up on each side and centre line", () => {
  const frames = { a: { x: 10, y: 10, w: 100, h: 50 }, b: { x: 200, y: 100, w: 50, h: 100 } };
  const to = { l: 10, t: 10, r: 250, b: 200 };
  assert.deepEqual(align(frames, "left", to), { a: { x: 10, y: 10, w: 100, h: 50 }, b: { x: 10, y: 100, w: 50, h: 100 } });
  assert.deepEqual(align(frames, "right", to), { a: { x: 150, y: 10, w: 100, h: 50 }, b: { x: 200, y: 100, w: 50, h: 100 } });
  assert.deepEqual(align(frames, "center", to), { a: { x: 80, y: 10, w: 100, h: 50 }, b: { x: 105, y: 100, w: 50, h: 100 } });
  assert.deepEqual(align(frames, "top", to).b, { x: 200, y: 10, w: 50, h: 100 });
  assert.deepEqual(align(frames, "bottom", to).a, { x: 10, y: 150, w: 100, h: 50 });
  assert.deepEqual(align(frames, "middle", to).a, { x: 10, y: 80, w: 100, h: 50 });
  // A lone element aligns to the slide.
  assert.deepEqual(align({ a: { x: 10, y: 10, w: 100, h: 50 } }, "center", { l: 0, t: 0, r: 1920, b: 1080 }).a, { x: 910, y: 10, w: 100, h: 50 });
  // Lines with a negative width align by their visible extent.
  assert.deepEqual(align({ l: { x: 300, y: 0, w: -100, h: 0 } }, "left", to).l, { x: 110, y: 0, w: -100, h: 0 });
});

test("distribute spaces three or more boxes evenly between the outer two", () => {
  const frames = { a: { x: 0, y: 0, w: 100, h: 10 }, c: { x: 900, y: 5, w: 100, h: 10 }, b: { x: 150, y: 9, w: 200, h: 10 } };
  const r = distribute(frames, "x");
  assert.deepEqual(r.a, frames.a);
  assert.deepEqual(r.c, frames.c);
  // Gaps of (1000 - 400) / 2 = 300: b starts at 400.
  assert.deepEqual(r.b, { x: 400, y: 9, w: 200, h: 10 });
  const v = distribute({ a: { x: 0, y: 0, w: 10, h: 10 }, b: { x: 0, y: 12, w: 10, h: 30 }, c: { x: 0, y: 30, w: 10, h: 10 }, d: { x: 0, y: 100, w: 10, h: 20 } }, "y");
  // Total 120, sizes 70, three gaps of 50/3.
  assert.equal(v.a.y, 0);
  assert.equal(v.b.y, 27);
  assert.equal(v.c.y, 73);
  assert.equal(v.d.y, 100);
  // Two boxes: nothing to distribute.
  assert.deepEqual(distribute({ a: frames.a, b: frames.b }, "x"), { a: frames.a, b: frames.b });
});

test("scaleFrame maps frames from a group box onto a resized one", () => {
  const from = { l: 100, t: 100, r: 300, b: 200 };
  const to = { l: 100, t: 100, r: 500, b: 300 };
  assert.deepEqual(scaleFrame({ x: 200, y: 150, w: 100, h: 50 }, from, to), { x: 300, y: 200, w: 200, h: 100 });
  assert.deepEqual(scaleFrame({ x: 300, y: 100, w: -200, h: 0 }, from, to), { x: 500, y: 100, w: -400, h: 0 });
});

test("snapAxis picks the nearest target within the threshold", () => {
  assert.deepEqual(snapAxis([95, 145], [0, 100, 150], 6), { d: 5, at: 100 });
  assert.deepEqual(snapAxis([97, 148], [0, 100, 150], 6), { d: 2, at: 150 });
  assert.equal(snapAxis([90], [0, 100], 6), undefined);
});

test("snapMove snaps to slide centre lines and other elements' edges, with guides", () => {
  const targets = snapTargets(1920, 1080, [{ x: 100, y: 100, w: 200, h: 100 }]);
  assert.ok(targets.x.includes(960) && targets.x.includes(300) && targets.y.includes(150));
  // A 200-wide box centred at 955 snaps 5 right onto the slide's centre.
  const r = snapMove({ l: 855, t: 497, r: 1055, b: 577 }, targets, 8);
  assert.equal(r.dx, 5);
  assert.equal(r.dy, 3);
  assert.deepEqual(r.guides.x, [960]);
  assert.deepEqual(r.guides.y, [540]);
  // Its left edge near the other element's right edge.
  const e = snapMove({ l: 304, t: 700, r: 404, b: 760 }, targets, 8);
  assert.equal(e.dx, -4);
  assert.deepEqual(e.guides.x, [300]);
  assert.deepEqual(e.guides.y, []);
  assert.deepEqual(guidesFor({ l: 0, t: 0, r: 10, b: 10 }, targets), { x: [0], y: [0] });
});
