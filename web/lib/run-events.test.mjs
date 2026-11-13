import assert from "node:assert/strict";
import test from "node:test";
import { eventsAfter, RUN_EVENTS_PAGE } from "./run-events.ts";

/** A server holding events 1..n that answers like GET /runs/{id}?after=seq. */
function server(n) {
  const events = Array.from({ length: n }, (_, i) => ({ seq: i + 1 }));
  const asked = [];
  const page = async (after) => {
    asked.push(after);
    return events.filter((e) => e.seq > after).slice(0, RUN_EVENTS_PAGE);
  };
  return { events, asked, page };
}

test("reads past the first page: a long chat's newest turn is not cut off", async () => {
  const s = server(2 * RUN_EVENTS_PAGE + 37);
  const got = await eventsAfter([], s.page);
  assert.equal(got.length, s.events.length);
  assert.equal(got.at(-1).seq, s.events.length);
  assert.deepEqual(s.asked, [0, RUN_EVENTS_PAGE, 2 * RUN_EVENTS_PAGE]);
});

test("a poll asks only for what came after the newest known event", async () => {
  const s = server(1500);
  const known = s.events.slice(0, 1490);
  const got = await eventsAfter(known, s.page);
  assert.deepEqual(s.asked, [1490]);
  assert.deepEqual(got.map((e) => e.seq).slice(-10), [1491, 1492, 1493, 1494, 1495, 1496, 1497, 1498, 1499, 1500]);
});

test("exactly one full page stops on the empty page after it", async () => {
  const s = server(RUN_EVENTS_PAGE);
  const got = await eventsAfter([], s.page);
  assert.equal(got.length, RUN_EVENTS_PAGE);
  assert.deepEqual(s.asked, [0, RUN_EVENTS_PAGE]);
});

test("nothing new keeps the same list", async () => {
  const s = server(3);
  const known = s.events.slice();
  assert.equal(await eventsAfter(known, s.page), known);
});

test("a server that ignores ?after cannot loop forever or duplicate events", async () => {
  const full = Array.from({ length: RUN_EVENTS_PAGE }, (_, i) => ({ seq: i + 1 }));
  let calls = 0;
  const got = await eventsAfter([], async () => (calls++, full));
  assert.equal(calls, 2);
  assert.equal(got.length, RUN_EVENTS_PAGE);
});
