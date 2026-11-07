import test from "node:test";
import assert from "node:assert/strict";
import { burnUp, epicFilterQuery, filterEpics, isOverdue, parseEpicFilters, sortEpics, statusCounts } from "./epics.ts";

const make = (name, health, extra = {}) => ({
  epic: { id: name, name, health, status: "open", points_total: 10, points_done: 0, ...extra.epic },
  target_date: extra.target_date,
});

test("filters default to open, any health, needs attention and ignore junk", () => {
  assert.deepEqual(parseEpicFilters(new URLSearchParams("")), { status: "open", health: "any", sort: "attention" });
  assert.deepEqual(parseEpicFilters(new URLSearchParams("status=done&health=at_risk&sort=name")), { status: "done", health: "at_risk", sort: "name" });
  assert.deepEqual(parseEpicFilters(new URLSearchParams("status=x&health=y&sort=z")), { status: "open", health: "any", sort: "attention" });
});

test("filter query leaves defaults out", () => {
  assert.equal(epicFilterQuery({ status: "open", health: "any", sort: "attention" }), "");
  assert.equal(epicFilterQuery({ status: "all", health: "off_track", sort: "target" }, "WEB"), "?project=WEB&status=all&health=off_track&sort=target");
});

test("status and health filters", () => {
  const list = [make("a", "on_track"), make("b", "at_risk", { epic: { status: "done" } }), make("c", "at_risk")];
  assert.deepEqual(statusCounts(list), { open: 2, done: 1, all: 3 });
  assert.deepEqual(filterEpics(list, { status: "open", health: "any" }).map((e) => e.epic.name), ["a", "c"]);
  assert.deepEqual(filterEpics(list, { status: "all", health: "at_risk" }).map((e) => e.epic.name), ["b", "c"]);
  assert.deepEqual(filterEpics(list, { status: "done", health: "on_track" }), []);
});

test("needs attention sorts by health, then nearest target, undated last", () => {
  const list = [
    make("on", "on_track", { target_date: "2026-11-01" }),
    make("risk-late", "at_risk", { target_date: "2026-12-01" }),
    make("risk-none", "at_risk"),
    make("off", "off_track"),
    make("risk-soon", "at_risk", { target_date: "2026-11-10" }),
  ];
  assert.deepEqual(sortEpics(list, "attention").map((e) => e.epic.name), ["off", "risk-soon", "risk-late", "risk-none", "on"]);
  assert.deepEqual(sortEpics(list, "target").map((e) => e.epic.name), ["on", "risk-soon", "risk-late", "off", "risk-none"]);
  assert.deepEqual(sortEpics(list, "name").map((e) => e.epic.name), ["off", "on", "risk-late", "risk-none", "risk-soon"]);
  const progress = [make("half", "on_track", { epic: { points_done: 5 } }), make("none", "on_track", { epic: { points_total: 0 } }), make("full", "on_track", { epic: { points_done: 10 } })];
  assert.deepEqual(sortEpics(progress, "progress").map((e) => e.epic.name), ["full", "half", "none"]);
});

test("overdue only for open epics past their target", () => {
  assert.equal(isOverdue({ epic: { status: "open" }, target_date: "2026-11-01" }, "2026-11-07"), true);
  assert.equal(isOverdue({ epic: { status: "done" }, target_date: "2026-11-01" }, "2026-11-07"), false);
  assert.equal(isOverdue({ epic: { status: "open" }, target_date: "2026-11-07" }, "2026-11-07"), false);
  assert.equal(isOverdue({ epic: { status: "open" } }, "2026-11-07"), false);
});

test("burn-up accumulates done points per day from the start date to today", () => {
  const at = (d) => new Date(`${d}T12:00:00`).toISOString();
  const tickets = [
    { status: "done", estimate_points: 3, updated_at: at("2026-11-03") },
    { status: "done", estimate_points: 2, updated_at: at("2026-11-03") },
    { status: "done", estimate_points: 5, updated_at: at("2026-11-05") },
    { status: "done", updated_at: at("2026-11-04") },
    { status: "todo", estimate_points: 8, updated_at: at("2026-11-06") },
    { status: "canceled", estimate_points: 13, updated_at: at("2026-11-02") },
  ];
  assert.deepEqual(burnUp(tickets, { start: "2026-11-01", today: "2026-11-07" }), {
    scope: 18, done: 10,
    points: [{ date: "2026-11-01", done: 0 }, { date: "2026-11-03", done: 5 }, { date: "2026-11-05", done: 10 }, { date: "2026-11-07", done: 10 }],
  });
  // Without a start date the series starts at the first completion.
  assert.deepEqual(burnUp(tickets, { today: "2026-11-05" }).points, [{ date: "2026-11-03", done: 5 }, { date: "2026-11-05", done: 10 }]);
  assert.deepEqual(burnUp([], { today: "2026-11-07" }), { scope: 0, done: 0, points: [{ date: "2026-11-07", done: 0 }] });
});
