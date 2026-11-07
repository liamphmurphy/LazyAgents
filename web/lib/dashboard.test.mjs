import test from "node:test";
import assert from "node:assert/strict";
import { blockedTickets, currentStep, epicsAtRisk, liveRuns, setupSteps, throughput, topInbox } from "./dashboard.ts";

const run = (id, purpose, state, plan = []) => ({ id, purpose, state, plan });

test("live runs drop chat runs waiting between turns but keep responding chats and waiting ticket runs", () => {
  const runs = [
    run("t1", "ticket", "running"),
    run("t2", "ticket", "waiting"),
    run("p1", "planning", "waiting"),
    run("c1", "chat", "waiting"),
    run("c2", "chat", "running"),
  ];
  assert.deepEqual(liveRuns(runs).map((r) => r.id), ["t1", "t2", "p1", "c2"]);
});

test("current step reports its 1-based position out of the plan length", () => {
  const plan = [
    { position: 1, text: "Read", state: "done" },
    { position: 2, text: "Implement API", state: "current" },
    { position: 3, text: "Verify", state: "pending" },
    { position: 4, text: "Ship", state: "pending" },
  ];
  assert.deepEqual(currentStep(plan), { text: "Implement API", position: 2, total: 4 });
  assert.equal(currentStep([]), undefined);
  assert.deepEqual(currentStep([{ position: 1, text: "Ask", state: "needs_human" }]), { text: "Ask", position: 1, total: 1 });
});

test("top inbox follows the Inbox's urgency order and counts every open item", () => {
  const at = (min) => new Date(Date.UTC(2026, 10, 7, 12) - min * 60_000).toISOString();
  const item = (id, kind, min, extra = {}) => ({ id, kind, created_at: at(min), resolved: false, read: false, ...extra });
  const items = [
    item("m1", "mention", 500),
    item("r1", "review", 10),
    item("a1", "approval", 5),
    item("q1", "question", 60),
    item("a2", "approval", 90),
    item("s1", "stalled", 30),
    item("x", "approval", 999, { read: true }),
  ];
  const top = topInbox(items, 5);
  assert.equal(top.open, 6);
  assert.deepEqual(top.items.map((i) => i.id), ["a2", "a1", "q1", "s1", "r1"]);
});

test("blocked tickets are open tickets with unmet dependencies, most urgent first", () => {
  const card = (key, status, unmet, priority = "medium", updated_at = "2026-11-01T00:00:00Z") => ({ key, status, unmet_dependencies: unmet, priority, updated_at });
  const cards = [
    card("A-1", "todo", 1, "low"),
    card("A-2", "in_progress", 0, "urgent"),
    card("A-3", "done", 2, "urgent"),
    card("A-4", "canceled", 1, "urgent"),
    card("A-5", "backlog", 3, "high"),
    card("A-6", "todo", 1, "high", "2026-11-05T00:00:00Z"),
  ];
  assert.deepEqual(blockedTickets(cards).map((c) => c.key), ["A-6", "A-5", "A-1"]);
});

test("epics at risk are open epics at risk or off track, off track first", () => {
  const epic = (name, status, health) => ({ name, status, health, points_total: 10, points_done: 2, tickets_open: 3 });
  const epics = [epic("ok", "open", "on_track"), epic("risk", "open", "at_risk"), epic("off", "open", "off_track"), epic("closed", "done", "off_track")];
  assert.deepEqual(epicsAtRisk(epics).map((e) => e.name), ["off", "risk"]);
});

test("throughput is exactly 14 UTC days ending today, zero-filled", () => {
  const now = new Date(Date.UTC(2026, 10, 7, 23, 30));
  const days = throughput([{ date: "2026-11-07", count: 3 }, { date: "2026-10-25", count: 1 }, { date: "2026-10-24", count: 9 }], now);
  assert.equal(days.length, 14);
  assert.equal(days[0].date, "2026-10-25");
  assert.equal(days[13].date, "2026-11-07");
  assert.equal(days.reduce((n, d) => n + d.count, 0), 4);
  assert.equal(days[5].count, 0);
});

test("setup checklist tracks project, agent and an online runner", () => {
  assert.deepEqual(setupSteps({ projects: 1, agents: 0, runnerOnline: false }).map((s) => s.done), [true, false, false]);
  assert.ok(setupSteps({ projects: 2, agents: 1, runnerOnline: true }).every((s) => s.done));
});
