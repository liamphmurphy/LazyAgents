import assert from "node:assert/strict";
import { test } from "node:test";
import { editEmail, emailError, makePrimary, profileChanges, validateProfile } from "./profile-ui.ts";

test("validateProfile requires a name and caps both fields at 80", () => {
  const emails = ["maya@example.com"];
  assert.deepEqual(validateProfile({ name: "Maya", role_title: "", emails }), {});
  assert.ok(validateProfile({ name: "   ", role_title: "", emails }).name);
  assert.deepEqual(validateProfile({ name: "a".repeat(80), role_title: "b".repeat(80), emails }), {});
  assert.ok(validateProfile({ name: "a".repeat(81), role_title: "", emails }).name);
  assert.ok(validateProfile({ name: "Maya", role_title: "b".repeat(81), emails }).role_title);
  assert.ok(validateProfile({ name: "Maya", role_title: "", emails: [] }).emails);
});

test("profileChanges lists only trimmed, changed fields", () => {
  const emails = ["maya@example.com", "maya@work.example"];
  const saved = { name: "Maya", role_title: "Owner", emails };
  assert.equal(profileChanges(saved, { name: " Maya ", role_title: "Owner", emails }), null);
  assert.deepEqual(profileChanges(saved, { name: "Maya C", role_title: "Owner", emails }), { name: "Maya C" });
  assert.deepEqual(profileChanges(saved, { name: "Maya", role_title: " ", emails }), { role_title: "" });
  // Reordering changes the primary, so it counts as a change.
  assert.deepEqual(profileChanges(saved, { name: "Maya", role_title: "Owner", emails: [...emails].reverse() }), { emails: [...emails].reverse() });
});

test("emailError checks shape and repeats, ignoring case", () => {
  assert.equal(emailError(" ada@example.com ", ["me@localhost"]), undefined);
  assert.equal(emailError("me@localhost", []), undefined);
  assert.ok(emailError("", []));
  assert.ok(emailError("ada", []));
  assert.ok(emailError("Ada <ada@example.com>", []));
  assert.ok(emailError("ADA@example.com", ["ada@example.com"]));
  assert.ok(emailError("new@example.com", Array.from({ length: 10 }, (_, i) => `a${i}@example.com`)));
});

test("makePrimary moves the address to the front", () => {
  assert.deepEqual(makePrimary(["a@x.io", "b@x.io", "c@x.io"], "c@x.io"), ["c@x.io", "a@x.io", "b@x.io"]);
});

test("editEmail replaces the address in place, checked against the others", () => {
  assert.deepEqual(editEmail(["a@x.io", "b@x.io"], "a@x.io", " new@x.io "), { emails: ["new@x.io", "b@x.io"] });
  // Fixing only the case of the same address isn't a repeat.
  assert.deepEqual(editEmail(["a@x.io"], "a@x.io", "A@x.io"), { emails: ["A@x.io"] });
  assert.ok(editEmail(["a@x.io", "b@x.io"], "a@x.io", "B@x.io").error);
  assert.ok(editEmail(["a@x.io"], "a@x.io", "nope").error);
  // A full list can still be edited: the count doesn't change.
  const full = Array.from({ length: 10 }, (_, i) => `a${i}@x.io`);
  assert.equal(editEmail(full, "a3@x.io", "z@x.io").error, undefined);
});
