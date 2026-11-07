import test from "node:test";
import assert from "node:assert/strict";
import { activityOf, bareTool, defaultChatAgent, linkLabel, needsSwitchConfirm, replyCost, toolCalls } from "./chat-ui.ts";

const lead = { id: "a1", kind: "main", name: "Lead" };
const helper = { id: "a2", kind: "sub", name: "Helper" };
const mind = { id: "a3", kind: "main", name: "Mastermind" };

test("a new chat starts with the remembered agent, then the newest chat's, then the first main agent", () => {
  const agents = [helper, lead, mind];
  assert.equal(defaultChatAgent(agents, "a3", [{ agent: { id: "a1" } }]), mind);
  assert.equal(defaultChatAgent(agents, undefined, [{ agent: { id: "a3" } }]), mind);
  assert.equal(defaultChatAgent(agents, "gone", undefined), lead);
  // Sub agents never chat, even when remembered or used before.
  assert.equal(defaultChatAgent(agents, "a2", [{ agent: { id: "a2" } }]), lead);
  assert.equal(defaultChatAgent([helper], null, []), undefined);
});

test("tool calls pair with results and skip events before the turn", () => {
  const events = [
    { seq: 1, type: "tool_call", at: "2026-11-06T10:00:00Z", payload: { id: "old", tool: "whoami" } },
    { seq: 2, type: "tool_call", at: "2026-11-06T10:01:00Z", payload: { id: "x", tool: "read_page", input: { ref: "Plan" } } },
    { seq: 3, type: "tool_result", at: "2026-11-06T10:01:01Z", payload: { id: "x" } },
    { seq: 4, type: "tool_call", at: "2026-11-06T10:01:02Z", payload: { id: "y", tool: "get_ticket", input: { ticket: "WEB-3" } } },
  ];
  assert.deepEqual(toolCalls(events, "2026-11-06T10:00:30Z"), [
    { key: 2, tool: "read_page", input: { ref: "Plan" }, done: true },
    { key: 4, tool: "get_ticket", input: { ticket: "WEB-3" }, done: false },
  ]);
  assert.equal(toolCalls(undefined).length, 0);
});

test("MCP tool names lose the lazyagents prefix from either harness", () => {
  assert.equal(bareTool("mcp__lazyagents__read_page"), "read_page");
  assert.equal(bareTool("lazyagents.read_page"), "read_page");
  assert.equal(bareTool("read_page"), "read_page");
  assert.equal(bareTool("mcp__github__get_issue"), "github.get_issue");
});

test("the live line names the page, ticket or query being used", () => {
  assert.deepEqual(activityOf({ tool: "mcp__lazyagents__read_page", input: { ref: "Architecture/ADR - Project chat sessions" } }), {
    verb: "Reading", ref: "Architecture/ADR - Project chat sessions", label: "ADR - Project chat sessions",
  });
  assert.deepEqual(activityOf({ tool: "read_page", input: { ref: "[[ADR-012]]" } }), { verb: "Reading", ref: "ADR-012", label: "ADR-012" });
  assert.deepEqual(activityOf({ tool: "read_artifact", input: { artifact: "ADR-012", section: "risks" } }), {
    verb: "Reading", ref: "ADR-012#risks", label: "ADR-012 › risks",
  });
  assert.deepEqual(activityOf({ tool: "read_artifact", input: { artifact: "01a1b4ab-1853-7647-8534-85c284cec5ec" } }), { verb: "Reading an artifact" });
  assert.deepEqual(activityOf({ tool: "get_ticket", input: { ticket: "LAZYAGENTS-12" } }), { verb: "Opening", ref: "LAZYAGENTS-12", label: "LAZYAGENTS-12" });
  assert.deepEqual(activityOf({ tool: "search_knowledge", input: { q: "harness switch" } }), { verb: "Searching", text: "“harness switch”" });
  assert.deepEqual(activityOf({ tool: "read_page", input: {} }), { verb: "Reading a page" });
  assert.deepEqual(activityOf({ tool: "shell", input: { command: "ls" } }), { verb: "Running a command" });
  assert.deepEqual(activityOf({ tool: "mcp__github__get_issue", input: null }), { verb: "Using", text: "github.get_issue" });
});

test("link labels drop folders and keep the section", () => {
  assert.equal(linkLabel("Eng/Ledger/Runbook"), "Runbook");
  assert.equal(linkLabel("Runbook#rollback"), "Runbook › rollback");
  assert.equal(linkLabel("#rollback"), "rollback");
});

test("only a harness change on a chat with messages asks first", () => {
  const claude = { harness: "claude-code" };
  const codex = { harness: "codex" };
  assert.equal(needsSwitchConfirm(claude, codex, null, 3), true);
  assert.equal(needsSwitchConfirm(claude, { harness: "claude-code" }, null, 3), false, "same harness, new model");
  assert.equal(needsSwitchConfirm(claude, codex, null, 0), false, "nothing to lose yet");
  assert.equal(needsSwitchConfirm(claude, codex, codex, 3), false, "already confirmed for codex");
  assert.equal(needsSwitchConfirm(claude, { harness: "opencode" }, codex, 3), true);
});

test("reply cost keeps small turns visible and never shows unknown as free", () => {
  assert.equal(replyCost(0.0421), "$0.042");
  assert.equal(replyCost(1.5), "$1.50");
  assert.equal(replyCost(0.0002), "< $0.001");
  assert.equal(replyCost(0), undefined);
  assert.equal(replyCost(undefined), undefined);
  assert.equal(replyCost(Number.NaN), undefined);
});
