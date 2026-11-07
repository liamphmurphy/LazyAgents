import test from "node:test";
import assert from "node:assert/strict";
import { dependencyProblems } from "./proposals.ts";
const a = { ref: "a", title: "First", depends_on: ["b"] };
const b = { ref: "b", title: "Second", depends_on: [] };
test("acceptance requires dependency closure including missing refs", () => {
  assert.match(
    dependencyProblems([a, b], new Set(["a"]))[0],
    /Second.*not selected/,
  );
  assert.match(dependencyProblems([a], new Set(["a"]))[0], /b.*not selected/);
  assert.deepEqual(dependencyProblems([a, b], new Set(["a", "b"])), []);
  assert.deepEqual(dependencyProblems([a, b], new Set(["b"])), []);
});
test("cycles block selected drafts, but not an unrelated selection", () => {
  const cycle = [
    a,
    { ...b, depends_on: ["a"] },
    { ref: "c", title: "Independent" },
  ];
  assert.match(dependencyProblems(cycle, new Set(["a", "b"]))[0], /cycle/);
  assert.deepEqual(dependencyProblems(cycle, new Set(["c"])), []);
  assert.match(
    dependencyProblems([{ ...a, depends_on: ["a"] }], new Set(["a"]))[0],
    /cycle/,
  );
});
