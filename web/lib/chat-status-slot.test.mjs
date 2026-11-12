import assert from "node:assert/strict";
import test from "node:test";
import { orderStatus, splitStatus, STATUS_PRIORITY } from "./chat-status-slot.ts";

const keys = (items) => items.map((i) => i.key);

test("the priority order: blocking, needs you, failed turn, errors, live status", () => {
  assert.deepEqual([...STATUS_PRIORITY], ["blocking", "needs-you", "failed-turn", "error", "live"]);
});

test("orderStatus sorts by priority and keeps the given order within a kind", () => {
  const items = [
    { key: "followups", kind: "live" },
    { key: "send-error", kind: "error" },
    { key: "runs", kind: "live" },
    { key: "failed", kind: "failed-turn" },
    { key: "start", kind: "needs-you" },
    { key: "model", kind: "blocking" },
  ];
  assert.deepEqual(keys(orderStatus(items)), ["model", "start", "failed", "send-error", "followups", "runs"]);
});

test("collapsed shows only the top item; the rest are hidden", () => {
  const { shown, hidden } = splitStatus([{ key: "runs", kind: "live" }, { key: "failed", kind: "failed-turn" }, { key: "err", kind: "error" }], false);
  assert.deepEqual(keys(shown), ["failed"]);
  assert.deepEqual(keys(hidden), ["err", "runs"]);
});

test("a commit-email pick or an unavailable model always shows first, even together", () => {
  const { shown, hidden } = splitStatus([{ key: "failed", kind: "failed-turn" }, { key: "email", kind: "blocking" }, { key: "model", kind: "blocking" }], false);
  assert.deepEqual(keys(shown), ["email", "model"]);
  assert.deepEqual(keys(hidden), ["failed"]);
});

test("a pinned item never collapses, even when something more important arrives", () => {
  const { shown, hidden } = splitStatus([{ key: "followups", kind: "live", pinned: true }, { key: "failed", kind: "failed-turn" }, { key: "runs", kind: "live" }], false);
  assert.deepEqual(keys(shown), ["failed", "followups"]);
  assert.deepEqual(keys(hidden), ["runs"]);
});

test("expanded shows everything in order", () => {
  const { shown, hidden } = splitStatus([{ key: "runs", kind: "live" }, { key: "err", kind: "error" }], true);
  assert.deepEqual(keys(shown), ["err", "runs"]);
  assert.deepEqual(hidden, []);
});

test("no items, nothing shown", () => {
  assert.deepEqual(splitStatus([], false), { shown: [], hidden: [] });
});
