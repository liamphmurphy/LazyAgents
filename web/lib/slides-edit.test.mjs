import test from "node:test";
import assert from "node:assert/strict";
import { LIMITS, parseFontSize, parseHex, parseInRange, pushRecent, shiftLevel, toggleArrow, withRadius, withStrokeWidth } from "./slides/edit.ts";

test("custom colours accept #RGB and #RRGGBB in any case and nothing else", () => {
  assert.equal(parseHex("#c2410c"), "#C2410C");
  assert.equal(parseHex("c2410c"), "#C2410C");
  assert.equal(parseHex(" #abc "), "#AABBCC");
  for (const bad of ["", "#12345", "#GGGGGG", "red", "#1234567", "accent"]) assert.equal(parseHex(bad), undefined, bad);
});

test("recent colours put the newest first, drop duplicates and keep eight", () => {
  let r = [];
  for (const c of ["#111111", "#222222", "#333333"]) r = pushRecent(r, c);
  assert.deepEqual(r, ["#333333", "#222222", "#111111"]);
  r = pushRecent(r, "#111111");
  assert.deepEqual(r, ["#111111", "#333333", "#222222"]);
  for (let i = 0; i < 12; i++) r = pushRecent(r, `#00000${i.toString(16)}`);
  assert.equal(r.length, 8);
  assert.equal(r[0], "#00000B");
  assert.deepEqual(pushRecent(r, "nope"), r);
});

test("typed numbers outside the schema's limits are refused with a message", () => {
  assert.deepEqual(parseInRange("12", 1, 100), { value: 12 });
  assert.match(parseInRange("0", 1, 100, " px").error, /1 to 100 px/);
  assert.match(parseInRange("abc", 1, 100).error, /1 to 100/);
  assert.match(parseInRange("", 1, 100).error, /1 to 100/);
});

test("font sizes are typed in pt within the schema's 8 to 400 range", () => {
  assert.deepEqual(parseFontSize("36"), { value: 72 });
  assert.deepEqual(parseFontSize("10.5"), { value: 21 });
  assert.deepEqual(parseFontSize("4"), { value: LIMITS.fontSize.min });
  assert.deepEqual(parseFontSize("200"), { value: LIMITS.fontSize.max });
  assert.match(parseFontSize("3.5").error, /4 to 200 pt/);
  assert.match(parseFontSize("201").error, /4 to 200 pt/);
});

test("indent and outdent move only bulleted paragraphs, within levels 0 to 4", () => {
  const ps = [{ runs: [], bullet: "disc", level: 0 }, { runs: [] }, { runs: [], bullet: "number", level: 4 }];
  const inn = shiftLevel(ps, 1);
  assert.deepEqual(inn.map((p) => p.level), [1, undefined, 4]);
  const out = shiftLevel(inn, -1);
  assert.deepEqual(out.map((p) => p.level), [0, undefined, 3]);
  // Nothing to do returns the same array, so no undo step is recorded.
  const flat = [{ runs: [], bullet: "disc", level: 0 }];
  assert.equal(shiftLevel(flat, -1), flat);
  assert.equal(shiftLevel([{ runs: [] }], 1).length, 1);
});

test("outline width gives a bare shape an outline and stays within limits", () => {
  const shape = { id: "e", type: "shape", shape: "rect", frame: { x: 0, y: 0, w: 10, h: 10 } };
  assert.deepEqual(withStrokeWidth(shape, 12), { ...shape, stroke: "text", strokeWidth: 12 });
  assert.equal(withStrokeWidth({ ...shape, stroke: "accent" }, 500).strokeWidth, LIMITS.strokeWidth.max);
  const line = { id: "l", type: "line", frame: { x: 0, y: 0, w: 10, h: 0 }, stroke: "text", strokeWidth: 6 };
  assert.equal(withStrokeWidth(line, 0).strokeWidth, 1);
  assert.equal(withStrokeWidth(line, 6), line);
  const text = { id: "t", type: "text", frame: shape.frame, paragraphs: [] };
  assert.equal(withStrokeWidth(text, 4), text);
});

test("corner radius only changes rounded rectangles", () => {
  const rr = { id: "e", type: "shape", shape: "roundRect", frame: { x: 0, y: 0, w: 10, h: 10 }, radius: 32 };
  assert.equal(withRadius(rr, 64).radius, 64);
  assert.equal(withRadius(rr, 0).radius, LIMITS.radius.min);
  const rect = { ...rr, shape: "rect", radius: undefined };
  assert.equal(withRadius(rect, 64), rect);
});

test("arrowheads toggle on and off and are omitted when off", () => {
  const line = { id: "l", type: "line", frame: { x: 0, y: 0, w: 10, h: 0 }, stroke: "text", strokeWidth: 6 };
  const a = toggleArrow(line, "end");
  assert.equal(a.endArrow, true);
  assert.equal(toggleArrow(a, "end").endArrow, undefined);
  assert.equal(toggleArrow(toggleArrow(line, "start"), "end").startArrow, true);
});
