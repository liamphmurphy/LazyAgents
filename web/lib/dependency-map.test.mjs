import test from "node:test";
import assert from "node:assert/strict";
import { dependencyMap } from "./dependency-map.ts";
const ticket = (key, points, parents = [], status = "todo") => ({ key, title: key, estimate_points: points, status, blocked_by: parents.map(key => ({ key })) });
test("depth includes all dependencies and remaining points choose the critical branch", () => {
  const g = dependencyMap([ticket("D", 3, ["B", "C"]), ticket("C", 8, ["A"]), ticket("A", 13, [], "done"), ticket("B", 2, ["A"])]);
  assert.deepEqual(Object.fromEntries(g.depth), { A: 0, B: 1, C: 1, D: 2 });
  assert.deepEqual(g.chain, { keys: ["C", "D"], points: 11 });
  assert.equal(g.edges.length, 4);
});
test("canceled blockers warn only for unfinished dependents and never add points", () => {
  const g = dependencyMap([ticket("X", 13, [], "canceled"), ticket("A", 3, ["X"]), ticket("B", 8, ["X"], "done"), ticket("C", undefined, ["A"])]);
  assert.deepEqual(g.warnings, [{ from: "X", to: "A" }]);
  assert.deepEqual(g.chain, { keys: ["A", "C"], points: 3 });
});
test("cycles and downstream tickets remain visible without infinite traversal", () => {
  const g = dependencyMap([ticket("A", 3, ["B"]), ticket("B", 5, ["A"]), ticket("C", 2, ["B"]), ticket("D", 1)]);
  assert.deepEqual(g.cyclic, ["A", "B", "C"]);
  assert.equal(g.nodes.length, 4);
  assert.deepEqual(g.chain, { keys: ["D"], points: 1 });
});
test("empty and unestimated graphs have stable zero totals", () => {
  assert.deepEqual(dependencyMap([]).chain, { keys: [], points: 0 });
  assert.deepEqual(dependencyMap([ticket("A", undefined), ticket("B", undefined, ["A"])]).chain, { keys: ["A", "B"], points: 0 });
});
