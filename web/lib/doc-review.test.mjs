import test from "node:test";
import assert from "node:assert/strict";
import { resolveReview, reviewBase, reviewNote, reviewSegments, splitBlocks } from "./doc-review.ts";

test("changes count from the person's last edit or finished review", () => {
  const versions = [
    { seq: 4, author_type: "agent" },
    { seq: 3, author_type: "agent" },
    { seq: 2, author_type: "user" },
    { seq: 1, author_type: "user" },
  ];
  assert.equal(reviewBase(versions), 2);
  assert.equal(reviewBase(versions, 3), 3);
  assert.equal(reviewBase(versions, 4), 4);
  assert.equal(reviewBase([{ seq: 1, author_type: "agent" }]), 0);
});

test("a document splits into its top-level blocks", () => {
  assert.deepEqual(splitBlocks("# T\n\nOne\ntwo.\n\n- a\n- b\n\n```\nx\n\ny\n```\n"), ["# T", "One\ntwo.", "- a\n- b", "```\nx\n\ny\n```"]);
});

const base = "# Plan\n\nFirst.\n\nSecond.\n\nThird.\n";
const head = "# Plan\n\nFirst, rewritten.\n\nSecond.\n\nThird.\n\nA new ending.\n";

test("each run of changed blocks is one change", () => {
  const segs = reviewSegments(base, head);
  assert.deepEqual(segs.map((s) => s.kind), ["same", "change", "same", "change"]);
  assert.deepEqual(segs[1], { kind: "change", id: 0, before: ["First."], after: ["First, rewritten."] });
  assert.deepEqual(segs[3], { kind: "change", id: 1, before: [], after: ["A new ending."] });
});

test("rejecting a change puts the earlier blocks back, accepting keeps the persona's", () => {
  const segs = reviewSegments(base, head);
  assert.equal(resolveReview(segs, { 0: "reject", 1: "accept" }), "# Plan\n\nFirst.\n\nSecond.\n\nThird.\n\nA new ending.\n");
  assert.equal(resolveReview(segs, { 0: "accept", 1: "reject" }), "# Plan\n\nFirst, rewritten.\n\nSecond.\n\nThird.\n");
  assert.equal(resolveReview(segs, { 0: "reject", 1: "reject" }), base);
});

test("the version note says what the person kept", () => {
  const segs = reviewSegments(base, head);
  assert.equal(reviewNote(segs, { 0: "reject", 1: "accept" }, "Maya"), "Kept 1 of Maya's 2 changes, rejected 1");
  assert.equal(reviewNote(segs, { 0: "reject", 1: "reject" }, "Maya"), "Rejected Maya's 2 changes");
});
