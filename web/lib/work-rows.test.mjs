import test from "node:test";
import assert from "node:assert/strict";
import {
  approvalId,
  epicProgress,
  filterText,
  needsYouByTicket,
  parseFilterText,
  resolveFilter,
  startReady,
  stateText,
  weekStart,
  withoutProjectToken,
  workGroups,
} from "./work-rows.ts";

const t = (key, over = {}) => ({
  id: key, key, project_key: "LA", title: key, status: "todo", priority: "none", labels: [],
  criteria_total: 0, criteria_done: 0, unmet_dependencies: 0, updated_at: "2026-11-12T12:00:00Z", ...over,
});
const item = (id, kind, ticket_key, over = {}) => ({ id, kind, ticket_key, resolved: false, summary: "", href: "/x", created_at: "2026-11-12T10:00:00Z", ...over });

test("needsYouByTicket keeps open approval, question and blocked items, most urgent per ticket", () => {
  const m = needsYouByTicket([
    item("question:r1", "question", "LA-1", { summary: "Which DB?" }),
    item("approval:a1", "approval", "LA-1", { summary: "Run migration" }),
    item("approval:a2", "approval", "LA-2", { resolved: true }),
    item("review:t3", "review", "LA-3"),
    item("blocked:r4", "blocked", "LA-4"),
    item("planning:p", "planning", ""),
  ]);
  assert.deepEqual([...m.keys()].sort(), ["LA-1", "LA-4"]);
  assert.equal(m.get("LA-1").kind, "approval");
  assert.equal(approvalId(m.get("LA-1")), "a1");
  assert.equal(approvalId(m.get("LA-4")), undefined);
});

test("stateText: needs you is amber, then blocked, then status", () => {
  assert.deepEqual(stateText(t("A"), { kind: "approval", summary: "Push" }), { text: "Wants approval: Push", tone: "human" });
  assert.deepEqual(stateText(t("A", { unmet_dependencies: 2 })), { text: "Blocked by 2", tone: "blocked" });
  assert.equal(stateText(t("A", { status: "done", unmet_dependencies: 2, pr: { number: 9, state: "merged" } })).text, "Done · PR #9 merged");
  assert.equal(stateText(t("A", { status: "in_review" })).text, "In review");
});

test("workGroups puts Needs you first and buckets the rest by status", () => {
  const now = new Date("2026-11-12T15:00:00"); // a Thursday
  const tickets = [
    t("A", { status: "in_progress" }),
    t("B", { status: "in_review" }),
    t("C"),
    t("D", { status: "done", updated_at: new Date("2026-11-10T09:00:00").toISOString() }),
    t("E", { status: "done", updated_at: new Date("2026-11-01T09:00:00").toISOString() }),
    t("F", { status: "backlog" }),
    t("G", { status: "done" }),
  ];
  const needs = new Map([["A", {}], ["G", {}]]);
  const groups = workGroups(tickets, needs, "status", now);
  assert.deepEqual(groups.map((g) => [g.title, g.tickets.map((x) => x.key)]), [
    ["Needs you", ["A"]],
    ["In progress", ["B"]],
    ["Ready", ["C"]],
    ["Backlog", ["F"]],
    ["Done this week", ["D", "G"]],
    ["Done earlier", ["E"]],
  ]);
  assert.ok(groups[0].needs);
  const byEpic = workGroups([t("A", { epic: { id: "e", name: "E" } }), t("B")], new Map([["B", {}]]), "epic", now);
  assert.deepEqual(byEpic.map((g) => g.title), ["Needs you", "E"]);
  assert.equal(weekStart(now), new Date("2026-11-09T00:00:00").getTime());
});

test("parseFilterText and filterText round-trip tokens and free text", () => {
  assert.deepEqual(parseFilterText('fix login epic:"Mobile UI" agent:@atlas status:ready'), {
    tokens: [{ key: "epic", value: "Mobile UI" }, { key: "agent", value: "@atlas" }, { key: "status", value: "ready" }],
    text: "fix login",
  });
  assert.deepEqual(parseFilterText("project: hello"), { tokens: [], text: "hello" });
  const s = filterText({ project: "LA", epic: "Mobile UI", q: "login" });
  assert.equal(s, 'project:LA epic:"Mobile UI" login');
  assert.deepEqual(parseFilterText(s).tokens.map((x) => x.value), ["LA", "Mobile UI"]);
});

test("resolveFilter maps tokens to URL filters and reports unknown ones", () => {
  const look = {
    projects: [{ id: "p1", key: "LA", name: "Lazy Agents" }, { id: "p2", key: "PAY", name: "Payments" }],
    epics: [{ id: "e1", name: "Mobile UI", project_id: "p2" }, { id: "e2", name: "Mobile UI", project_id: "p1" }],
    agents: [{ handle: "atlas", name: "Atlas" }],
  };
  assert.deepEqual(resolveFilter("project:la epic:mobile-ui agent:@Atlas status:review login", look), {
    unknown: [], q: "login", project: "LA", epic: "e2", assignee: "atlas", status: "in_review",
  });
  assert.deepEqual(resolveFilter("epic:nope status:zzz", look), { unknown: ["epic:nope", "status:zzz"] });
  // Same name in two projects with All projects: the URL's epic wins.
  assert.equal(resolveFilter('epic:"Mobile UI"', { ...look, currentEpic: "e2" }).epic, "e2");
  assert.equal(resolveFilter('epic:"Mobile UI"', look).epic, "e1");
  assert.equal(withoutProjectToken('project:OPS epic:"A b" project:"Lazy Agents" bug'), 'epic:"A b" bug');
});

test("epicProgress and startReady", () => {
  const tickets = [t("A", { status: "done" }), t("B", { status: "in_progress" }), t("C", { status: "in_progress" }), t("D"), t("E", { status: "canceled" }), t("F", { unmet_dependencies: 1 })];
  assert.deepEqual(epicProgress(tickets, new Map([["C", {}]])), { done: 1, running: 1, needs: 1, open: 2, total: 5 });
  const r = startReady(tickets, new Set(["B"]));
  assert.deepEqual(r.ready.map((x) => x.key), ["C", "D"]);
  assert.deepEqual(r.blocked.map((x) => x.key), ["F"]);
});
