import test from "node:test";
import assert from "node:assert/strict";
import { CONNECTIONS, connectionState, missingConnections, needsAttention, otherSecrets, savedMessage, signInView } from "./settings.ts";

const at = "2026-11-01T00:00:00Z";
const ws = (name) => ({ id: name, name, updated_at: at });
const pay = { key: "PAY", name: "Payments", color: "#22c55e" };

test("connections cover both harness keys and both git credentials", () => {
  assert.deepEqual(CONNECTIONS.map((c) => c.name), ["harness:anthropic", "harness:openai", "git:ssh", "git:pat"]);
  assert.deepEqual(CONNECTIONS.find((c) => c.name === "harness:openai").unlocks, ["Codex"]);
});

test("workspace secrets set a connection, project-only ones are partial", () => {
  const secrets = [ws("harness:anthropic"), { ...ws("harness:openai"), id: "p", project: pay }];
  assert.equal(connectionState(secrets, "harness:anthropic").state, "set");
  assert.deepEqual(connectionState(secrets, "harness:openai"), { state: "projects", projects: ["Payments"] });
  assert.deepEqual(connectionState(secrets, "git:pat"), { state: "unset" });
});

test("only required connections without a workspace value are missing", () => {
  assert.deepEqual(missingConnections([ws("harness:anthropic"), ws("git:ssh")]).map((c) => c.name), ["harness:openai"]);
  assert.deepEqual(missingConnections([ws("harness:anthropic"), ws("harness:openai")]), []);
});

test("other secrets exclude workspace connections but keep project overrides", () => {
  const p = { ...ws("git:pat"), id: "pp", project: pay };
  assert.deepEqual(otherSecrets([ws("harness:openai"), ws("db:staging"), p]).map((s) => s.id), ["db:staging", "pp"]);
});

test("save message names what became available", () => {
  assert.equal(savedMessage("harness:openai"), "harness:openai saved. Codex is now available.");
  assert.equal(savedMessage("git:pat"), "git:pat saved. gh pull requests is now available.");
  assert.equal(savedMessage("db:staging"), "db:staging saved.");
});

test("sidebar attention: missing key or missing harness secret", () => {
  const both = [ws("harness:anthropic"), ws("harness:openai")];
  assert.equal(needsAttention(undefined), false);
  assert.equal(needsAttention({ key_configured: true, secrets: both }), false);
  assert.equal(needsAttention({ key_configured: false, secrets: both }), true);
  assert.equal(needsAttention({ key_configured: true, secrets: both.slice(1) }), true);
});

test("Claude, Codex and the GitHub token can be signed in; the SSH key can't", () => {
  assert.deepEqual(
    CONNECTIONS.map((c) => [c.name, c.signIn ?? null]),
    [["harness:anthropic", "claude"], ["harness:openai", "codex"], ["git:ssh", null], ["git:pat", "github"]],
  );
});

test("signInView follows the sign-in through its steps", () => {
  const s = (o) => ({ status: "running", url: "", user_code: "", needs_code: false, error: "", ...o });
  assert.deepEqual(signInView(s({ status: "pending" })), { step: "starting" });
  assert.deepEqual(signInView(s({})), { step: "starting" });
  assert.deepEqual(signInView(s({ url: "https://github.com/login/device", user_code: "AB12-CD34" })), {
    step: "open", url: "https://github.com/login/device", code: "AB12-CD34", pasteBack: false,
  });
  assert.deepEqual(signInView(s({ url: "https://claude.com/x", needs_code: true })), { step: "open", url: "https://claude.com/x", code: "", pasteBack: true });
  // Claude after the code is pasted, before the runner picked it up.
  assert.deepEqual(signInView(s({ url: "https://claude.com/x" })), { step: "verifying" });
  assert.deepEqual(signInView(s({ status: "verifying", url: "https://claude.com/x" })), { step: "verifying" });
  assert.deepEqual(signInView(s({ status: "done" })), { step: "done" });
  assert.deepEqual(signInView(s({ status: "failed", error: "timed out" })), { step: "failed", message: "timed out" });
  assert.equal(signInView(s({ status: "cancelled" })).step, "failed");
});
