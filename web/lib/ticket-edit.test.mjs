import test from "node:test";
import assert from "node:assert/strict";
import { changesOf, errorField, formOf, patchOf, reapply } from "./ticket-edit.ts";

const ticket = {
  key: "WEB-1", title: "Old", description_md: "body", status: "todo", priority: "medium", revision: 4,
  assignee: { id: "a1", name: "Impl", handle: "implementer", color_index: 0 },
};

test("formOf normalizes absent optional fields to null", () => {
  const f = formOf(ticket);
  assert.equal(f.estimate_points, null);
  assert.equal(f.epic_id, null);
  assert.equal(f.assignee_agent_id, "a1");
});

test("the patch carries only changed fields and the base revision", () => {
  const base = formOf(ticket);
  const changes = changesOf(base, { ...base, title: "New", estimate_points: 5, assignee_agent_id: null });
  assert.deepEqual(patchOf(changes, ticket.revision), { title: "New", estimate_points: 5, assignee_agent_id: null, revision: 4 });
  assert.deepEqual(changesOf(base, { ...base }), {});
});

test("reapply keeps the latest server values for fields the person did not touch", () => {
  const latest = formOf({ ...ticket, title: "Theirs", priority: "urgent", revision: 5 });
  const next = reapply(latest, { title: "Mine", estimate_points: 8 });
  assert.equal(next.title, "Mine");
  assert.equal(next.priority, "urgent");
  assert.equal(next.estimate_points, 8);
});

test("errorField only maps form fields", () => {
  assert.equal(errorField("estimate_points"), "estimate_points");
  assert.equal(errorField("epic_id"), "epic_id");
  assert.equal(errorField("label_ids"), undefined);
  assert.equal(errorField(undefined), undefined);
});
