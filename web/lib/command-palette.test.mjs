import test from "node:test";
import assert from "node:assert/strict";
import {
  GROUP_ORDER,
  appMatches,
  groupOf,
  linkFromTarget,
  matchParts,
  matchesWords,
  pageContext,
  parseFilters,
  parseRecent,
  passesFilters,
  personaMatches,
  projectOfKey,
  promoteTop,
  pushRecent,
  RECENT_KEY,
  recordRecent,
  removeToken,
  snippetParts,
  sortByGroup,
  ticketKeyFrom,
} from "./command-palette.ts";

test("a typed ticket key is recognised and uppercased", () => {
  assert.equal(ticketKeyFrom("LAZYAGENTS-39"), "LAZYAGENTS-39");
  assert.equal(ticketKeyFrom("  pay-214 "), "PAY-214");
  assert.equal(ticketKeyFrom("PAY-214 deploy"), undefined);
  assert.equal(ticketKeyFrom("deploy"), undefined);
  assert.equal(ticketKeyFrom("-12"), undefined);
});

test("hits land in the mockup's groups", () => {
  assert.equal(groupOf({ kind: "ticket", title: "t" }), "Tickets");
  assert.equal(groupOf({ kind: "artifact", artifact_type: "note", title: "p" }), "Pages");
  assert.equal(groupOf({ kind: "artifact", artifact_type: "adr", title: "a" }), "Artifacts");
  assert.equal(groupOf({ kind: "section", title: "s" }), "Artifacts");
  assert.equal(groupOf({ kind: "chat", title: "c" }), "Chats");
  // A work record is its persona's work page: no separate Agents group.
  assert.equal(groupOf({ kind: "agent", title: "a" }), "Personas");
  assert.equal(groupOf({ kind: "epic", title: "e" }), "Epics");
});

test("links use the wiki path for pages and skip targets without an href", () => {
  assert.deepEqual(
    linkFromTarget({ kind: "artifact", artifact_type: "note", id: "1", title: "Runbook", path: "Payments/Runbook", href: "/wiki/Payments/Runbook" }),
    { id: "1", title: "Payments/Runbook", meta: "wiki page", href: "/wiki/Payments/Runbook", kind: "Pages" },
  );
  assert.equal(linkFromTarget({ kind: "ticket", title: "x" }), undefined);
  assert.equal(linkFromTarget({ kind: "ticket", id: "2", title: "x", display_id: "PAY-1", href: "/tickets/PAY-1" }).meta, "PAY-1");
});

test("sortByGroup follows GROUP_ORDER and keeps ranking within a group", () => {
  const items = [
    { group: "Personas", n: 1 },
    { group: "Tickets", n: 2 },
    { group: "Chats", n: 3 },
    { group: "Tickets", n: 4 },
    { group: "Pages", n: 5 },
  ];
  assert.deepEqual(sortByGroup(items).map((i) => i.n), [2, 4, 5, 3, 1]);
  assert.ok(GROUP_ORDER.indexOf("Recent") < GROUP_ORDER.indexOf("Tickets"));
});

test("matchesWords needs every word", () => {
  assert.ok(matchesWords("Switch to PAY · Payments", "switch pay"));
  assert.ok(matchesWords("anything", ""));
  assert.ok(!matchesWords("Switch to PAY · Payments", "switch web"));
});

test("recent items are deduped, newest first and capped", () => {
  const a = { id: "a", title: "A", href: "/a", kind: "Tickets" };
  const b = { id: "b", title: "B", href: "/b", kind: "Pages" };
  assert.deepEqual(pushRecent(pushRecent([a], b), a).map((r) => r.id), ["a", "b"]);
  assert.equal(pushRecent([a, b], { ...a, id: "c", href: "/c" }, 2).length, 2);
  assert.deepEqual(parseRecent(JSON.stringify([a, { id: "x", title: "X", href: "https://evil" }, 3])), [a]);
  assert.deepEqual(parseRecent("{nope"), []);
  assert.deepEqual(parseRecent(null), []);
});

test("page context comes from the route", () => {
  assert.deepEqual(pageContext("/tickets/LAZYAGENTS-39"), { kind: "ticket", key: "LAZYAGENTS-39" });
  assert.equal(pageContext("/tickets/new"), undefined);
  assert.deepEqual(pageContext("/artifacts/01a1b654-610b-7c95-ada1-9120068a5400"), { kind: "artifact", id: "01a1b654-610b-7c95-ada1-9120068a5400" });
  assert.deepEqual(pageContext("/wiki/Payments/Rollback%20runbook"), { kind: "wiki", path: "Payments/Rollback runbook" });
  assert.equal(pageContext("/board"), undefined);
  assert.equal(projectOfKey("LAZYAGENTS-39"), "LAZYAGENTS");
});

test("a bare number is a ticket in the current project", () => {
  assert.equal(ticketKeyFrom("143", "WEB"), "WEB-143");
  assert.equal(ticketKeyFrom("143"), undefined);
  assert.equal(ticketKeyFrom("pay-1", "WEB"), "PAY-1");
});

