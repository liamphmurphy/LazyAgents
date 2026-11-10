import assert from "node:assert/strict";
import test from "node:test";
import { ago, appendLogs, idleRule, isErrorLine, isPlaygroundFence, linesDuring, logSources, normalizePath, parsePlaygroundFence, playgroundOrigin } from "./playground.ts";

const ID = "01a1c6cb-986d-73de-b7f4-52e0cef03de9";

test("the playground fence holds one id", () => {
  assert.equal(isPlaygroundFence("playground"), true);
  assert.equal(isPlaygroundFence("playgrounds"), false);
  assert.equal(parsePlaygroundFence(`\n${ID.toUpperCase()}\n`), ID);
  assert.equal(parsePlaygroundFence("http://localhost:8091"), null);
});

test("a playground is on Vesper's host, at the runner's port", () => {
  assert.equal(playgroundOrigin({ protocol: "http:", hostname: "localhost" }, 8091), "http://localhost:8091");
  assert.equal(playgroundOrigin({ protocol: "http:", hostname: "127.0.0.1" }, 8090), "http://127.0.0.1:8090");
  assert.equal(playgroundOrigin({ protocol: "http:", hostname: "::1" }, 8090), "http://[::1]:8090");
});

test("a typed path never leaves the playground", () => {
  assert.equal(normalizePath("settings/runner"), "/settings/runner");
  assert.equal(normalizePath(" /a?b=1 "), "/a?b=1");
  assert.equal(normalizePath("https://evil.example/x"), "/x");
  assert.equal(normalizePath("//evil.example/x"), "/evil.example/x");
});

const line = (t, source, text) => ({ t, source, stream: "stdout", text });

test("logs keep the newest lines, and know their sources and errors", () => {
  const lines = appendLogs([line("2026-11-10T17:44:19.000Z", "web", "a")], [line("2026-11-10T17:44:19.100Z", "api", "b"), line("2026-11-10T17:44:19.200Z", "api", "c")], 2);
  assert.deepEqual(lines.map((l) => l.text), ["b", "c"]);
  assert.deepEqual(logSources([line("", "web", ""), line("", "api", ""), line("", "web", "")]), ["web", "api"]);
  assert.equal(isErrorLine(line("", "api", 'error: null value in column "packages"')), true);
  assert.equal(isErrorLine(line("", "api", "terrorist")), false);
});

test("a request's lines are the ones written while it ran", () => {
  const sent = Date.parse("2026-11-10T17:44:19.000Z");
  const lines = [line("2026-11-10T17:44:10.000Z", "api", "before"), line("2026-11-10T17:44:19.050Z", "api", "during"), line("2026-11-10T17:44:20.000Z", "postgres", "just after"), line("2026-11-10T17:44:30.000Z", "api", "later")];
  assert.deepEqual(linesDuring(lines, sent, sent + 20).map((l) => l.text), ["during", "just after"]);
});

test("footer wording", () => {
  const now = Date.parse("2026-11-10T18:00:00Z");
  assert.equal(ago("2026-11-10T17:54:00Z", now), "6 min ago");
  assert.equal(ago("2026-11-10T17:59:30Z", now), "just now");
  assert.equal(ago(null, now), "");
  assert.equal(idleRule(1800), "stops after 30 min idle");
  assert.equal(idleRule(7200), "stops after 2 h idle");
  assert.equal(idleRule(null), "stops when unused");
});
