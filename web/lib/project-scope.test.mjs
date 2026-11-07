import test from "node:test";
import assert from "node:assert/strict";
import { NAV_GROUPS, currentProject, isActive, projectSection, resolveProject, switchHref, withProject } from "./project-scope.ts";

const web = { key: "WEB", name: "Web app", color: "#6e56cf", archived: false };
const ops = { key: "OPS", name: "Operations", color: "#3e9bff", archived: false };
const old = { key: "OLD", name: "Legacy", color: "#888888", archived: true };
const projects = [web, ops, old];

test("nav is three groups with unique hrefs", () => {
  assert.deepEqual(NAV_GROUPS.map((g) => g.label), ["Personal", "Project", "Workspace"]);
  const hrefs = NAV_GROUPS.flatMap((g) => g.items.map((i) => i.href));
  assert.equal(new Set(hrefs).size, hrefs.length);
  assert.deepEqual(NAV_GROUPS[1].items.map((i) => i.label), ["Board", "Tickets", "Epics", "Plans", "Chat", "Artifacts"]);
});

test("isActive matches the section, Dashboard only on /", () => {
  assert.equal(isActive("/", "/"), true);
  assert.equal(isActive("/board", "/"), false);
  assert.equal(isActive("/runs/abc", "/runs"), true);
  assert.equal(isActive("/runsx", "/runs"), false);
});

test("projectSection finds the project route of detail pages", () => {
  assert.equal(projectSection("/board"), "/board");
  assert.equal(projectSection("/chat/123"), "/chat");
  assert.equal(projectSection("/tickets/WEB-1"), "/tickets");
  assert.equal(projectSection("/inbox"), undefined);
  assert.equal(projectSection("/"), undefined);
});

test("withProject only scopes project routes", () => {
  assert.equal(withProject("/board", "WEB"), "/board?project=WEB");
  assert.equal(withProject("/board", null), "/board");
  assert.equal(withProject("/runs", "WEB"), "/runs");
});

test("resolveProject reads the URL first, then the remembered project", () => {
  assert.deepEqual(resolveProject("OPS", "WEB", projects), { state: "project", project: ops });
  assert.deepEqual(resolveProject(null, "WEB", projects), { state: "redirect", key: "WEB" });
  assert.deepEqual(resolveProject("", null, projects), { state: "none" });
});

test("resolveProject never resolves archived or unknown projects", () => {
  assert.deepEqual(resolveProject("OLD", "WEB", projects), { state: "unavailable", key: "OLD", archived: old });
  assert.deepEqual(resolveProject("NOPE", "WEB", projects), { state: "unavailable", key: "NOPE", archived: undefined });
  assert.deepEqual(resolveProject(null, "OLD", projects), { state: "none" });
  assert.deepEqual(resolveProject(null, "GONE", projects), { state: "none" });
});

test("switchHref rewrites the current project route and drops project-bound params", () => {
  const q = (s) => new URLSearchParams(s);
  assert.equal(switchHref("/board", q("project=WEB&epic=e1&assignee=builder&ticket=WEB-1"), "OPS"), "/board?assignee=builder&project=OPS");
  assert.equal(switchHref("/tickets", q("project=WEB&view=all"), null), "/tickets?view=all");
  assert.equal(switchHref("/plans", q("id=p1&project=WEB"), "OPS"), "/plans?project=OPS");
  assert.equal(switchHref("/chat/123", q("project=WEB"), "OPS"), "/chat?project=OPS");
  assert.equal(switchHref("/tickets/WEB-1", q(""), "OPS"), "/tickets?project=OPS");
  assert.equal(switchHref("/tickets/new", q("project=WEB"), "OPS"), null);
  assert.equal(switchHref("/inbox", q(""), "OPS"), null);
});

test("currentProject prefers the URL on project routes and hides archived projects", () => {
  assert.equal(currentProject("/board", "OPS", "WEB", projects), ops);
  assert.equal(currentProject("/inbox", "OPS", "WEB", projects), web);
  assert.equal(currentProject("/board", "OLD", "WEB", projects), undefined);
  assert.equal(currentProject("/inbox", null, "OLD", projects), undefined);
  assert.equal(currentProject("/inbox", null, null, projects), undefined);
});
