import assert from "node:assert/strict";
import { test } from "node:test";
import { assistantLine, credentialPlaceholder, draftBullets, draftPrompt, harnessHint, initialOf, needsWizard, prefillName, refinePrompt, rowState, setupProgress } from "./onboarding.ts";

test("needsWizard until onboarded, and again without a harness", () => {
  assert.equal(needsWizard(undefined), false);
  assert.equal(needsWizard({ harness_connected: true }), true);
  assert.equal(needsWizard({ onboarded_at: "2026-11-11T00:00:00Z", harness_connected: true }), false);
  assert.equal(needsWizard({ onboarded_at: "2026-11-11T00:00:00Z", harness_connected: false }), true);
  // Older servers don't send harness_connected: only onboarded_at counts.
  assert.equal(needsWizard({ onboarded_at: "2026-11-11T00:00:00Z" }), false);
});

test("progress fills as steps finish and rows follow the current step", () => {
  assert.deepEqual([0, 1, 2, 3, 4].map(setupProgress), [0, 0, 1 / 3, 2 / 3, 1]);
  assert.deepEqual([0, 1, 2].map((r) => rowState(r, 2)), ["done", "current", "todo"]);
  assert.deepEqual([0, 1, 2].map((r) => rowState(r, 4)), ["done", "done", "done"]);
  assert.deepEqual([0, 1, 2].map((r) => rowState(r, 0)), ["todo", "todo", "todo"]);
});

test("the Assistant greets by time of day", () => {
  assert.match(assistantLine(0, 8), /^Good morning\./);
  assert.match(assistantLine(0, 14), /^Good afternoon\./);
  assert.match(assistantLine(0, 22), /^Good evening\./);
  assert.match(assistantLine(2, 9), /Claude Code, Codex or both/);
});

test("profile helpers", () => {
  assert.equal(initialOf(" liam"), "L");
  assert.equal(initialOf(""), "?");
  assert.equal(prefillName("Local user"), "");
  assert.equal(prefillName("Ada"), "Ada");
});

test("harness hint and placeholders", () => {
  assert.equal(harnessHint([]), "Connect at least one to continue.");
  assert.equal(harnessHint(["claude-code"]), "Image generation stays off until Codex is connected.");
  assert.equal(harnessHint(["codex"]), "");
  assert.equal(credentialPlaceholder("codex", "subscription"), "Paste auth.json");
  assert.equal(credentialPlaceholder("claude-code", "api_key"), "ANTHROPIC_API_KEY");
});

test("setup prompts carry the marker and the person's words", () => {
  assert.match(draftPrompt("  A calm health coach "), /^\(Vesper setup\).*"A calm health coach"/s);
  assert.match(draftPrompt("x"), /propose_persona/);
  assert.match(refinePrompt("make it funnier"), /^\(Vesper setup\) Change the draft: "make it funnier"/);
});

test("draftBullets turns instructions into three short lines", () => {
  assert.deepEqual(draftBullets("You reproduce a bug before fixing it. You lead with the answer. You push back with alternatives. You never gloat."), [
    "Reproduce a bug before fixing it",
    "Lead with the answer",
    "Push back with alternatives",
  ]);
  assert.deepEqual(draftBullets("- Short scenes\n- Tracks the party"), ["Short scenes", "Tracks the party"]);
  assert.deepEqual(draftBullets(""), []);
});
