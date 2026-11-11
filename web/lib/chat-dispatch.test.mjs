import test from "node:test";
import assert from "node:assert/strict";
import { MAX_DISPATCH_KEYS, isDispatchFence, parseDispatchKeys, planLine } from "./chat-dispatch.ts";

test("a dispatch fence names ticket keys, one per line", () => {
  assert.equal(isDispatchFence("dispatch"), true);
  assert.equal(isDispatchFence(" Dispatch extra"), true);
  assert.equal(isDispatchFence("dispatcher"), false);
  assert.deepEqual(parseDispatchKeys("pay-31\n"), ["PAY-31"]);
  assert.deepEqual(parseDispatchKeys("  WEB-3  \nignored"), ["WEB-3"]);
  assert.deepEqual(parseDispatchKeys("PAY-"), [], "incomplete while streaming");
  assert.deepEqual(parseDispatchKeys("not a key"), []);
});

test("one card can offer several tickets, each once", () => {
  assert.deepEqual(parseDispatchKeys("LAZYAGENTS-114\nlazyagents-115\n\nLAZYAGENTS-114\nLAZYAGENTS-1"), ["LAZYAGENTS-114", "LAZYAGENTS-115", "LAZYAGENTS-1"]);
  const many = Array.from({ length: MAX_DISPATCH_KEYS + 5 }, (_, i) => `PAY-${i + 1}`).join("\n");
  assert.equal(parseDispatchKeys(many).length, MAX_DISPATCH_KEYS);
});

test("the plan line says where the agent is", () => {
  assert.equal(planLine({ steps_total: 0, steps_done: 0 }), "");
  assert.equal(planLine({ steps_total: 4, steps_done: 1, current_step: "Fix Round()" }), "Step 2 of 4 · Fix Round()");
  assert.equal(planLine({ steps_total: 4, steps_done: 4 }), "4 of 4 steps done");
});
