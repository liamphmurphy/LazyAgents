import test from "node:test";
import assert from "node:assert/strict";
import { DOCK_MAX_PERSONAS, dockEntries, dockSummary, viewedChatId } from "./persona-dock.ts";

const chat = (id, state, unread = 0, waiting_approval = false) => ({ id, title: id, state, unread, waiting_approval, last_seq: 1, last_line: "" });
const entry = (state, ...chats) => ({ state, unread: chats.reduce((n, c) => n + c.unread, 0), chats });

test("reads the chat id from the path", () => {
  assert.equal(viewedChatId("/chat/abc"), "abc");
  assert.equal(viewedChatId("/chat"), null);
  assert.equal(viewedChatId("/runs/abc"), null);
});

test("the chat on screen never counts as unread", () => {
  assert.deepEqual(dockEntries([entry("replied", chat("a", "replied", 2))], "a"), []);
  const [left] = dockEntries([entry("needs_you", chat("a", "needs_you", 1), chat("b", "replied", 3))], "a");
  assert.equal(left.state, "replied");
  assert.equal(left.unread, 3);
});

test("viewing keeps live work and pending approvals", () => {
  assert.equal(dockEntries([entry("working", chat("a", "working", 1))], "a")[0].unread, 0);
  assert.equal(dockEntries([entry("needs_you", chat("a", "needs_you", 1, true))], "a")[0].state, "needs_you");
});

test("keeps the server's order so the cap takes the most recent", () => {
  const all = ["p1", "p2", "p3", "p4"].map((id) => entry("replied", chat(id, "replied", 1)));
  assert.deepEqual(dockEntries(all, null).slice(0, DOCK_MAX_PERSONAS).map((e) => e.chats[0].id), ["p1", "p2", "p3"]);
});

test("summarizes for the narrow badge", () => {
  assert.equal(dockSummary([entry("needs_you", chat("a", "needs_you", 1)), entry("working", chat("b", "working"))]), "1 needs you · 1 unread · 1 working");
});
