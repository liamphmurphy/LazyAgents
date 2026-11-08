import assert from "node:assert/strict";
import { test } from "node:test";
import { profileChanges, validateProfile } from "./profile-ui.ts";

test("validateProfile requires a name and caps both fields at 80", () => {
  assert.deepEqual(validateProfile({ name: "Maya", role_title: "" }), {});
  assert.ok(validateProfile({ name: "   ", role_title: "" }).name);
  assert.deepEqual(validateProfile({ name: "a".repeat(80), role_title: "b".repeat(80) }), {});
  assert.ok(validateProfile({ name: "a".repeat(81), role_title: "" }).name);
  assert.ok(validateProfile({ name: "Maya", role_title: "b".repeat(81) }).role_title);
});

test("profileChanges lists only trimmed, changed fields", () => {
  const saved = { name: "Maya", role_title: "Owner" };
  assert.equal(profileChanges(saved, { name: " Maya ", role_title: "Owner" }), null);
  assert.deepEqual(profileChanges(saved, { name: "Maya C", role_title: "Owner" }), { name: "Maya C" });
  assert.deepEqual(profileChanges(saved, { name: "Maya", role_title: " " }), { role_title: "" });
});
