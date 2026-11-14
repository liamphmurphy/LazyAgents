import test from "node:test";
import assert from "node:assert/strict";
import { designBriefTitle, isDeckBrief, isDesignBrief, splitOutline } from "./start.ts";

test("a deck's start box tells a title from a brief", () => {
  assert.equal(isDeckBrief("Q4 review"), false);
  assert.equal(isDeckBrief("Make a deck about the homelab"), true);
  assert.equal(isDeckBrief("a short talk on how we run agents in containers at home"), true);
});

test("a design's start box drafts from a request or a ticket key, and titles it with the key", () => {
  assert.equal(isDesignBrief("Checkout"), false);
  assert.equal(isDesignBrief("Turn LAZYAGENTS-150 into a flow"), true);
  assert.equal(isDesignBrief("LAZYAGENTS-150"), true);
  assert.equal(designBriefTitle("Turn LAZYAGENTS-150 into a flow"), "LAZYAGENTS-150");
  assert.equal(designBriefTitle("Sketch the sign-in flow"), "");
});

test("an outline's first line titles the deck unless a title was typed", () => {
  assert.deepEqual(splitOutline("\n- Q3 roadmap\nWhy now\n  - churn\n", ""), { title: "Q3 roadmap", slides: "Why now\n  - churn" });
  assert.deepEqual(splitOutline("Why now\nPlan", "Roadmap"), { title: "Roadmap", slides: "Why now\nPlan" });
  assert.deepEqual(splitOutline("Only a title", ""), { title: "Only a title", slides: "" });
  assert.deepEqual(splitOutline("  \n", ""), { title: "", slides: "" });
});