test("filters come out of the query and unknown tokens stay text", () => {
  const f = parseFilters("is:ticket deploy STATUS:done is:nope");
  assert.deepEqual(f, { text: "deploy is:nope", groups: ["Tickets"], status: "done", tokens: ["is:ticket", "STATUS:done"] });
  assert.ok(passesFilters({ group: "Tickets", status: "done" }, f));
  assert.ok(!passesFilters({ group: "Pages", status: "done" }, f));
  assert.ok(!passesFilters({ group: "Tickets", status: "todo" }, f));
  assert.equal(removeToken("is:ticket deploy", "is:ticket"), "deploy");
});

test("new hit kinds have groups and carry status", () => {
  assert.equal(groupOf({ kind: "run", title: "r" }), "Runs");
  assert.equal(groupOf({ kind: "memory", title: "m" }), "Memories");
  assert.equal(linkFromTarget({ kind: "run", id: "1", title: "r", href: "/runs/1", status: "failed" }).status, "failed");
});

test("snippets and titles mark every match", () => {
  assert.deepEqual(snippetParts("a «b» c"), [{ text: "a ", match: false }, { text: "b", match: true }, { text: " c", match: false }]);
  assert.deepEqual(matchParts("Deploy the web", "web deploy").filter((p) => p.match).map((p) => p.text), ["Deploy", "web"]);
  assert.deepEqual(matchParts("a.b", "."), [{ text: "a", match: false }, { text: ".", match: true }, { text: "b", match: false }]);
});

test("the best hit is promoted only when grouping buried it", () => {
  const first = { group: "Tickets", score: 2 };
  const page = { group: "Pages", score: 1 };
  assert.deepEqual(promoteTop([first, page]), [first, page]);
  const best = { group: "Personas", score: 5 };
  assert.deepEqual(promoteTop([first, best]), [{ ...best, group: "Top result" }, first]);
});

test("opening an item records it as recent, newest first, once per href", () => {
  const m = new Map();
  const store = { getItem: (k) => m.get(k) ?? null, setItem: (k, v) => m.set(k, v) };
  recordRecent(store, { id: "r1", title: "Search index", href: "/runs/r1", kind: "Runs" });
  recordRecent(store, { id: "p1", title: "Reading list", href: "/knowledge/p1", kind: "Pages" });
  recordRecent(store, { id: "r1", title: "Search index", href: "/runs/r1", kind: "Runs" });
  assert.deepEqual(parseRecent(m.get(RECENT_KEY)).map((r) => r.href), ["/runs/r1", "/knowledge/p1"]);
  // Blocked storage is skipped, not thrown.
  assert.doesNotThrow(() => recordRecent({ getItem: () => { throw new Error("blocked"); }, setItem: () => {} }, { id: "x", title: "x", href: "/x", kind: "Runs" }));
});

test("a typed persona name finds the persona, whole name first, then its start, then a word's start", () => {
  const ps = [{ name: "Health Coach" }, { name: "Laura" }, { name: "Lau" }, { name: "Maya" }];
  assert.deepEqual(personaMatches(ps, "lau").map((p) => p.name), ["Lau", "Laura"]);
  assert.deepEqual(personaMatches(ps, "coach").map((p) => p.name), ["Health Coach"]);
  assert.deepEqual(personaMatches(ps, "hea co").map((p) => p.name), ["Health Coach"]);
  assert.deepEqual(personaMatches(ps, "chat with maya").map((p) => p.name), ["Maya"]);
  assert.deepEqual(personaMatches(ps, "talk to LAURA").map((p) => p.name), ["Laura"]);
  // One letter inside a name isn't a match: it would take the top result from most queries.
  assert.deepEqual(personaMatches(ps, "a"), []);
  assert.deepEqual(personaMatches(ps, "  "), []);
  assert.deepEqual(personaMatches(ps, "chat"), []);
});

test("a typed app name finds the app; app or apps lists them all", () => {
  const apps = [{ label: "Knowledge" }, { label: "Slides" }, { label: "Designs" }];
  assert.deepEqual(appMatches(apps, "sli").map((a) => a.label), ["Slides"]);
  assert.deepEqual(appMatches(apps, "open designs").map((a) => a.label), ["Designs"]);
  assert.deepEqual(appMatches(apps, "go to know").map((a) => a.label), ["Knowledge"]);
  assert.deepEqual(appMatches(apps, "slides app").map((a) => a.label), ["Slides"]);
  assert.equal(appMatches(apps, "apps").length, 3);
  assert.deepEqual(appMatches(apps, "s"), [{ label: "Slides" }]);
  assert.deepEqual(appMatches(apps, "deploy"), []);
  assert.deepEqual(appMatches(apps, ""), []);
});

test("is:app filters to the Apps group", () => {
  assert.deepEqual(parseFilters("is:app slides").groups, ["Apps"]);
});
