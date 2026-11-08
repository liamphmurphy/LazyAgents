import assert from "node:assert/strict";
import test from "node:test";
import { chartLayout, compactNumber, isChartFence, niceTicks, parseChart, withUnit } from "./chat-chart.ts";

const bar = (over = {}) => JSON.stringify({ type: "bar", categories: ["a", "b"], series: [{ name: "s", values: [1, 2] }], ...over });

test("isChartFence matches only the chart tag", () => {
  assert.equal(isChartFence(" Chart title=x"), true);
  assert.equal(isChartFence("charts"), false);
  assert.equal(isChartFence(undefined), false);
});

test("parseChart reads a valid spec and keeps gaps", () => {
  const out = parseChart(bar({ title: " T ", y: { unit: "%" }, series: [{ name: "s", values: [1, null] }] }));
  assert.equal(out.spec.title, "T");
  assert.equal(out.spec.y.unit, "%");
  assert.deepEqual(out.spec.series[0].values, [1, null]);
});

test("parseChart refuses what it can't draw honestly", () => {
  assert.match(parseChart("{").error, /valid JSON/);
  assert.match(parseChart(bar({ type: "pie" })).error, /type/);
  assert.match(parseChart(bar({ series: [] })).error, /series/);
  assert.match(parseChart(bar({ categories: [] })).error, /categories/);
  assert.match(parseChart(bar({ series: [{ name: "s", values: [1] }] })).error, /one value per category/);
  assert.match(parseChart(bar({ series: [{ name: "s", values: [null, null] }] })).error, /one value per category/);
  assert.match(parseChart(bar({ series: [{ values: [1, 2] }, { name: "t", values: [1, 2] }] })).error, /needs a name/);
});

test("a scatter takes points, up to three series", () => {
  const s = (n) => ({ name: n, points: [[1, 2]] });
  assert.deepEqual(parseChart(JSON.stringify({ type: "scatter", series: [s("a")] })).spec.series[0].points, [[1, 2]]);
  assert.match(parseChart(JSON.stringify({ type: "scatter", series: ["a", "b", "c", "d"].map(s) })).error, /1 to 3/);
});

test("niceTicks covers the range with round numbers", () => {
  assert.deepEqual(niceTicks(0, 100), [0, 25, 50, 75, 100]);
  assert.deepEqual(niceTicks(0, 0.3, 4), [0, 0.1, 0.2, 0.3]);
  const t = niceTicks(-3, 7);
  assert.ok(t[0] <= -3 && t.at(-1) >= 7);
  assert.ok(niceTicks(5, 5).length >= 2);
});

test("numbers are compact and units go on the right side", () => {
  assert.equal(compactNumber(1500), "1.5K");
  assert.equal(compactNumber(-2500000), "-2.5M");
  assert.equal(compactNumber(12.34567), "12.35");
  assert.equal(withUnit("5", "$"), "$5");
  assert.equal(withUnit("-5", "$"), "-$5");
  assert.equal(withUnit("5", "%"), "5%");
  assert.equal(withUnit("5", "ms"), "5 ms");
});

const spec = (over = {}) => parseChart(JSON.stringify({ type: "bar", categories: ["a", "b", "c"], series: [{ name: "s", values: [1, 2, -1] }], ...over })).spec;

test("bars grow from zero, capped at 24px, rounded only at the data end", () => {
  const l = chartLayout(spec(), 640);
  assert.equal(l.horizontal, false);
  assert.equal(l.bars.length, 3);
  assert.ok(l.valueTicks.some((t) => t.label === "0"));
  assert.ok(l.baseline > l.plot.top && l.baseline < l.plot.bottom);
  const width = Number(/H([\d.]+)Q/.exec(l.bars[0].d)[1]) - Number(/^M([\d.]+)/.exec(l.bars[0].d)[1]);
  assert.ok(width <= 24, `bar width ${width}`);
  assert.ok(l.bars[2].d.includes("V"), "the negative bar is drawn");
  assert.equal(l.labels.length, 3, "a single series with few bars is labelled at the tips");
});

test("bars with long labels turn horizontal rather than collide", () => {
  const long = spec({ categories: ["A rather long category name", "Another very long category", "And a third long one"] });
  assert.equal(chartLayout(long, 340).horizontal, true);
  const l = chartLayout(spec({ categories: Array.from({ length: 4 }, (_, i) => `Category number ${i}`), series: [{ name: "s", values: [1, 2, 3, 4] }] }), 300);
  assert.equal(l.horizontal, true);
  assert.ok(l.categoryLabels.every((c) => c.label.length * 6.2 <= l.plot.left));
  assert.equal(l.nearest(150, l.plot.top + 1), 0);
});

test("a line breaks at gaps, marks points, and labels its ends", () => {
  const l = chartLayout(spec({ type: "line", series: [{ name: "up", values: [1, null, 3] }, { name: "down", values: [9, 8, 7] }] }), 640);
  assert.equal(l.lines[0].d.match(/M/g).length, 2);
  assert.equal(l.lines[0].dots.length, 2);
  assert.deepEqual(l.labels.map((t) => t.label), ["up", "down"]);
  assert.equal(chartLayout(spec({ type: "line", series: [{ name: "a", values: [1, 2, 3] }, { name: "b", values: [1, 2, 3.001] }] }), 640).labels.length, 0, "colliding end labels are dropped, not nudged");
  assert.equal(chartLayout(spec({ type: "line" }), 320).labels.length, 0, "no end labels when narrow");
});

test("category labels thin out instead of overlapping", () => {
  const many = spec({ type: "line", categories: Array.from({ length: 40 }, (_, i) => `2026-${i}`), series: [{ name: "s", values: Array.from({ length: 40 }, (_, i) => i) }] });
  const l = chartLayout(many, 400);
  assert.ok(l.categoryLabels.length < 40);
  for (let i = 1; i < l.categoryLabels.length; i++) assert.ok(l.categoryLabels[i].x - l.categoryLabels[i - 1].x >= l.categoryLabels[i].label.length * 6.2);
  assert.equal(l.lines[0].dots.length, 1, "past 24 points only the end is marked");
});

test("a scatter finds the nearest point, within reach", () => {
  const l = chartLayout(parseChart(JSON.stringify({ type: "scatter", series: [{ name: "a", points: [[0, 0], [10, 10]] }] })).spec, 640);
  assert.equal(l.positions, 2);
  const p = l.points[1];
  assert.equal(l.nearest(p.x + 3, p.y - 3), 1);
  assert.equal(l.nearest(-500, -500), null);
});
