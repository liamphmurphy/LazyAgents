import assert from "node:assert/strict";
import { test } from "node:test";
import { TIME_STEPS, buildStepIndex, courseBuilding, estimatedRuns, moveLesson, partsEstimate, pickLesson, timeDetail, timeLabel } from "./learning.ts";

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

test("the time slider runs from half an hour to 20 hours", () => {
  assert.equal(TIME_STEPS[0], 30);
  assert.equal(TIME_STEPS.at(-1), 1200);
  assert.ok(TIME_STEPS.includes(180) && TIME_STEPS.includes(480));
  assert.deepEqual([timeLabel(30), timeLabel(60), timeLabel(90), timeLabel(1200)], ["30 min", "1 hour", "1.5 hours", "20 hours"]);
});

test("a module over 8 hours is built in parts", () => {
  assert.deepEqual([partsEstimate(480), partsEstimate(540), partsEstimate(720), partsEstimate(1200)], [1, 2, 3, 5]);
  assert.match(timeDetail(1200), /about 5 parts/);
  assert.doesNotMatch(timeDetail(480), /parts/);
});
