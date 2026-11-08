import test from "node:test";
import assert from "node:assert/strict";
import { NAV_GROUPS, currentProject, hasFilters, isActive, modeHref, navGroupsFor, navMode, projectSection, resolveProject, routeMode, switchHref, withProject } from "./project-scope.ts";

const web = { key: "WEB", name: "Web app", color: "#6e56cf", archived: false };
const ops = { key: "OPS", name: "Operations", color: "#3e9bff", archived: false };
const old = { key: "OLD", name: "Legacy", color: "#888888", archived: true };
const projects = [web, ops, old];

test("nav is three groups with unique hrefs", () => {
  assert.deepEqual(NAV_GROUPS.map((g) => g.label), ["Personal", "Project", "Workspace"]);
  const hrefs = NAV_GROUPS.flatMap((g) => g.items.map((i) => i.href));
  assert.equal(new Set(hrefs).size, hrefs.length);
  assert.deepEqual(NAV_GROUPS[0].items.map((i) => i.label), ["Inbox", "Dashboard", "Assistant", "Library", "Workflows", "Learn"]);
  assert.deepEqual(NAV_GROUPS[1].items.map((i) => i.label), ["Board", "Tickets", "Epics", "Plans", "Artifacts"]);
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
  const q = (s) => new URLSearchParams(s);
  assert.equal(currentProject("/board", q("project=OPS"), "WEB", projects), ops);
  assert.equal(currentProject("/inbox", q("project=OPS"), "WEB", projects), web);
  assert.equal(currentProject("/board", q("project=OLD"), "WEB", projects), undefined);
  assert.equal(currentProject("/inbox", q(""), "OLD", projects), undefined);
  assert.equal(currentProject("/inbox", q(""), null, projects), undefined);
  assert.equal(currentProject("/tickets/WEB-1", q(""), "WEB", projects), web);
});

test("cross-project links on optional routes show All projects, bare routes the remembered one", () => {
  const q = (s) => new URLSearchParams(s);
  assert.equal(hasFilters(q("project=WEB")), false);
  assert.equal(hasFilters(q("view=all&assignee=builder")), true);
  assert.equal(currentProject("/tickets", q("view=all&assignee=builder"), "WEB", projects), undefined);
  assert.equal(currentProject("/epics", q("epic=e1"), "WEB", projects), undefined);
  assert.equal(currentProject("/tickets", q(""), "WEB", projects), web);
  // Required routes always need one project.
  assert.equal(currentProject("/board", q("epic=e1"), "WEB", projects), web);
});


test("each page belongs to a mode, and pinned pages keep the last one", () => {
  for (const p of ["/chat", "/chat/1", "/memory", "/personas", "/library", "/workflows", "/learn/2"]) assert.equal(routeMode(p), "chat", p);
  for (const p of ["/board", "/tickets/WEB-1", "/runs/1", "/settings/projects", "/wiki"]) assert.equal(routeMode(p), "work", p);
  for (const p of ["/", "/inbox"]) assert.equal(routeMode(p), undefined, p);
  assert.equal(navMode("/board", "chat"), "work");
  assert.equal(navMode("/inbox", "chat"), "chat");
  assert.equal(navMode("/", null), "work");
});

test("Work pins Inbox and Dashboard above Project and Workspace; Chat keeps all of Personal", () => {
  const labels = (m) => navGroupsFor(m).map((g) => [g.label, g.items.map((i) => i.label)]);
  assert.deepEqual(labels("chat"), [["Personal", NAV_GROUPS[0].items.map((i) => i.label)]]);
  assert.deepEqual(labels("work")[0], ["Personal", ["Inbox", "Dashboard"]]);
  assert.deepEqual(labels("work").slice(1).map(([l]) => l), ["Project", "Workspace"]);
});

test("the toggle resumes chat and returns to the last Work page in the current project", () => {
  assert.equal(modeHref("chat", "WEB", "/runs"), "/chat?project=WEB");
  assert.equal(modeHref("work", "WEB", "/tickets"), "/tickets?project=WEB");
  assert.equal(modeHref("work", "WEB", "/runs/1"), "/runs/1");
  assert.equal(modeHref("work", "WEB", null), "/board?project=WEB");
  assert.equal(modeHref("work", undefined, null), "/board");
});
