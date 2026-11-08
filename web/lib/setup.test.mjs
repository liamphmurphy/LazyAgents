import test from "node:test";
import assert from "node:assert/strict";
import { harnessHealth, orList, setupChecklist, systemBanner } from "./setup.ts";

const groups = (anthropic, openai) => [
  { harness: "claude-code", label: "Claude Code", available: anthropic, ...(anthropic ? {} : { unavailable_reason: "no harness:anthropic secret" }) },
  { harness: "codex", label: "Codex", available: openai, ...(openai ? {} : { unavailable_reason: "No harness:openai secret" }) },
];

test("harness health names missing secrets once, by name only", () => {
  assert.equal(harnessHealth(undefined), undefined);
  const down = harnessHealth(groups(false, false));
  assert.equal(down.ready, false);
  assert.deepEqual(down.secrets, ["harness:anthropic", "harness:openai"]);
  assert.equal(down.reasons[0], "Claude Code: no harness:anthropic secret");
  assert.equal(harnessHealth(groups(true, false)).ready, true);
  assert.equal(harnessHealth([{ harness: "x", label: "X", available: false }]).reasons[0], "X: not configured");
});

test("orList joins names for a sentence", () => {
  assert.equal(orList(["a"]), "a");
  assert.equal(orList(["a", "b"]), "a or b");
  assert.equal(orList(["a", "b", "c"]), "a, b or c");
});

const base = {
  repos: 0, harnessSecret: false, harness: harnessHealth(groups(false, false)), runnerOnline: false, runners: [],
  agents: 3, agentsReviewed: false, plans: 0,
};

test("a new workspace has every step open, each linking to its settings section", () => {
  const steps = setupChecklist(base);
  assert.deepEqual(steps.map((s) => s.key), ["repo", "secret", "runner", "agents", "plan"]);
  assert.ok(steps.every((s) => !s.done));
  assert.deepEqual(steps.slice(0, 3).map((s) => s.href), ["/settings#projects", "/settings#secrets", "/settings#runners"]);
  assert.match(steps[1].detail, /Claude Code: no harness:anthropic secret/);
});

test("steps tick from real state", () => {
  const steps = setupChecklist({
    ...base, repos: 1, harness: harnessHealth(groups(true, false)), runnerOnline: true,
    runners: [{ backend: "docker", max_concurrent: 4 }], agentsReviewed: true, plans: 0,
  });
  assert.deepEqual(steps.map((s) => s.done), [true, true, true, true, false]);
  assert.equal(steps[2].detail, "1 runner online · docker · 4 slots");
  // A stored key counts even before model discovery reports the harness.
  assert.equal(setupChecklist({ ...base, harnessSecret: true })[1].done, true);
  // Agents: reviewed in this browser, or already used for a plan; never with no agents.
  assert.equal(setupChecklist({ ...base, plans: 1 })[3].done, true);
  assert.equal(setupChecklist({ ...base, agents: 0, agentsReviewed: true })[3].done, false);
});

test("the shell banner prefers the harness problem, then the runner", () => {
  assert.equal(systemBanner(harnessHealth(groups(false, false)), true), "harness");
  assert.equal(systemBanner(harnessHealth(groups(true, false)), false), "runner");
  assert.equal(systemBanner(harnessHealth(groups(true, false)), true), undefined);
  // Unknown runner state (still loading or failed) doesn't raise a banner.
  assert.equal(systemBanner(undefined, undefined), undefined);
});
