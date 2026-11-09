import assert from "node:assert/strict";
import { test } from "node:test";
import { needsCommitEmailPick, previewHash } from "./commit-email.ts";

test("needsCommitEmailPick asks only in a repo chat with several addresses and no valid pick", () => {
  const two = ["ada@example.com", "ada@work.example"];
  const repo = "git@github.com:acme/app.git";
  assert.equal(needsCommitEmailPick(two, repo, undefined), true);
  assert.equal(needsCommitEmailPick(two, repo, "ADA@work.example"), false);
  // An address since removed from the profile asks again.
  assert.equal(needsCommitEmailPick(two, repo, "old@example.com"), true);
  assert.equal(needsCommitEmailPick(["ada@example.com"], repo, undefined), false);
  assert.equal(needsCommitEmailPick(two, undefined, undefined), false);
});

test("previewHash is seven hex characters, stable and case-insensitive", () => {
  assert.match(previewHash("ada@example.com"), /^[0-9a-f]{7}$/);
  assert.equal(previewHash("Ada@Example.com"), previewHash("ada@example.com"));
  assert.notEqual(previewHash("ada@example.com"), previewHash("ada@work.example"));
});
