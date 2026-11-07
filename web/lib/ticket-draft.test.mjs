import test from "node:test";
import assert from "node:assert/strict";
import { buildPatch, classifyError, draftFromSnapshot, moveCriterion, newCriterion, snapshotFromTicket } from "./ticket-draft.ts";

const ticket = {
  revision: 4, title: "T", description_md: "d", status: "todo", priority: "high", estimate_points: 3,
  assignee: { id: "a1" }, epic: { id: "e1" },
  criteria: [{ id: "c1", position: 1, text: "one", done: false }, { id: "c2", position: 2, text: "two", done: true }],
};
const base = snapshotFromTicket(ticket);

test("no changes yields no patch", () => {
  assert.equal(buildPatch(base, draftFromSnapshot(base)), null);
});

test("only changed fields plus revision are sent, criteria omitted when untouched", () => {
  const d = draftFromSnapshot(base);
  d.title = "New"; d.priority = "low";
  assert.deepEqual(buildPatch(base, d), { revision: 4, title: "New", priority: "low" });
});

test("null clears estimate, assignee and epic", () => {
  const d = draftFromSnapshot(base);
  d.estimate_points = null; d.assignee_agent_id = null; d.epic_id = null;
  assert.deepEqual(buildPatch(base, d), { revision: 4, estimate_points: null, assignee_agent_id: null, epic_id: null });
});

test("missing estimate/assignee/epic baseline is null, so no spurious change", () => {
  const b = snapshotFromTicket({ ...ticket, estimate_points: undefined, assignee: undefined, epic: undefined });
  assert.equal(buildPatch(b, draftFromSnapshot(b)), null);
});

test("criteria edits send one draft with the baseline as expected", () => {
  const d = draftFromSnapshot(base);
  d.criteria = moveCriterion(d.criteria, 1, -1);
  d.criteria[1] = { ...d.criteria[1], done: true };
  d.criteria.push({ ...newCriterion(), text: "three" });
  const patch = buildPatch(base, d);
  assert.deepEqual(patch.criteria.expected, base.criteria);
  assert.deepEqual(patch.criteria.items, [
    { id: "c2", text: "two", done: true }, { id: "c1", text: "one", done: true }, { text: "three", done: false },
  ]);
});

test("deleting a criterion omits its id; blank new rows are ignored", () => {
  const d = draftFromSnapshot(base);
  d.criteria = [d.criteria[0], newCriterion()];
  assert.deepEqual(buildPatch(base, d).criteria.items, [{ id: "c1", text: "one", done: false }]);
  const blankOnly = draftFromSnapshot(base);
  blankOnly.criteria.push(newCriterion());
  assert.equal(buildPatch(base, blankOnly), null);
});

test("moveCriterion is bounded and does not mutate", () => {
  const list = draftFromSnapshot(base).criteria;
  assert.equal(moveCriterion(list, 0, -1), list);
  assert.equal(moveCriterion(list, 1, 1), list);
  assert.equal(moveCriterion(list, 0, 1)[0].id, "c2");
  assert.equal(list[0].id, "c1");
});

test("classifyError separates stale conflicts, field errors and general errors", () => {
  assert.equal(classifyError(409, "revision", "x").conflict, true);
  assert.equal(classifyError(409, "criteria", "x").conflict, true);
  assert.deepEqual(classifyError(409, undefined, "cancel the run first"), { conflict: false, fields: {}, general: "cancel the run first" });
  assert.deepEqual(classifyError(422, "title", "bad"), { conflict: false, fields: { title: "bad" } });
  assert.deepEqual(classifyError(422, "mystery", "bad"), { conflict: false, fields: {}, general: "bad" });
});
