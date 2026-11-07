import assert from "node:assert/strict";
import test from "node:test";
import { axisMax, costPerPoint, OTHER, presetRange, rangeError, share, spendLabel, stackSeries, usd } from "./costs.ts";

test("money formats from integer micros and small spend never reads free", () => {
  assert.equal(usd(412_370_000), "$412.37");
  assert.equal(usd(1_234_560_000), "$1,234.56");
  assert.equal(usd(0), "$0.00");
  assert.equal(usd(4_000), "$0.004");
  assert.equal(usd(400), "< $0.001");
});

test("unpriced runs make spend a lower bound or unknown", () => {
  assert.equal(spendLabel(2_000_000, 0), "$2.00");
  assert.equal(spendLabel(2_000_000, 1), "≥ $2.00");
  assert.equal(spendLabel(0, 2), "Unknown");
});

test("share and cost per point guard empty denominators", () => {
  assert.equal(share(1, 4), "25.0%");
  assert.equal(share(1, 0), "–");
  assert.equal(costPerPoint(38_420_000, 8), "$4.80");
  assert.equal(costPerPoint(10_000_000, undefined), undefined);
  assert.equal(costPerPoint(10_000_000, 0), undefined);
});

test("presets end today in UTC and custom ranges are validated", () => {
  assert.deepEqual(presetRange(30, new Date("2026-11-06T23:30:00-08:00")), { from: "2026-10-09", to: "2026-11-07" });
  assert.deepEqual(presetRange(1, new Date("2026-11-07T01:00:00Z")), { from: "2026-11-07", to: "2026-11-07" });
  assert.equal(rangeError("2026-11-01", "2026-11-07"), undefined);
  assert.match(rangeError("2026-11-07", "2026-11-01"), /before/);
  assert.match(rangeError("2025-01-01", "2026-11-01"), /366/);
  assert.match(rangeError("", "2026-11-01"), /Pick/);
});

test("the chart keeps the top series and folds the rest into Other, preserving day totals", () => {
  const days = [
    { date: "2026-11-01", runs: 3, cost_micros: 70, agents: [{ key: "a", cost_micros: 40 }, { key: "b", cost_micros: 20 }, { key: "c", cost_micros: 10 }], models: [] },
    { date: "2026-11-02", runs: 1, cost_micros: 5, agents: [{ key: "d", cost_micros: 5 }], models: [] },
    { date: "2026-11-03", runs: 0, cost_micros: 0, agents: [], models: [] },
  ];
  const s = stackSeries(days, "agents", 3);
  assert.deepEqual(s.keys, ["a", "b", OTHER]);
  assert.deepEqual(s.bars[0].segments.map((x) => x.cost_micros), [40, 20, 10]);
  assert.deepEqual(s.bars[1].segments.map((x) => x.cost_micros), [0, 0, 5]);
  assert.equal(s.bars[2].total, 0);
  for (const bar of s.bars) assert.equal(bar.segments.reduce((t, x) => t + x.cost_micros, 0), bar.total);
  assert.deepEqual(s.totals.map((x) => x.cost_micros), [40, 20, 15]);
  // Within the limit there is no Other.
  assert.deepEqual(stackSeries(days, "agents", 4).keys, ["a", "b", "c", "d"]);
});

test("the axis rounds up to a readable maximum", () => {
  assert.equal(axisMax(24_100_000), 25_000_000);
  assert.equal(axisMax(31_000_000), 50_000_000);
  assert.equal(axisMax(0), 1_000_000);
  assert.equal(axisMax(900_000), 1_000_000);
});
