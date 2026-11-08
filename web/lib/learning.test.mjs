import assert from "node:assert/strict";
import { test } from "node:test";
import { buildStepIndex, courseBuilding, estimatedRuns, moveLesson, pickLesson } from "./learning.ts";

const units = [{ title: "A", lessons: ["a", "b"] }, { title: "B", lessons: ["c"] }];

test("moveLesson swaps within a unit", () => {
  assert.deepEqual(moveLesson(units, "b", -1)[0].lessons, ["b", "a"]);
});

test("moveLesson crosses unit edges and leaves the input alone", () => {
  const down = moveLesson(units, "b", 1);
  assert.deepEqual(down[0].lessons, ["a"]);
  assert.deepEqual(down[1].lessons, ["b", "c"]);
  const up = moveLesson(units, "c", -1);
  assert.deepEqual(up[0].lessons, ["a", "b", "c"]);
  assert.deepEqual(units[0].lessons, ["a", "b"]);
});

test("moveLesson can't leave the module", () => {
  assert.equal(moveLesson(units, "a", -1), units);
  assert.equal(moveLesson(units, "c", 1), units);
  assert.equal(moveLesson(units, "nope", 1), units);
});

test("estimatedRuns skips units that are all known", () => {
  assert.equal(estimatedRuns([{ lessons: [{ known: true }] }, { lessons: [{}, { known: true }] }]), 3);
});

test("pickLesson honours the request, then the first unfinished", () => {
  const l = (key, status, progress, checks = [1]) => ({ key, status, progress, checks });
  const list = [l("k", "known", "new", []), l("done", "verified", "solid"), l("next", "verified", "new"), l("gone", "flagged", "new", [])];
  assert.equal(pickLesson(list, "done").key, "done");
  assert.equal(pickLesson(list, null).key, "next");
  assert.equal(pickLesson(list, "k").key, "next");
  assert.equal(pickLesson([l("a", "verified", "solid")], null).key, "a");
  assert.equal(pickLesson([], null), undefined);
});

test("build helpers", () => {
  assert.equal(buildStepIndex("researching"), 2);
  assert.equal(buildStepIndex("failed"), -1);
  assert.ok(courseBuilding("authoring"));
  assert.ok(!courseBuilding("outline_ready"));
});
