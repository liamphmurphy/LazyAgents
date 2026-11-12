import test from "node:test";
import assert from "node:assert/strict";
import { endedToday, finishedLabel, finishedToday, livePill, stepProgress } from "./run-mobile.ts";

const step = (position, state, text = `Step ${position}`) => ({ position, state, text });

test("step progress counts done steps and finds the current one in plan order", () => {
  const p = stepProgress({ plan: [step(3, "pending"), step(1, "done"), step(2, "current", "Write it")] });
  assert.equal(p.done, 1);
  assert.equal(p.total, 3);
  assert.equal(p.current?.text, "Write it");
  assert.deepEqual(p.steps.map((s) => s.position), [1, 2, 3]);
});

test("a step needing a person counts as current; no plan has nothing", () => {
  assert.equal(stepProgress({ plan: [step(1, "done"), step(2, "needs_human", "Ask")] }).current?.text, "Ask");
  assert.deepEqual(stepProgress({ plan: [] }), { done: 0, total: 0, current: undefined, steps: [] });
});

test("finished today keeps ended runs from today's calendar day, latest first", () => {
  const now = new Date(2026, 10, 12, 15, 0);
  const at = (d, h) => new Date(2026, 10, d, h).toISOString();
  const runs = [
    { id: "a", state: "completed", ended_at: at(12, 9) },
    { id: "b", state: "failed", ended_at: at(12, 14) },
    { id: "c", state: "completed", ended_at: at(11, 23) },
    { id: "d", state: "running", ended_at: null },
    { id: "e", state: "cancelled", ended_at: at(12, 1) },
  ];
  assert.deepEqual(finishedToday(runs, now).map((r) => r.id), ["b", "a", "e"]);
  assert.equal(endedToday({ state: "completed" }, now), false);
});

test("labels: Done, Failed or Cancelled; amber Needs you only when a person unblocks the run", () => {
  assert.deepEqual(finishedLabel("completed"), { label: "Done", tone: "done" });
  assert.deepEqual(finishedLabel("failed"), { label: "Failed", tone: "blocked" });
  assert.equal(finishedLabel("cancelled").label, "Cancelled");
  assert.deepEqual(livePill({ state: "running" }), { label: "Running", tone: "done" });
  assert.deepEqual(livePill({ state: "waiting", wait_reason: "approval" }), { label: "Needs you", tone: "human" });
  assert.equal(livePill({ state: "waiting", wait_reason: "human_reply" }).label, "Needs you");
  assert.equal(livePill({ state: "waiting", wait_reason: "delegation" }).label, "Waiting");
  assert.equal(livePill({ state: "queued" }).label, "Queued");
});
