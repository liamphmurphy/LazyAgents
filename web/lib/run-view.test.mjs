import test from "node:test";
import assert from "node:assert/strict";
import {
  argSummary, cacheShare, delegateChild, groupByTurn, hasActiveFilters, parseRunFilters, rowsOf, runFilterQuery,
  toolLink, toolName, turnAt, turnByEvent, turnStarts, usageByModel, workHref,
} from "./run-view.ts";

const ev = (seq, type, payload, at = `2026-11-03T12:00:${String(seq).padStart(2, "0")}Z`) => ({ seq, type, payload, at });
const marker = (seq, turn) => ev(seq, "lifecycle", { state: "turn_started", turn });

test("toolName and argSummary", () => {
  assert.equal(toolName("mcp__lazyagents__read_page"), "lazyagents.read_page");
  assert.equal(toolName("Bash"), "Bash");
  assert.equal(argSummary({ command: "ls" }), "ls");
  assert.equal(argSummary({ ref: "a" }), '(ref: "a")');
});

test("rowsOf pairs results by id", () => {
  const rows = rowsOf([ev(1, "tool_call", { id: "x", tool: "Bash" }), ev(2, "message", { text: "hi" }), ev(3, "tool_result", { id: "x", output: "ok" })]);
  assert.equal(rows.length, 2);
  assert.equal(rows[0].kind, "tool");
  assert.equal(rows[0].result.seq, 3);
});

test("turnByEvent assigns events to turns, with setup before the first marker", () => {
  const events = [ev(1, "message", {}), marker(2, 1), ev(3, "message", {}), marker(4, 2), ev(5, "message", {})];
  const m = turnByEvent(events);
  assert.deepEqual([1, 2, 3, 4, 5].map((s) => m.get(s)), [0, 1, 1, 2, 2]);
  const groups = groupByTurn(events, (e) => m.get(e.seq));
  assert.deepEqual(groups.map((g) => [g.turn, g.items.length]), [[0, 1], [1, 2], [2, 2]]);
});

test("turnAt places child events by time", () => {
  const starts = turnStarts([marker(2, 1), marker(10, 2)]);
  assert.equal(turnAt(starts, "2026-11-03T12:00:01Z"), 0);
  assert.equal(turnAt(starts, "2026-11-03T12:00:05Z"), 1);
  assert.equal(turnAt(starts, "2026-11-03T12:00:30Z"), 2);
});

test("toolLink maps wiki and artifact reads", () => {
  assert.deepEqual(toolLink("mcp__lazyagents__read_page", { ref: "Patterns/Go errors" }), { href: "/wiki/Patterns/Go%20errors", label: "Patterns/Go errors" });
  assert.equal(toolLink("mcp__lazyagents__read_artifact", { artifact: "abc" })?.href, "/artifacts/abc");
  assert.equal(toolLink("Bash", { ref: "x" }), null);
  assert.equal(toolLink("mcp__lazyagents__read_page", {}), null);
});

test("delegateChild picks the closest child of that agent after the call", () => {
  const children = [
    { id: "early", started_at: "2026-11-03T11:00:00Z", agent: { handle: "rev" } },
    { id: "b", started_at: "2026-11-03T12:00:09Z", agent: { handle: "rev" } },
    { id: "a", started_at: "2026-11-03T12:00:05Z", agent: { handle: "rev" } },
    { id: "other", started_at: "2026-11-03T12:00:04Z", agent: { handle: "qa" } },
  ];
  const call = ev(3, "tool_call", { tool: "mcp__lazyagents__delegate", input: { agent: "rev" } });
  assert.equal(delegateChild(call, children)?.id, "a");
  assert.equal(delegateChild(ev(3, "tool_call", { tool: "Bash" }), children), undefined);
});

test("usageByModel groups this run and its children", () => {
  const r = (model, cost, source = "harness") => ({ model, cost_usd: cost, cost_source: source, input_tokens: 0, output_tokens: 0 });
  const out = usageByModel([r("opus", 1), r("sonnet", 0.5), r("sonnet", 0.25, "unknown")]);
  assert.deepEqual(out.map((m) => [m.model, m.runs, m.cost, m.unpriced]), [["opus", 1, 1, false], ["sonnet", 2, 0.75, true]]);
});

test("cacheShare", () => {
  assert.equal(cacheShare(100, 90), 0.9);
  assert.equal(cacheShare(0, 0), null);
  assert.equal(cacheShare(100, undefined), null);
});

test("filters round-trip through the URL", () => {
  assert.deepEqual(parseRunFilters(new URLSearchParams("")), { state: "live", agent: undefined, project: undefined, purpose: undefined });
  const f = parseRunFilters(new URLSearchParams("state=failed&agent=dev&project=WEB&purpose=chat"));
  assert.deepEqual(f, { state: "failed", agent: "dev", project: "WEB", purpose: "chat" });
  assert.equal(runFilterQuery(f), "state=failed&agent=dev&project=WEB&purpose=chat");
  assert.equal(runFilterQuery({ state: "live" }), "");
  assert.equal(parseRunFilters(new URLSearchParams("state=bogus&purpose=x")).state, "live");
  assert.equal(hasActiveFilters({ state: "live" }), false);
  assert.equal(hasActiveFilters({ state: "all" }), true);
});

test("workHref never links a ticket without a key", () => {
  assert.equal(workHref({ purpose: "ticket", ticket_key: "WEB-1" }), "/tickets/WEB-1");
  assert.equal(workHref({ purpose: "ticket", ticket_key: "" }), null);
  assert.equal(workHref({ purpose: "planning", ticket_key: "", planning_id: "p1" }), "/plans?id=p1");
  assert.equal(workHref({ purpose: "chat", ticket_key: "", chat_id: "c1", project_key: "WEB" }), "/chat/c1?project=WEB");
});

test("parallel delegations consume distinct children in call order", () => {
  const calls = [ev(1, "tool_call", { tool: "mcp__lazyagents__delegate", input: { agent: "rev" } }), ev(2, "tool_call", { tool: "mcp__lazyagents__delegate", input: { agent: "rev" } })];
  const children = [3, 4].map((n) => ({ id: String(n), started_at: ev(n, "", {}).at, agent: { handle: "rev" } }));
  assert.equal(delegateChild(calls[0], children, calls)?.id, "3");
  assert.equal(delegateChild(calls[1], children, calls)?.id, "4");
  for (const state of ["starting", "blocked"]) assert.equal(parseRunFilters(new URLSearchParams({ state })).state, state);
});
