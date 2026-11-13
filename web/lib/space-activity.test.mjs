import test from "node:test";
import assert from "node:assert/strict";
import { activityLabel, badgeTarget, groupSpaceActivity, jumpTarget } from "./space-activity.ts";

const chat = (id, space_id, state, started_at) => ({ id, space_id, title: id, state, started_at });

test("groups chats by space and finds the latest working one", () => {
  const map = groupSpaceActivity([chat("a", "s1", "working", "t2"), chat("b", "s1", "working", "t1"), chat("c", "s2", "needs_you")]);
  assert.equal(map.get("s1").latestWorking.id, "a");
  assert.equal(map.get("s1").needsYou, false);
  assert.equal(map.get("s2").needsYou, true);
  assert.equal(map.get("s2").latestWorking, undefined);
  assert.equal(groupSpaceActivity(undefined).size, 0);
});

test("one chat goes straight to it; several open the flyout", () => {
  const one = groupSpaceActivity([chat("a", "s1", "working", "t1")]).get("s1");
  assert.deepEqual(badgeTarget(one), { kind: "chat", chat: one.chats[0] });
  const two = groupSpaceActivity([chat("a", "s1", "working", "t2"), chat("b", "s1", "needs_you")]).get("s1");
  assert.deepEqual(badgeTarget(two), { kind: "flyout" });
});

test("the jump button needs several chats and one that is working", () => {
  const one = groupSpaceActivity([chat("a", "s1", "working", "t1")]).get("s1");
  assert.equal(jumpTarget(one), undefined);
  const waiting = groupSpaceActivity([chat("a", "s1", "needs_you"), chat("b", "s1", "needs_you")]).get("s1");
  assert.equal(jumpTarget(waiting), undefined);
  const mixed = groupSpaceActivity([chat("a", "s1", "working", "t1"), chat("b", "s1", "needs_you")]).get("s1");
  assert.equal(jumpTarget(mixed).id, "a");
});

test("the label counts working and waiting chats", () => {
  const a = groupSpaceActivity([chat("a", "s1", "working", "t2"), chat("b", "s1", "working", "t1"), chat("c", "s1", "needs_you")]).get("s1");
  assert.equal(activityLabel("AIQ", a), "AIQ: 2 working, 1 waiting on you");
});
