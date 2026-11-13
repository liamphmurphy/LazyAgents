import assert from "node:assert/strict";
import test from "node:test";
import { invalidatedBy } from "./live-updates.ts";

test("a change hint invalidates its entity's queries and the overview counts", () => {
  assert.deepEqual(invalidatedBy("chat.updated"), ["chats", "canvases", "videos", "dock", "dashboard", "epics", "agent-overviews"]);
  assert.deepEqual(invalidatedBy("ticket.updated").slice(0, 2), ["planning", "tickets"]);
});

test("harness log lines and video progress refetch only their own queries", () => {
  assert.deepEqual(invalidatedBy("run.events"), ["runs"]);
  assert.deepEqual(invalidatedBy("video.updated"), ["videos"]);
});

test("an unknown hint refetches only the overview counts", () => {
  assert.deepEqual(invalidatedBy("nope.updated"), ["dashboard", "epics", "agent-overviews"]);
});
