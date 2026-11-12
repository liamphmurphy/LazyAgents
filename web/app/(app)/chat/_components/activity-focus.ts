"use client";

import { useSyncExternalStore } from "react";

// "View reasoning" on a chat step opens the desktop dock's Activity tab on that step. The step is
// handed over here: the transcript (conversation.tsx) and the dock's Activity (chat-panel-host) are siblings.

type Focus = { key: string; at: number } | undefined;
let focus: Focus;
const listeners = new Set<() => void>();

export function focusActivityStep(key: string) {
  focus = { key, at: Date.now() };
  listeners.forEach((l) => l());
}

export function useActivityFocus(): Focus {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => focus,
    () => undefined,
  );
}
