import test from "node:test";
import assert from "node:assert/strict";
import { engineChoices, engineSummary } from "./image-engine.ts";

const codex = { engine: "codex", label: "Codex", available: true };
const agy = { engine: "antigravity", label: "Antigravity", available: false, reason: "no harness:google secret" };

test("Auto names the connected engines in preference order; a disconnected engine says why", () => {
  const [auto, c, a] = engineChoices([codex, agy]);
  assert.deepEqual([auto.value, c.value, a.value], ["auto", "codex", "antigravity"]);
  assert.equal(auto.detail, "Use the first connected: Codex.");
  assert.equal(c.detail, "Connected");
  assert.equal(a.detail, "Not connected: no harness:google secret");
  assert.equal(engineChoices([{ ...codex, available: false }, agy])[0].detail, "No image engine is connected yet. Connect Codex or Antigravity first.");
  assert.equal(engineChoices([codex, { ...agy, available: true }])[0].detail, "Use the first connected: Codex, then Antigravity.");
});

test("a persona or step with no engine reads as the workspace's", () => {
  assert.equal(engineSummary("", "antigravity", [codex, agy]), "Workspace setting (Antigravity)");
  assert.equal(engineSummary(undefined, "auto", [codex, agy]), "Workspace setting (Auto)");
  assert.equal(engineSummary("codex", "antigravity", [codex, agy]), "Codex");
  assert.equal(engineSummary("auto", "codex", [codex, agy]), "Auto");
});
