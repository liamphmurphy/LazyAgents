import test from "node:test";
import assert from "node:assert/strict";
import { attachmentBadge, attachmentProblem, MAX_ATTACHMENT_BYTES, pastedImageName } from "./attachments.ts";

test("attachments are checked by type, emptiness and size", () => {
  assert.equal(attachmentProblem({ name: "Spec.PDF", size: 10 }), undefined);
  assert.equal(attachmentProblem({ name: "a.exe", size: 10 }), "type not supported");
  assert.equal(attachmentProblem({ name: "noext", size: 10 }), "type not supported");
  assert.equal(attachmentProblem({ name: "a.txt", size: 0 }), "file is empty");
  assert.equal(attachmentProblem({ name: "a.csv", size: MAX_ATTACHMENT_BYTES + 1 }), "over 25 MB");
  assert.equal(attachmentProblem({ name: "a.csv", size: MAX_ATTACHMENT_BYTES }), undefined);
});

test("pasted images are named by their type, so only supported ones pass", () => {
  assert.equal(pastedImageName("image/png", 1), "pasted-image-1.png");
  assert.equal(pastedImageName("image/jpeg", 2), "pasted-image-2.jpg");
  assert.equal(attachmentProblem({ name: pastedImageName("image/webp", 3), size: 10 }), undefined);
  assert.equal(attachmentProblem({ name: pastedImageName("image/svg+xml", 4), size: 10 }), "type not supported");
  assert.equal(attachmentProblem({ name: pastedImageName("image/tiff", 5), size: 10 }), "type not supported");
});

test("badges group images and name the rest", () => {
  assert.equal(attachmentBadge("a.jpeg"), "IMG");
  assert.equal(attachmentBadge("notes.md"), "MD");
  assert.equal(attachmentBadge("x.json"), "JSON");
  assert.equal(attachmentBadge("x.bin"), "FILE");
});
