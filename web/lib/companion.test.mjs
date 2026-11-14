import test from "node:test";
import assert from "node:assert/strict";
import { claimCompanion, companionClaimed, companionOpen, companionSlot, setCompanionOpen, setCompanionSlot, subscribeCompanion } from "./companion.ts";

test("the right column is claimed while any page holds it, and tells its listeners", () => {
  let calls = 0;
  const off = subscribeCompanion(() => calls++);
  const a = claimCompanion();
  const b = claimCompanion();
  a();
  a(); // releasing twice is harmless
  assert.equal(companionClaimed(), true);
  b();
  assert.equal(companionClaimed(), false);
  const el = {};
  setCompanionSlot(el);
  setCompanionSlot(el);
  assert.equal(companionSlot(), el);
  setCompanionSlot(null);
  assert.equal(calls, 6);
  off();
});

test("every dock shares one open state once it has been chosen", () => {
  const m = new Map();
  const kv = { getItem: (k) => m.get(k) ?? null, setItem: (k, v) => m.set(k, v) };
  assert.equal(companionOpen(kv), undefined);
  setCompanionOpen(kv, false);
  assert.equal(companionOpen(kv), false);
  setCompanionOpen(kv, true);
  assert.equal(companionOpen(kv), true);
  assert.equal(companionOpen(undefined), undefined);
});
