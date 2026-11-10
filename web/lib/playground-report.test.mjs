import assert from "node:assert/strict";
import test from "node:test";
import { applyProbe, emptyProbe, logsFor, MAX_PROBE_ENTRIES, onComposerDrop, playgroundReport, reportNames, sendToComposer } from "./playground-report.ts";

const probe = (...msgs) => msgs.reduce((s, m, i) => applyProbe(s, m, 1_000 * (i + 1)), emptyProbe());

test("the probe's messages build up the page, console errors and failed requests", () => {
  const s = probe(
    { vesperProbe: 1, type: "location", path: "/orders?page=2", title: "Orders" },
    { vesperProbe: 1, type: "console", text: "TypeError: x is undefined" },
    { vesperProbe: 1, type: "request", method: "post", url: "/api/orders", status: 500, id: "req-1", error: "" },
  );
  assert.equal(s.path, "/orders?page=2");
  assert.equal(s.title, "Orders");
  assert.deepEqual(s.console, [{ text: "TypeError: x is undefined", t: 2000 }]);
  assert.deepEqual(s.requests, [{ method: "post", url: "/api/orders", status: 500, id: "req-1", error: "", t: 3000 }]);
});

test("anything that isn't a well-formed probe message leaves the state alone", () => {
  const before = probe({ vesperProbe: 1, type: "location", path: "/" });
  for (const m of [null, "text", { type: "console", text: "no marker" }, { vesperProbe: 1, type: "unknown" }, { vesperProbe: 1, type: "console", text: { not: "a string" } }]) {
    assert.equal(applyProbe(before, m, 5), before);
  }
  const odd = applyProbe(before, { vesperProbe: 1, type: "request", method: {}, url: 7, status: "500" }, 5);
  assert.deepEqual(odd.requests[0], { method: "GET", url: "", status: 0, id: "", error: "", t: 5 });
});

test("only the latest entries are kept", () => {
  let s = emptyProbe();
  for (let i = 0; i < MAX_PROBE_ENTRIES + 5; i++) s = applyProbe(s, { vesperProbe: 1, type: "console", text: `e${i}` }, i);
  assert.equal(s.console.length, MAX_PROBE_ENTRIES);
  assert.equal(s.console[0].text, "e5");
});

test("a failed request's logs are the lines naming its id, else stderr written around it", () => {
  const at = Date.parse("2026-11-10T22:00:00Z");
  const line = (ms, text, stream = "stdout") => ({ t: new Date(at + ms).toISOString(), source: "api", stream, text });
  const lines = [line(-5000, "boot"), line(100, "GET /api/orders id=req-1 500"), line(200, "panic: nil map", "stderr"), line(9000, "later", "stderr")];
  assert.deepEqual(logsFor({ id: "req-1", t: at }, lines).map((l) => l.text), ["GET /api/orders id=req-1 500"]);
  assert.deepEqual(logsFor({ id: "", t: at }, lines).map((l) => l.text), ["panic: nil map"]);
});

test("the report says what was seen, and what wasn't", () => {
  const at = Date.parse("2026-11-10T22:00:00Z");
  const s = probe(
    { vesperProbe: 1, type: "location", path: "/orders", title: "" },
    { vesperProbe: 1, type: "console", text: "has ``` fences" },
    { vesperProbe: 1, type: "request", method: "GET", url: "/api/x", status: 0, error: "network error" },
  );
  const md = playgroundReport({ origin: "http://localhost:8090", probe: s, logs: [], branch: "feat/x", at, noScreenshot: "sharing was cancelled" });
  assert.match(md, /- Page: `\/orders`/);
  assert.match(md, /- Branch: `feat\/x`/);
  assert.match(md, /- Screenshot: none \(sharing was cancelled\)/);
  assert.match(md, /````\nhas ``` fences\n````/, "a fence longer than any run of backticks in the text");
  assert.match(md, /### GET \/api\/x → no response: network error/);

  const blank = playgroundReport({ origin: "o", probe: emptyProbe(), logs: [], at });
  assert.match(blank, /Page: unknown/);
  assert.match(blank, /Screenshot: attached/);
  assert.equal((blank.match(/None since the page loaded/g) ?? []).length, 2);
});

test("attachment names are stamped and pass the attachment rules", () => {
  assert.deepEqual(reportNames(Date.parse("2026-11-10T22:01:02Z")), { report: "playground-20261110-220102.md", screenshot: "playground-20261110-220102.png" });
});

test("drops reach the composer of the same chat only, and a send with none listening says so", () => {
  const got = [];
  const off = onComposerDrop("chat-a", (d) => got.push(d));
  assert.equal(sendToComposer("chat-b", { text: "x" }), false);
  assert.equal(sendToComposer("chat-a", { files: ["y"] }), true);
  off();
  assert.equal(sendToComposer("chat-a", { text: "z" }), false);
  assert.deepEqual(got, [{ files: ["y"] }]);
});

test("a whole-tab capture is cropped to the frame, scaled to the video's pixels", async () => {
  const { cropFor } = await import("./frame-capture.ts");
  const frame = { x: 600, y: 100, width: 500, height: 600 };
  assert.deepEqual(cropFor(frame, { width: 1200, height: 800 }, { width: 2400, height: 1600 }), { x: 1200, y: 200, width: 1000, height: 1200 });
  assert.equal(cropFor(frame, { width: 1200, height: 800 }, { width: 1920, height: 1080 }), null, "another surface: no crop");
  assert.equal(cropFor(frame, { width: 1200, height: 800 }, { width: 0, height: 0 }), null);
});
