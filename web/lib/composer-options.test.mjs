import test from "node:test";
import assert from "node:assert/strict";
import { composerOptions } from "./composer-options.ts";

const ids = (list, place) => list.filter((o) => !place || o.place === place).map((o) => o.id);

test("a chat whose persona can't code has no repository option", () => {
  const list = composerOptions({ code: false, research: false });
  assert.deepEqual(ids(list), ["context", "attach", "research"]);
});

test("a coding chat without a repository offers the picker with nothing chosen", () => {
  const repo = composerOptions({ code: true, research: false }).find((o) => o.id === "repository");
  assert.deepEqual(repo, { id: "repository", label: "Repository", icon: "branch", value: undefined, opens: "popover", place: "options" });
});

test("a chat with a repository shows its short name, and a queued one for the next turn", () => {
  const value = (s) => composerOptions({ code: true, research: false, ...s }).find((o) => o.id === "repository").value;
  assert.equal(value({ repo: "https://github.com/acme/web" }), "acme/web");
  assert.equal(value({ repo: "https://github.com/acme/web", pendingRepo: "https://github.com/acme/api" }), "acme/web → acme/api (next turn)");
});

test("a fresh running chat lists every control in the desktop order", () => {
  const list = composerOptions({
    code: true,
    repo: "https://github.com/acme/web",
    persona: "Ada",
    sendMode: "Follow-up",
    research: true,
    temporary: false,
    model: "Opus 5.5 · Auto",
    project: { name: "Web" },
  });
  assert.deepEqual(ids(list, "options"), ["context", "attach", "repository", "persona", "send-mode", "research", "temporary"]);
  assert.deepEqual(ids(list, "actions"), ["model"]);
  assert.deepEqual(ids(list, "header"), ["project"]);
  assert.equal(list.find((o) => o.id === "research").on, true);
  assert.equal(list.find((o) => o.id === "temporary").on, false);
  assert.equal(list.find((o) => o.id === "model").value, "Opus 5.5 · Auto");
  assert.equal(list.find((o) => o.id === "project").value, "Web");
});
