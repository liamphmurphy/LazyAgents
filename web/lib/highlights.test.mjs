import assert from "node:assert/strict";
import test from "node:test";
import { routingHighlights } from "./highlights.ts";

const bucket = (over = {}) => ({ persona_id: "p", persona: "Persona", harness: "codex", model: "model", effort: "", effort_level: "", reasons: [], turns: 1, duration_ms: 0, timed_turns: 0, ...over });

test("routing counts distinguish automatic, chosen and unclassified history", () => {
  const h = routingHighlights([
    bucket({ turns: 3, effort_level: "xhigh", effort: "high", reasons: ["confirmation", "effort_mismatch"], duration_ms: 9000, timed_turns: 3 }),
    bucket({ turns: 2, effort_level: "low" }), // Auto, but actual effort unavailable.
    bucket({ turns: 4, effort: "medium", persona_id: "guest", persona: "Guest" }),
    bucket({ turns: 5 }), // Legacy is not counted as manual.
  ]);
  assert.deepEqual([h.turns, h.automatic, h.chosen, h.unknown], [14, 5, 4, 5]);
  assert.deepEqual([h.raised, h.adjusted, h.averageMs], [3, 3, 3000]);
  assert.deepEqual(h.reasons.map((r) => r.turns), [3, 3]);
  assert.equal(h.personas.find((r) => r.key === "guest").turns, 4);
  assert.equal(h.models[0].turns, 14);
  assert.equal(h.levels.reduce((n, r) => n + r.turns, 0), h.automatic);
  assert.equal(h.efforts.reduce((n, r) => n + r.turns, 0), h.turns);
});

test("empty and unknown samples do not manufacture timing or routing decisions", () => {
  const h = routingHighlights([]);
  assert.equal(h.averageMs, undefined);
  assert.equal(h.turns, 0);
  const unknown = routingHighlights([bucket({ persona_id: "", persona: "Unknown persona", model: "" })]);
  assert.equal(unknown.unknown, 1);
  assert.equal(unknown.models[0].label, "Unknown model");
  assert.equal(unknown.averageMs, undefined);
});
