import test from "node:test";
import assert from "node:assert/strict";
import { attachmentBadge, attachmentProblem, MAX_ATTACHMENT_BYTES } from "./attachments.ts";

test("attachments are checked by type, emptiness and size", () => {
  assert.equal(attachmentProblem({ name: "Spec.PDF", size: 10 }), undefined);
  assert.equal(attachmentProblem({ name: "a.exe", size: 10 }), "type not supported");
  assert.equal(attachmentProblem({ name: "noext", size: 10 }), "type not supported");
  assert.equal(attachmentProblem({ name: "a.txt", size: 0 }), "file is empty");
  assert.equal(attachmentProblem({ name: "a.csv", size: MAX_ATTACHMENT_BYTES + 1 }), "over 25 MB");
  assert.equal(attachmentProblem({ name: "a.csv", size: MAX_ATTACHMENT_BYTES }), undefined);
});

test("badges group images and name the rest", () => {
  assert.equal(attachmentBadge("a.jpeg"), "IMG");
  assert.equal(attachmentBadge("notes.md"), "MD");
  assert.equal(attachmentBadge("x.json"), "JSON");
  assert.equal(attachmentBadge("x.bin"), "FILE");
});
