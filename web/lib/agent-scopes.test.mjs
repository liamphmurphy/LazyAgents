import assert from "node:assert/strict";
import test from "node:test";
import {
  classifyChange,
  collapseDiff,
  countModes,
  effectiveMode,
  liveCounts,
  matches,
  modeFor,
  scopeChanges,
  scopeRows,
  skillViews,
} from "./agent-scopes.ts";

test("wildcards match their prefix only", () => {
  assert.equal(matches("prod:*", "prod:deploy"), true);
  assert.equal(matches("prod:*", "production:x"), false);
  assert.equal(matches("repo:read", "repo:read"), true);
  assert.equal(matches("repo:read", "repo:write"), false);
});

test("modeFor takes the strictest match and denies unknown scopes", () => {
  assert.deepEqual(modeFor({ "db:*": "allowed", "db:migrate": "requires_approval" }, "db:migrate"), { mode: "requires_approval", found: true });
  assert.deepEqual(modeFor({ "repo:read": "allowed" }, "db:migrate"), { mode: "denied", found: false });
});

test("projects only tighten, mirroring agentdef.Resolve", () => {
  const agent = { "repo:read": "allowed", "db:migrate": "requires_approval", "tests:run": "denied" };
  // Tightening is attributed to the project.
  assert.deepEqual(effectiveMode(agent, { "db:migrate": "denied" }, "db:migrate"), { mode: "denied", source: "project" });
  // Loosening is ignored.
  assert.deepEqual(effectiveMode(agent, { "tests:run": "allowed" }, "tests:run"), { mode: "denied", source: "agent" });
  // A project can't grant what the agent doesn't; naming it records the denial.
  assert.deepEqual(effectiveMode(agent, { "deploy:staging": "allowed" }, "deploy:staging"), { mode: "denied", source: "project" });
  // Wildcards in the override cover matching scopes.
  assert.deepEqual(effectiveMode({ "prod:deploy": "allowed" }, { "prod:*": "denied" }, "prod:deploy"), { mode: "denied", source: "project" });
  // No override: the agent's own level.
  assert.deepEqual(effectiveMode(agent, undefined, "repo:read"), { mode: "allowed", source: "agent" });
  assert.deepEqual(effectiveMode(agent, undefined, "wiki:write"), { mode: "denied", source: "agent" });
});

test("scope rows: catalog first, then custom agent scopes, then project-only names", () => {
  const rows = scopeRows(["repo:read", "db:migrate"], { "zz:custom": "allowed", "repo:read": "allowed" }, { "prod:*": "denied", "db:migrate": "denied" });
  assert.deepEqual(rows.map((r) => r.scope), ["repo:read", "db:migrate", "zz:custom", "prod:*"]);
  const byName = Object.fromEntries(rows.map((r) => [r.scope, r]));
  assert.equal(byName["db:migrate"].definition, undefined);
  assert.equal(byName["db:migrate"].granted, false);
  assert.equal(byName["db:migrate"].projectOnly, false);
  assert.equal(byName["prod:*"].projectOnly, true);
  assert.deepEqual(byName["prod:*"].effective, { mode: "denied", source: "project" });
  assert.deepEqual(byName["zz:custom"].effective, { mode: "allowed", source: "agent" });
});

test("change classification: approval is always called out, widening is separate", () => {
  assert.equal(classifyChange("denied", "requires_approval"), "approval");
  assert.equal(classifyChange(undefined, "requires_approval"), "approval");
  assert.equal(classifyChange("allowed", "requires_approval"), "approval");
  assert.equal(classifyChange(undefined, "allowed"), "widens");
  assert.equal(classifyChange("requires_approval", "allowed"), "widens");
  assert.equal(classifyChange("allowed", "denied"), "tightens");
  assert.equal(classifyChange("allowed", undefined), "tightens");
  assert.deepEqual(
    scopeChanges({ "a:x": "allowed", "b:y": "denied" }, { "a:x": "allowed", "b:y": "requires_approval", "c:z": "allowed" }),
    [
      { scope: "b:y", from: "denied", to: "requires_approval", kind: "approval" },
      { scope: "c:z", from: undefined, to: "allowed", kind: "widens" },
    ],
  );
});

test("countModes counts each level", () => {
  assert.deepEqual(countModes([{ mode: "allowed" }, { mode: "denied" }, { mode: "denied" }]), { allowed: 1, requires_approval: 0, denied: 2 });
});

test("collapseDiff numbers lines and folds long unchanged runs", () => {
  const same = (t) => ({ op: "same", text: t });
  const lines = [same("1"), same("2"), same("3"), same("4"), same("5"), { op: "del", text: "6" }, { op: "add", text: "6b" }, same("7"), same("8"), same("9")];
  const rows = collapseDiff(lines, 2);
  assert.deepEqual(rows[0], { kind: "gap", count: 3 });
  assert.deepEqual(rows[1], { kind: "line", op: "same", text: "4", oldNo: 4, newNo: 4 });
  assert.deepEqual(rows[3], { kind: "line", op: "del", text: "6", oldNo: 6, newNo: undefined });
  assert.deepEqual(rows[4], { kind: "line", op: "add", text: "6b", oldNo: undefined, newNo: 6 });
  assert.deepEqual(rows.at(-1), { kind: "gap", count: 1 });
  assert.equal(collapseDiff([same("a")], 2)[0].kind, "gap");
});

test("liveCounts: planning runs are live; open tickets dedupe live tickets", () => {
  const o = { assignments: [{ ticket: { key: "A-1" } }, { ticket: { key: "A-1" } }, { ticket: { key: "A-2" } }], planning_runs: [{}], queue_count: 4 };
  assert.deepEqual(liveCounts(o), { live: 4, open: 6 });
});

test("skillViews marks project removals and appends project additions", () => {
  const k = (name) => ({ name, description: name, body: "" });
  const draft = [k("git"), k("pr"), k("notes"), k("fresh")];
  const views = skillViews(draft, ["git", "pr", "notes"], [k("git"), k("pr"), k("wiki")]);
  assert.deepEqual(views.map((v) => [v.skill.name, v.origin]), [
    ["git", "agent"],
    ["pr", "agent"],
    ["notes", "removed"],
    ["fresh", "agent"],
    ["wiki", "added"],
  ]);
  assert.deepEqual(skillViews(draft, [], undefined).map((v) => v.origin), ["agent", "agent", "agent", "agent"]);
});
