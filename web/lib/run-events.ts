// Fetching a run's events in pages. Kept free of React so `npm run test:unit` covers it.

/** GET /runs/{id} returns at most this many events after ?after=seq (ListRunEvents' LIMIT). */
export const RUN_EVENTS_PAGE = 1000;

/**
 * The events after `known`, read page by page until a short page shows it has caught up.
 * One chat run spans many turns, so a long chat has more events than one page holds;
 * reading only the first page left every later turn without its tool calls.
 */
export async function eventsAfter<E extends { seq: number }>(known: E[], page: (after: number) => Promise<E[]>): Promise<E[]> {
  let all = known;
  for (;;) {
    const after = all.at(-1)?.seq ?? 0;
    const next = await page(after);
    // Only events past the cursor count, so a server that ignores ?after can't loop us forever.
    const fresh = next.filter((e) => e.seq > after);
    if (fresh.length) all = [...all, ...fresh];
    if (next.length < RUN_EVENTS_PAGE || fresh.length === 0) return all;
  }
}
