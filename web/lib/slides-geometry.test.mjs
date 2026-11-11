import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { SHAPES, isPreset, shapeGeometry, svgPath } from "./slides/geometry.ts";

const frames = [
  { x: 0, y: 0, w: 400, h: 300 },
  { x: 120, y: 80, w: 900, h: 200 }, // wide
  { x: 40, y: 500, w: 160, h: 700 }, // tall
  { x: 760, y: 390, w: 300, h: 300 }, // square
];

/** Points along every segment, Béziers sampled, so the curve itself is checked rather than its handles. */
function points(segs) {
  const out = [];
  let cur = [0, 0];
  for (const s of segs) {
    if (s[0] === "M" || s[0] === "L") out.push((cur = [s[1], s[2]]));
    else if (s[0] === "C") {
      const [x0, y0] = cur;
      for (let i = 1; i <= 16; i++) {
        const t = i / 16, u = 1 - t;
        out.push([u * u * u * x0 + 3 * u * u * t * s[1] + 3 * u * t * t * s[3] + t * t * t * s[5], u * u * u * y0 + 3 * u * u * t * s[2] + 3 * u * t * t * s[4] + t * t * t * s[6]]);
      }
      cur = [s[5], s[6]];
    }
  }
  return out;
}

test("the shape list matches the server's enum", () => {
  const go = readFileSync(new URL("../../server/internal/deck/deck.go", import.meta.url), "utf8");
  const block = go.match(/Shapes = \[\]string\{([^}]*)\}/);
  assert.ok(block, "Shapes not found in deck.go");
  const server = [...block[1].matchAll(/"([A-Za-z0-9]+)"/g)].map((m) => m[1]);
  assert.deepEqual(SHAPES.map((s) => s.id), server);
  assert.equal(SHAPES.filter((s) => isPreset(s.id)).length, 18);
});

for (const { id } of SHAPES) {
  test(`${id}: closed, within its frame, text box inside`, () => {
    for (const f of frames) {
      const { segs, text } = shapeGeometry(id, f);
      assert.equal(segs[0][0], "M", "starts with a move");
      assert.equal(segs.at(-1)[0], "Z", "ends closed");
      segs.forEach((s, i) => i > 0 && s[0] === "M" && assert.equal(segs[i - 1][0], "Z", "every subpath is closed"));
      for (const s of segs) for (const v of s.slice(1)) assert.ok(Number.isFinite(v), `${id} has a non-finite coordinate`);
      // PowerPoint's default callouts point their tail below the box (adj2 = 62.5% from the centre).
      const bottom = id.startsWith("wedge") ? f.y + 1.125 * f.h : f.y + f.h;
      // PowerPoint's heart bulges 0.35% past the frame's sides and its lobes stop just short of the top; we match it rather than redraw it.
      const e = id === "heart" ? f.w * 0.005 : 0.01;
      const pts = points(segs);
      for (const [x, y] of pts) {
        assert.ok(x >= f.x - e && x <= f.x + f.w + e && y >= f.y - e && y <= bottom + e, `${id} point ${x},${y} outside ${JSON.stringify(f)}`);
      }
      // The outline reaches every edge of the frame (the callout's body spans it; its tail goes past).
      const xs = pts.map((p) => p[0]), ys = pts.map((p) => p[1]);
      assert.ok(Math.abs(Math.min(...xs) - f.x) < 1 + e && Math.abs(Math.max(...xs) - f.x - f.w) < 1 + e, `${id} doesn't span the width`);
      assert.ok(Math.abs(Math.min(...ys) - f.y) < (id === "heart" ? f.h * 0.02 : 1) && Math.max(...ys) >= f.y + f.h - 1, `${id} doesn't span the height`);
      assert.ok(text.w > 0 && text.h > 0, `${id} text box is empty`);
      assert.ok(text.x >= f.x - 0.01 && text.y >= f.y - 0.01 && text.x + text.w <= f.x + f.w + 0.01 && text.y + text.h <= f.y + f.h + 0.01, `${id} text box leaves the frame`);
      assert.match(svgPath(segs), /^M[^Z]*.*Z$/);
    }
  });
}

test("text rectangles follow PowerPoint's", () => {
  const f = { x: 0, y: 0, w: 1200, h: 1200 };
  const r = (s) => Object.values(shapeGeometry(s, f).text).map((v) => Math.round(v));
  assert.deepEqual(r("triangle"), [300, 600, 600, 600]);
  assert.deepEqual(r("diamond"), [300, 300, 600, 600]);
  assert.deepEqual(r("hexagon"), [200, 200, 800, 800]);
  assert.deepEqual(r("rightArrow"), [0, 300, 900, 600]);
  assert.deepEqual(r("heart"), [200, 300, 800, 500]);
  assert.deepEqual(r("rect"), [0, 0, 1200, 1200]);
});
