// The one narrow-screen breakpoint the mobile UI switches on, and hooks that
// follow a media query. COMPACT is Tailwind's `md`, which chat and playground
// already use as `max-md`.

import { useSyncExternalStore } from "react";

/** Below this width (in CSS px) the app uses the compact, mobile shell. */
export const COMPACT = 768;

/** Matches below COMPACT, the same range as Tailwind's `max-md` variant. */
export const COMPACT_QUERY = `(max-width: ${COMPACT - 0.02}px)`;

/** A useSyncExternalStore subscription and snapshot over one media query. */
export function mediaStore(query: string) {
  return {
    subscribe(onChange: () => void) {
      const mq = window.matchMedia(query);
      mq.addEventListener("change", onChange);
      return () => mq.removeEventListener("change", onChange);
    },
    getSnapshot: () => window.matchMedia(query).matches,
  };
}

const stores = new Map<string, ReturnType<typeof mediaStore>>();

function storeFor(query: string) {
  let store = stores.get(query);
  if (!store) stores.set(query, (store = mediaStore(query)));
  return store;
}

/**
 * Whether `query` matches now, re-rendering when that changes. `server` is
 * the value used while rendering on the server and hydrating.
 */
export function useMediaQuery(query: string, server: boolean): boolean {
  const store = storeFor(query);
  return useSyncExternalStore(store.subscribe, store.getSnapshot, () => server);
}

/** Whether the viewport is narrower than COMPACT; false on the server. */
export function useCompact(): boolean {
  return useMediaQuery(COMPACT_QUERY, false);
}
