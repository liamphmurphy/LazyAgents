import test from "node:test";
import assert from "node:assert/strict";
import { isDispatchFence, parseDispatch, planLine } from "./chat-dispatch.ts";

test("a dispatch fence names one ticket key", () => {
  assert.equal(isDispatchFence("dispatch"), true);
  assert.equal(isDispatchFence(" Dispatch extra"), true);
  assert.equal(isDispatchFence("dispatcher"), false);
  assert.equal(parseDispatch("pay-31\n"), "PAY-31");
  assert.equal(parseDispatch("  WEB-3  \nignored"), "WEB-3");
  assert.equal(parseDispatch("PAY-"), null, "incomplete while streaming");
  assert.equal(parseDispatch("not a key"), null);
});

test("the plan line says where the agent is", () => {
  assert.equal(planLine({ steps_total: 0, steps_done: 0 }), "");
  assert.equal(planLine({ steps_total: 4, steps_done: 1, current_step: "Fix Round()" }), "Step 2 of 4 · Fix Round()");
  assert.equal(planLine({ steps_total: 4, steps_done: 4 }), "4 of 4 steps done");
});
