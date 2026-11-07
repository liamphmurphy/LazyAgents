import test from "node:test";
import assert from "node:assert/strict";
import { firstLine, planCount, planLive, planStatus, readSources } from "./plans.ts";

const plan = (over) => ({ state: "planning", run_state: "running", goal: "Goal", proposal_count: 0, ticket_keys: [], ...over });

test("status pill follows the draft, then the run", () => {
  assert.deepEqual(planStatus(plan({})), { label: "Planning", tone: "review" });
  assert.equal(planStatus(plan({ run_state: "queued" })).label, "Queued");
  assert.equal(planStatus(plan({ run_state: "failed" })).label, "Failed");
  assert.equal(planStatus(plan({ run_state: "cancelled" })).label, "Cancelled");
  assert.equal(planStatus(plan({ state: "review", run_state: "completed" })).label, "Review");
  assert.equal(planStatus(plan({ state: "accepted", run_state: "completed" })).label, "Accepted");
  assert.equal(planStatus(plan({ state: "dismissed", run_state: "completed" })).label, "Dismissed");
});

test("index count: reading while live, accepted/total after, no proposals when it stopped empty", () => {
  assert.equal(planLive(plan({})), true);
  assert.equal(planCount(plan({})), "reading…");
  assert.equal(planCount(plan({ run_state: "queued" })), "reading…");
  assert.equal(planCount(plan({ run_state: "failed" })), "no proposals");
  assert.equal(planCount(plan({ state: "review", run_state: "completed", proposal_count: 6 })), "0/6 accepted");
  assert.equal(planCount(plan({ state: "accepted", run_state: "completed", proposal_count: 6, ticket_keys: ["A-1", "A-2", "A-3", "A-4", "A-5"] })), "5/6 accepted");
  assert.equal(planCount(plan({ state: "dismissed", run_state: "completed", proposal_count: 4 })), "0/4 accepted");
});

test("first line of the goal skips blank lines", () => {
  assert.equal(firstLine("\n  Ship estimates  \nmore detail"), "Ship estimates");
  assert.equal(firstLine(""), "");
});

const at = "2026-11-07T10:00:00Z";
const call = (seq, tool, input, id = String(seq)) => ({ seq, type: "tool_call", at, payload: { id, tool, input } });
const result = (seq, id) => ({ seq, type: "tool_result", at, payload: { id } });

test("read sources: MCP reads and context files, deduped, without searches or commands", () => {
  const events = [
    call(1, "mcp__lazyagents__read_page", { ref: "Architecture/ADR: Project chat sessions" }),
    result(2, "1"),
    call(3, "mcp__lazyagents__search_knowledge", { q: "planning" }),
    call(4, "Read", { file_path: "/w/.lazyagents/context/planning-ref-1.md" }),
    call(5, "Read", { file_path: "/w/web/app/page.tsx" }),
    call(6, "get_ticket", { ticket: "LAZYAGENTS-13" }),
    call(7, "mcp__lazyagents__read_page", { ref: "[[Architecture/ADR: Project chat sessions]]" }),
    call(8, "Bash", { command: "ls" }),
  ];
  const got = readSources(events);
  assert.deepEqual(got.map((s) => [s.activity.verb, s.activity.ref ?? s.activity.text, s.done]), [
    ["Reading", "Architecture/ADR: Project chat sessions", true],
    ["Reading", "planning-ref-1.md", false],
    ["Opening", "LAZYAGENTS-13", false],
  ]);
  assert.deepEqual(readSources(undefined), []);
});
