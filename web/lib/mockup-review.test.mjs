import test from "node:test";
import assert from "node:assert/strict";
import { actualView, clampScale, fitView, fromFraction, MAX_SCALE, MIN_SCALE, svgSize, swipeKey, toFraction, zoomAt, startView } from "./mockup-review.ts";

test("svgSize reads width/height, then viewBox, then defaults", () => {
  assert.deepEqual(svgSize('<svg xmlns="http://www.w3.org/2000/svg" width="1440" height="900">'), { w: 1440, h: 900 });
  assert.deepEqual(svgSize("<svg width='320px' height='200px' viewBox='0 0 10 10'/>"), { w: 320, h: 200 });
  assert.deepEqual(svgSize('<?xml version="1.0"?>\n<svg viewBox="0 0 800 600">'), { w: 800, h: 600 });
  assert.deepEqual(svgSize('<svg width="100%" height="100%" viewBox="0,0,400,300">'), { w: 400, h: 300 });
  assert.deepEqual(svgSize('<svg width="600" viewBox="0 0 400 300">'), { w: 600, h: 450 });
  assert.deepEqual(svgSize("<svg>"), { w: 1280, h: 800 });
  assert.deepEqual(svgSize("not svg"), { w: 1280, h: 800 });
});

test("fitView centres the whole drawing inside the padding", () => {
  const v = fitView({ w: 1000, h: 500 }, { w: 1064, h: 800 }, 32);
  assert.equal(v.scale, 1);
  assert.equal(v.x, 32);
  assert.equal(v.y, 150);
  assert.equal(fitView({ w: 2000, h: 1000 }, { w: 1064, h: 800 }, 32).scale, 0.5);
});

test("actualView is 100%, centred when it fits and pinned to the padding when not", () => {
  assert.deepEqual(actualView({ w: 200, h: 100 }, { w: 1000, h: 500 }), { scale: 1, x: 400, y: 200 });
  assert.deepEqual(actualView({ w: 2000, h: 100 }, { w: 1000, h: 500 }, 32), { scale: 1, x: 32, y: 200 });
});

test("zoomAt keeps the point under the cursor fixed and clamps the scale", () => {
  const v = { x: 100, y: 50, scale: 1 };
  const z = zoomAt(v, 2, 300, 250);
  assert.equal(z.scale, 2);
  // The drawing point under (300, 250) was (200, 200); it still is.
  assert.deepEqual(toFraction(z, { w: 400, h: 400 }, 300, 250), toFraction(v, { w: 400, h: 400 }, 300, 250));
  assert.equal(zoomAt(v, 1000, 0, 0).scale, MAX_SCALE);
  assert.equal(zoomAt(v, 0.0001, 0, 0).scale, MIN_SCALE);
  assert.equal(clampScale(1), 1);
});

test("toFraction and fromFraction round-trip, and off-drawing points are null", () => {
  const view = { x: 40, y: 20, scale: 0.5 };
  const size = { w: 1000, h: 600 };
  assert.deepEqual(toFraction(view, size, 290, 170), { x: 0.5, y: 0.5 });
  assert.deepEqual(fromFraction(view, size, { x: 0.5, y: 0.5 }), { left: 290, top: 170 });
  assert.equal(toFraction(view, size, 39, 100), null);
  assert.equal(toFraction(view, size, 100, 321), null);
});

test("swipeKey moves the handle and clamps it", () => {
  assert.equal(swipeKey(0.5, "ArrowLeft"), 0.48);
  assert.equal(swipeKey(0.5, "ArrowRight", true), 0.6);
  assert.equal(swipeKey(0.05, "ArrowLeft", true), 0);
  assert.equal(swipeKey(0.3, "Home"), 0);
  assert.equal(swipeKey(0.3, "End"), 1);
  assert.equal(swipeKey(0.3, "a"), null);
});

test("startView opens a little zoomed out and never enlarges", () => {
  const small = startView({ w: 360, h: 640 }, { w: 1400, h: 900 });
  assert.equal(small.scale, 0.85);
  assert.equal(small.x, (1400 - 360 * 0.85) / 2);
  assert.equal(small.y, (900 - 640 * 0.85) / 2);
  const large = startView({ w: 2000, h: 1000 }, { w: 1400, h: 900 }, 32);
  assert.equal(large.scale, (1400 - 64) / 2000);
});

test("startView opens something very tall at its top, still readable", () => {
  const tall = startView({ w: 760, h: 6000 }, { w: 1400, h: 900 }, 32);
  assert.equal(tall.scale, 0.6);
  assert.equal(tall.y, 32);
  assert.equal(tall.x, (1400 - 760 * 0.6) / 2);
});
