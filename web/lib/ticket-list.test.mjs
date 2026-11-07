import test from "node:test";
import assert from "node:assert/strict";
import {
  applyLocalFilters,
  describeQuery,
  failureReason,
  groupTickets,
  moveFocus,
  parseGroup,
  rangeKeys,
  runEach,
  savedViewHref,
  viewQuery,
} from "./ticket-list.ts";

const t = (key, over = {}) => ({
  id: key, key, project_key: "LA", title: key, status: "todo", priority: "none", labels: [],
  criteria_total: 2, criteria_done: 1, unmet_dependencies: 0, updated_at: "2026-11-06T12:00:00Z", ...over,
});
const ux = { id: "e2", name: "UI & UX overhaul" };
const runner = { id: "e1", name: "runner reliability" };
const maya = { id: "a1", name: "mastermind", handle: "mastermind", color_index: 0 };
const ivan = { id: "a2", name: "Implementer", handle: "implementer", color_index: 1 };

test("parseGroup defaults to status", () => {
  assert.equal(parseGroup(null), "status");
  assert.equal(parseGroup("nonsense"), "status");
  assert.equal(parseGroup("epic"), "epic");
});

test("groups by status in STATUS_ORDER, with sums (canceled points excluded)", () => {
  const gs = groupTickets([t("A", { status: "done", estimate_points: 3 }), t("B", { status: "in_progress", estimate_points: 5 }), t("C", { status: "canceled", estimate_points: 8 })], "status");
  assert.deepEqual(gs.map((g) => g.id), ["in_progress", "done", "canceled"]);
  assert.equal(gs[0].points, 5);
  assert.equal(gs[2].points, 0);
  assert.equal(gs[0].criteriaDone, 1);
  assert.equal(gs[0].criteriaTotal, 2);
});

test("epic and assignee groups sort A–Z (case-insensitive) with none last", () => {
  const list = [t("A"), t("B", { epic: ux, assignee: ivan }), t("C", { epic: runner, assignee: maya }), t("D", { epic: ux })];
  const byEpic = groupTickets(list, "epic");
  assert.deepEqual(byEpic.map((g) => g.id), ["e1", "e2", "none"]);
  assert.deepEqual(byEpic[1].tickets.map((x) => x.key), ["B", "D"]);
  const byAgent = groupTickets(list, "assignee");
  assert.deepEqual(byAgent.map((g) => g.name), ["Implementer", "mastermind", ""]);
});

test("priority groups run urgent to no priority", () => {
  const gs = groupTickets([t("A", { priority: "low" }), t("B", { priority: "urgent" }), t("C")], "priority");
  assert.deepEqual(gs.map((g) => g.id), ["urgent", "low", "none"]);
});

test("local filters: blocked open tickets, unestimated open tickets", () => {
  const list = [t("A", { unmet_dependencies: 1 }), t("B", { unmet_dependencies: 2, status: "done" }), t("C", { estimate_points: 3 }), t("D", { status: "canceled" })];
  assert.deepEqual(applyLocalFilters(list, { blocked: true }).map((x) => x.key), ["A"]);
  assert.deepEqual(applyLocalFilters(list, { unestimated: true }).map((x) => x.key), ["A"]);
  assert.equal(applyLocalFilters(list, {}).length, 4);
});

test("viewQuery keeps view params in a fixed order and drops defaults, project and sv", () => {
  assert.equal(viewQuery("?project=LA&sv=x&group=epic&view=active&q=ui&ready=false"), "view=active&group=epic&q=ui");
  assert.equal(viewQuery("view=all&group=status"), "");
  assert.equal(viewQuery(new URLSearchParams("blocked=true&assignee=builder")), "assignee=builder&blocked=true");
});

test("savedViewHref keeps the project and marks the view", () => {
  assert.equal(savedViewHref({ id: "v1", query: "group=epic&view=active" }, "LA"), "/tickets?project=LA&sv=v1&view=active&group=epic");
  assert.equal(savedViewHref({ id: "v1", query: "" }), "/tickets?sv=v1");
});

test("describeQuery summarizes a query", () => {
  assert.equal(describeQuery("view=active&group=epic", { view: (v) => v[0].toUpperCase() + v.slice(1) }), "View Active · Group Epic · Agent any · Search none");
});

test("moveFocus and rangeKeys", () => {
  const ks = ["A", "B", "C", "D"];
  assert.equal(moveFocus(ks, null, 1), "A");
  assert.equal(moveFocus(ks, null, -1), "D");
  assert.equal(moveFocus(ks, "B", 1), "C");
  assert.equal(moveFocus(ks, "D", 1), "D");
  assert.equal(moveFocus(ks, "A", -1), "A");
  assert.equal(moveFocus([], null, 1), null);
  assert.deepEqual(rangeKeys(ks, "D", "B"), ["B", "C", "D"]);
  assert.deepEqual(rangeKeys(ks, null, "C"), ["C"]);
  assert.deepEqual(rangeKeys(ks, "A", "Z"), []);
});

test("runEach saves each ticket on its own and reports failures in key order", async () => {
  let live = 0;
  let peak = 0;
  const res = await runEach(["A", "B", "C", "D"], async (k) => {
    live++;
    peak = Math.max(peak, live);
    await new Promise((r) => setTimeout(r, k === "A" ? 10 : 1));
    live--;
    if (k === "A" || k === "C") throw Object.assign(new Error("cancel the run first"), { status: 409 });
  }, 2);
  assert.deepEqual(res.updated, ["B", "D"]);
  assert.deepEqual(res.failed.map((f) => [f.key, f.status]), [["A", 409], ["C", 409]]);
  assert.ok(peak <= 2);
  assert.equal(failureReason(res.failed[0]), "Cancel the run first: a run is live on this ticket.");
  assert.equal(failureReason({ key: "X", status: 422, message: "estimate_points: bad" }), "Estimate_points: bad");
});
