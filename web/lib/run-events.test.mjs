import assert from "node:assert/strict";
import test from "node:test";
import { eventsAfter, RUN_EVENTS_OVERLAP, RUN_EVENTS_PAGE } from "./run-events.ts";

/** A server holding the given seqs that answers like GET /runs/{id}?after=seq. */
function server(seqs) {
  const events = seqs.map((seq) => ({ seq }));
  const asked = [];
  const page = async (after) => {
    asked.push(after);
    return events.filter((e) => e.seq > after).slice(0, RUN_EVENTS_PAGE);
  };
  return { events, asked, page };
}
const upTo = (n) => Array.from({ length: n }, (_, i) => i + 1);
const seqs = (events) => events.map((e) => e.seq);

test("reads past the first page: a long chat's newest turn is not cut off", async () => {
  const s = server(upTo(2 * RUN_EVENTS_PAGE + 37));
  const got = await eventsAfter([], s.page);
  assert.deepEqual(seqs(got), upTo(2 * RUN_EVENTS_PAGE + 37));
  assert.deepEqual(s.asked, [0, RUN_EVENTS_PAGE, 2 * RUN_EVENTS_PAGE]);
});

test("a poll re-reads only the newest few known events, and adds what came after them", async () => {
  const s = server(upTo(1500));
  const got = await eventsAfter(s.events.slice(0, 1490), s.page);
  assert.deepEqual(s.asked, [1490 - RUN_EVENTS_OVERLAP]);
  assert.deepEqual(seqs(got), upTo(1500));
});

test("an event that commits late, below the newest seq, is still picked up in order", async () => {
  const s = server([...upTo(99), 101, 102]);
  const known = await eventsAfter([], s.page);
  s.events.splice(99, 0, { seq: 100 });
  assert.deepEqual(seqs(await eventsAfter(known, s.page)), upTo(102));
});

test("exactly one full page stops on the empty page after it", async () => {
  const s = server(upTo(RUN_EVENTS_PAGE));
  const got = await eventsAfter([], s.page);
  assert.equal(got.length, RUN_EVENTS_PAGE);
  assert.deepEqual(s.asked, [0, RUN_EVENTS_PAGE]);
});

test("nothing new keeps the same list", async () => {
  const s = server(upTo(3));
  const known = s.events.slice();
  assert.equal(await eventsAfter(known, s.page), known);
});

test("a server that ignores ?after cannot loop forever or duplicate events", async () => {
  const full = upTo(RUN_EVENTS_PAGE).map((seq) => ({ seq }));
  let calls = 0;
  const got = await eventsAfter([], async () => (calls++, full));
  assert.equal(calls, 2);
  assert.deepEqual(seqs(got), upTo(RUN_EVENTS_PAGE));
  // Polling again: still no duplicates.
  assert.deepEqual(seqs(await eventsAfter(got, async () => full)), upTo(RUN_EVENTS_PAGE));
});
