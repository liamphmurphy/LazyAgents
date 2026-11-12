import test from "node:test";
import assert from "node:assert/strict";
import { liveStrip, stepOf, stripLabel, stripWork } from "./live-run-strip.ts";

const plan = (...states) => states.map((state, i) => ({ position: i + 1, text: `s${i}`, state }));
const run = (id, started_at, over = {}) => ({ id, ticket_key: id.toUpperCase(), state: "running", started_at, agent: { name: "Keel" }, plan: [], ...over });

test("no live run gives no strip", () => {
  assert.equal(liveStrip([]), null);
});

test("the strip shows the newest run and counts the others", () => {
  const s = liveStrip([run("a-1", "2026-11-12T10:00:00Z"), run("b-2", "2026-11-12T12:00:00Z"), run("c-3", "2026-11-12T11:00:00Z")]);
  assert.equal(s.run.id, "b-2");
  assert.equal(s.more, 2);
});

test("step N of M follows the current step", () => {
  assert.deepEqual(stepOf(plan("done", "current", "pending")), { step: 2, total: 3 });
  assert.deepEqual(stepOf(plan("done", "pending")), { step: 2, total: 2 });
  assert.deepEqual(stepOf(plan("done", "done")), { step: 2, total: 2 });
  assert.deepEqual(stepOf([]), { step: null, total: 0 });
});

test("the label names agent, work and step", () => {
  const s = liveStrip([run("vesper-11", "2026-11-12T10:00:00Z", { plan: plan("done", "current", "pending", "pending") })]);
  assert.equal(stripLabel(s), "Keel on VESPER-11, step 2 of 4");
  assert.equal(stripLabel(liveStrip([run("x", "2026-11-12T10:00:00Z", { state: "waiting" })])), "Keel on X, waiting");
  assert.equal(stripWork({ ...s.run, ticket_key: "", work_title: "Plan Q1" }), "Plan Q1");
});
