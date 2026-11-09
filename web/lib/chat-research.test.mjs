import test from "node:test";
import assert from "node:assert/strict";
import { isResearchPlanFence, lengthOf, minutesLabel, parseResearchPlan, toneOf, planReady, reportSections, researchActive, researchProgress, spendLabel } from "./chat-research.ts";

test("a plan is a title then questions", () => {
  assert.ok(isResearchPlanFence("research-plan"));
  assert.deepEqual(parseResearchPlan("Local TTS\n- Quality | Which | is best?\n- Just a question\n"), {
    title: "Local TTS",
    questions: [{ label: "Quality", question: "Which | is best?" }, { label: "Just a question", question: "Just a question" }],
  });
});

test("a plan may preset the report's length and tone", () => {
  assert.deepEqual(parseResearchPlan("WWII\nLength: in-depth\nTone: News article\n- Causes | Why?\n"), {
    title: "WWII", length: "in_depth", tone: "news", questions: [{ label: "Causes", question: "Why?" }],
  });
  // Unknown values are ignored, and a title that looks like a setting stays the title.
  assert.deepEqual(parseResearchPlan("Tone: a history\nLength: epic\n- A | q"), { title: "Tone: a history", questions: [{ label: "A", question: "q" }] });
  assert.equal(lengthOf("Detailed"), "detailed");
  assert.equal(toneOf("essay"), "essay");
  assert.equal(toneOf("limerick"), undefined);
});

test("a plan without a title or questions, or with too many, isn't a plan", () => {
  assert.equal(parseResearchPlan("- a | b"), null);
  assert.equal(parseResearchPlan("Title only"), null);
  assert.equal(parseResearchPlan("T\n" + Array.from({ length: 6 }, (_, i) => `- ${i} | q`).join("\n")), null);
});

test("limits stay in range and questions must be filled in", () => {
  const p = { title: "T", questions: [{ label: "A", question: "q" }] };
  assert.ok(planReady(p, 60, 30));
  assert.ok(!planReady(p, 201, 30));
  assert.ok(!planReady(p, 60, 4));
  assert.ok(!planReady({ title: "T", questions: [{ label: "A", question: " " }] }, 60, 30));
});

test("labels and progress", () => {
  assert.equal(spendLabel(3.4, 6), "$3.40 of $6");
  assert.equal(spendLabel(3.4, undefined), "no spend limit");
  assert.equal(minutesLabel(18 * 60000), "18 min");
  assert.equal(minutesLabel(1000), "<1 min");
  assert.ok(researchActive("writing"));
  assert.ok(!researchActive("done"));
  assert.equal(researchProgress([{ state: "done" }, { state: "running" }], 3, 60), 0.5);
  assert.equal(researchProgress([{ state: "running" }], 90, 60), 1);
});

test("sections come from second-level headings", () => {
  const anchors = new Map([[1, "comparison"], [3, "not-covered"]]);
  assert.deepEqual(reportSections("## Comparison\nx\n## Not covered", anchors), [{ title: "Comparison", anchor: "comparison" }, { title: "Not covered", anchor: "not-covered" }]);
});
