import test from "node:test";
import assert from "node:assert/strict";
import { runCost, spendCost } from "./format.ts";

test("unpriced usage stays unknown and mixed totals stay partial", () => {
  assert.equal(runCost(0, "unknown"), "Unknown");
  assert.equal(runCost(0), "Unknown");
  assert.equal(runCost(0, "harness"), "$0.00");
  assert.equal(runCost(1.25, "computed"), "~$1.25");
  assert.equal(runCost(1.25, "partial"), "≥ $1.25 (partial)");
  assert.equal(spendCost(0, 1), "Unknown");
  assert.equal(spendCost(1.25, 1), "≥ $1.25 (partial)");
  assert.equal(spendCost(0, 0), "$0.00");
});
