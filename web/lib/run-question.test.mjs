import test from "node:test";
import assert from "node:assert/strict";
import { latestQuestion, withMention } from "./run-question.ts";

const call = (reason, tool = "mcp__lazyagents__await_human") => ({ type: "tool_call", payload: { tool, input: { reason } } });

test("latestQuestion picks the newest await_human reason", () => {
  assert.equal(latestQuestion([]), null);
  assert.equal(latestQuestion([call("first?"), { type: "message", payload: { text: "x" } }, call(" second? ")]), "second?");
  assert.equal(latestQuestion([call("hi", "Bash")]), null);
});

test("latestQuestion falls back to a waiting lifecycle reason but ignores wait kinds", () => {
  assert.equal(latestQuestion([{ type: "lifecycle", payload: { state: "waiting", reason: "Which db?" } }]), "Which db?");
  assert.equal(latestQuestion([{ type: "lifecycle", payload: { state: "waiting", reason: "approval" } }]), null);
});

test("withMention adds the handle once", () => {
  assert.equal(withMention("  yes  ", "dev"), "@dev yes");
  assert.equal(withMention("@dev yes", "dev"), "@dev yes");
  assert.equal(withMention("thanks @Dev", "dev"), "thanks @Dev");
  assert.equal(withMention("@developer yes", "dev"), "@dev @developer yes");
  assert.equal(withMention("a@dev.com", "dev"), "@dev a@dev.com");
});
