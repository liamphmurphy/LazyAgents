import test from "node:test";
import assert from "node:assert/strict";
import { dockKey, dockReducer, dockShortcut, loadDock, saveDock } from "./editor-layout.ts";

const open = { open: true, tab: "chat", hidden: false };
const memory = () => {
  const m = new Map();
  return { getItem: (k) => m.get(k) ?? null, setItem: (k, v) => m.set(k, String(v)), m };
};
const key = (key, mods = {}) => ({ key, metaKey: true, ctrlKey: false, altKey: false, shiftKey: false, ...mods });

test("collapse and reopen: ⇥/⌘\\ toggles, clicking the active tab collapses, a rail icon reopens to its tab", () => {
  const collapsed = dockReducer(open, { type: "toggle" });
  assert.deepEqual(collapsed, { ...open, open: false });
  assert.deepEqual(dockReducer(collapsed, { type: "toggle" }), open);
  assert.deepEqual(dockReducer(open, { type: "select", tab: "chat" }), { ...open, open: false });
  // Another tab switches without collapsing.
  assert.deepEqual(dockReducer(open, { type: "select", tab: "comments" }), { ...open, tab: "comments" });
  // From the rail: opens to the clicked tab, even the one that was active.
  assert.deepEqual(dockReducer(collapsed, { type: "open", tab: "inspect" }), { ...open, tab: "inspect" });
  assert.deepEqual(dockReducer(collapsed, { type: "select", tab: "chat" }), open);
  assert.deepEqual(dockReducer(open, { type: "collapse" }), { ...open, open: false });
});

test("⌘. hides both panels and restores them as they were; opening a tab also restores", () => {
  const hidden = dockReducer(open, { type: "toggleHidden" });
  assert.equal(hidden.hidden, true);
  assert.deepEqual(dockReducer(hidden, { type: "toggleHidden" }), open);
  const collapsedHidden = dockReducer({ ...open, open: false }, { type: "toggleHidden" });
  assert.deepEqual(dockReducer(collapsedHidden, { type: "toggleHidden" }), { ...open, open: false });
  assert.deepEqual(dockReducer(hidden, { type: "open", tab: "chat" }), open);
  assert.deepEqual(dockReducer(collapsedHidden, { type: "toggle" }), open);
});

test("shortcuts: ⌘\\, ⌘J and ⌘. (or Ctrl); only ⌘J while typing", () => {
  assert.equal(dockShortcut(key("\\"), false), "toggle");
  assert.equal(dockShortcut(key("j"), false), "chat");
  assert.equal(dockShortcut(key("J"), false), "chat");
  assert.equal(dockShortcut(key("."), false), "hideAll");
  assert.equal(dockShortcut(key(".", { metaKey: false, ctrlKey: true }), false), "hideAll");
  assert.equal(dockShortcut(key("j"), true), "chat");
  assert.equal(dockShortcut(key("\\"), true), null);
  assert.equal(dockShortcut(key("."), true), null);
  assert.equal(dockShortcut(key("j", { metaKey: false }), false), null);
  assert.equal(dockShortcut(key("j", { shiftKey: true }), false), null);
  assert.equal(dockShortcut(key("k"), false), null);
});

test("persistence: open state and tab per app, survive a reload, never the ⌘. hide", () => {
  const s = memory();
  const tabs = ["chat", "comments"];
  assert.deepEqual(loadDock(s, "designs", tabs), open);
  saveDock(s, "designs", { open: false, tab: "comments", hidden: true });
  assert.equal(s.m.get(dockKey("designs")), JSON.stringify({ open: false, tab: "comments" }));
  assert.deepEqual(loadDock(s, "designs", tabs), { open: false, tab: "comments", hidden: false });
  // Per app.
  assert.deepEqual(loadDock(s, "docs", tabs), open);
  // A tab the app no longer has, junk, or no storage fall back.
  s.setItem(dockKey("docs"), JSON.stringify({ open: false, tab: "gone" }));
  assert.deepEqual(loadDock(s, "docs", tabs), { open: false, tab: "chat", hidden: false });
  s.setItem(dockKey("docs"), "{");
  assert.deepEqual(loadDock(s, "docs", tabs), open);
  assert.deepEqual(loadDock(undefined, "docs", tabs), open);
  // An app can start collapsed; a remembered choice still wins, and junk falls back to its default.
  const closed = { open: false, tab: "chat", hidden: false };
  assert.deepEqual(loadDock(memory(), "designs", tabs, false), closed);
  assert.deepEqual(loadDock(undefined, "designs", tabs, false), closed);
  s.setItem(dockKey("designs"), JSON.stringify({ open: true, tab: "comments" }));
  assert.deepEqual(loadDock(s, "designs", tabs, false), { open: true, tab: "comments", hidden: false });
  s.setItem(dockKey("designs"), "{");
  assert.deepEqual(loadDock(s, "designs", tabs, false), closed);
  const full = { getItem: () => null, setItem: () => { throw new Error("quota"); } };
  assert.doesNotThrow(() => saveDock(full, "docs", open));
});

