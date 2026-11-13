// Fetching a run's events in pages. Kept free of React so `npm run test:unit` covers it.

/** GET /runs/{id} returns at most this many events after ?after=seq (ListRunEvents' LIMIT). */
export const RUN_EVENTS_PAGE = 1000;

/**
 * How many of the newest known events each poll reads again. A seq is taken when its row is
 * inserted, not when it commits, so with two writers on one run (the runner, and the API
 * stopping a turn) a lower seq can become visible after a higher one. Re-reading the tail
 * picks it up instead of skipping it for good.
 */
export const RUN_EVENTS_OVERLAP = 32;

/**
 * The known events plus any new ones, in seq order, read page by page until a short page shows
 * it has caught up. One chat run spans many turns, so a long chat has more events than one page
 * holds; reading only the first page left every later turn without its tool calls.
 */
export async function eventsAfter<E extends { seq: number }>(known: E[], page: (after: number) => Promise<E[]>): Promise<E[]> {
  let all = known;
  let after = known.length > RUN_EVENTS_OVERLAP ? known[known.length - RUN_EVENTS_OVERLAP - 1].seq : 0;
  for (;;) {
    const next = await page(after);
    const have = new Set(all.filter((e) => e.seq > after).map((e) => e.seq));
    // Only events past the cursor count, so a server that ignores ?after can't loop us forever.
    const fresh = next.filter((e) => e.seq > after && !have.has(e.seq));
    if (fresh.length) {
      const inOrder = fresh[0].seq > (all.at(-1)?.seq ?? 0) && fresh.every((e, i) => i === 0 || e.seq > fresh[i - 1].seq);
      all = inOrder ? [...all, ...fresh] : [...all, ...fresh].sort((a, b) => a.seq - b.seq);
    }
    const newest = next.at(-1)?.seq;
    if (next.length < RUN_EVENTS_PAGE || newest === undefined || newest <= after) return all;
    after = newest;
  }
}
