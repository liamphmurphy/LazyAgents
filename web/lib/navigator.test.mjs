import test from "node:test";
import assert from "node:assert/strict";
import { inChats, navigatorArea } from "./navigator.ts";

test("chat, spaces and the landing page show the Chats navigator", () => {
  assert.equal(navigatorArea("/"), "chats");
  assert.equal(navigatorArea("/chat"), "chats");
  assert.equal(navigatorArea("/chat/abc"), "chats");
  assert.equal(navigatorArea("/spaces/xyz"), "chats");
  assert.equal(inChats("/chat/abc"), true);
});

test("areas with their own list, or none, get no shell navigator, so two lists never stack", () => {
  for (const path of ["/runs", "/runs/123", "/knowledge", "/knowledge/p1", "/apps/designs/d1", "/tickets", "/settings", "/reports"]) {
    assert.equal(navigatorArea(path), null, path);
  }
  assert.equal(navigatorArea("/chatter"), null);
  assert.equal(inChats("/runs"), false);
});
