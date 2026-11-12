import test from "node:test";
import assert from "node:assert/strict";
import { TABS, badgeLabel, currentTab, hidesTabBar } from "./tab-bar.ts";

const counts = { inbox: 0, runs: 0, scheduled: 0, wiki: 0, settings: 0 };

test("the bar has Chat, Inbox, Runs, Tickets and More", () => {
  assert.deepEqual(TABS.map((t) => t.label), ["Chat", "Inbox", "Runs", "Tickets", "More"]);
});

test("a path selects its section's tab, and anything else selects More", () => {
  assert.equal(currentTab("/"), "chat");
  assert.equal(currentTab("/chat/abc"), "chat");
  assert.equal(currentTab("/inbox"), "inbox");
  assert.equal(currentTab("/runs"), "runs");
  assert.equal(currentTab("/tickets"), "tickets");
  assert.equal(currentTab("/more"), "more");
  assert.equal(currentTab("/apps/docs"), "more");
  assert.equal(currentTab("/settings"), "more");
  assert.equal(currentTab("/runsx"), "more");
});

test("run and ticket detail screens hide the bar; their lists keep it", () => {
  assert.equal(hidesTabBar("/runs/123"), true);
  assert.equal(hidesTabBar("/tickets/WEB-3"), true);
  assert.equal(hidesTabBar("/tickets/new"), true);
  assert.equal(hidesTabBar("/runs"), false);
  assert.equal(hidesTabBar("/tickets"), false);
  assert.equal(hidesTabBar("/chat/abc"), false);
});

test("counts reach the link name for Inbox and Runs only", () => {
  assert.equal(badgeLabel("Inbox", "inbox", { ...counts, inbox: 3 }), "Inbox 3 unread");
  assert.equal(badgeLabel("Runs", "runs", { ...counts, runs: 2 }), "Runs 2 running");
  assert.equal(badgeLabel("Inbox", "inbox", counts), "Inbox");
  assert.equal(badgeLabel("Wiki", "wiki", { ...counts, wiki: 1 }), "Wiki");
  assert.equal(badgeLabel("Chat", undefined, counts), "Chat");
});
