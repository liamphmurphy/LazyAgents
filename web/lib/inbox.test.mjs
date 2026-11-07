import test from "node:test";
import assert from "node:assert/strict";
import { groupItems, isOpen, nextSelection, step, waited } from "./inbox.ts";

const at = (min) => new Date(Date.UTC(2026, 10, 7, 12, 0) - min * 60_000).toISOString();
const item = (id, kind, min, extra = {}) => ({ id, kind, created_at: at(min), resolved: false, read: false, ...extra });

test("groups follow urgency order and the longest wait comes first", () => {
  const items = [
    item("m1", "mention", 5),
    item("s1", "stalled", 40),
    item("q1", "question", 10),
    item("a2", "approval", 30),
    item("a1", "approval", 60),
    item("f1", "blocked", 300),
    item("old", "approval", 90, { resolved: true }),
  ];
  const groups = groupItems(items, "open", "oldest");
  assert.deepEqual(groups.map((g) => g.label), ["Approvals", "Questions", "Failed & stalled runs", "Mentions"]);
  assert.deepEqual(groups[0].items.map((i) => i.id), ["a1", "a2"]);
  assert.deepEqual(groups[2].items.map((i) => i.id), ["f1", "s1"]);
  assert.deepEqual(groupItems(items, "open", "newest")[0].items.map((i) => i.id), ["a2", "a1"]);
  assert.deepEqual(groupItems(items, "done", "oldest").flatMap((g) => g.items.map((i) => i.id)), ["old"]);
});

test("done or resolved items are not open", () => {
  assert.equal(isOpen(item("x", "approval", 1)), true);
  assert.equal(isOpen(item("x", "approval", 1, { read: true })), false);
  assert.equal(isOpen(item("x", "approval", 1, { resolved: true })), false);
});

test("focus keeps its slot when the selected item leaves", () => {
  assert.equal(nextSelection(["a", "b", "c"], "b", 1), "b");
  // b resolved: the next item takes its slot.
  assert.equal(nextSelection(["a", "c"], "b", 1), "c");
  // The last item resolved: the previous one.
  assert.equal(nextSelection(["a", "b"], "c", 2), "b");
  assert.equal(nextSelection([], "c", 2), undefined);
  assert.equal(nextSelection(["a"], undefined, 0), "a");
});

test("j/k wrap around", () => {
  assert.equal(step(["a", "b"], "b", 1), "a");
  assert.equal(step(["a", "b"], "a", -1), "b");
  assert.equal(step(["a", "b"], undefined, 1), "a");
  assert.equal(step([], "a", 1), undefined);
});

test("wait time reads in minutes, hours and days", () => {
  const now = Date.UTC(2026, 10, 7, 12, 0);
  assert.equal(waited(at(0), now), "now");
  assert.equal(waited(at(42), now), "42m");
  assert.equal(waited(at(65), now), "1h 5m");
  assert.equal(waited(at(180), now), "3h");
  assert.equal(waited(at(60 * 50), now), "2d");
});
