"use client";

import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

const prefixes: Record<string, string[]> = {
  planning: ["planning", "inbox", "bootstrap"],
  metrics: ["dashboard", "agent-overviews"],
  ticket: ["planning", "tickets", "boards", "inbox", "bootstrap", "knowledge", "search"],
  epic: ["projects", "planning", "bootstrap", "tickets", "boards", "epics", "inbox", "search"],
  activity: ["tickets", "inbox", "bootstrap"],
  approval: ["tickets", "runs", "inbox", "bootstrap"],
  // "dock" is the activity orb: which harnesses are running.
  run: ["planning", "runs", "boards", "tickets", "agents", "inbox", "bootstrap", "runner", "search", "dock"],
  agent: ["agents", "bootstrap", "search"],
  artifact: ["artifacts", "wiki", "knowledge", "inbox", "bootstrap", "search"],
  knowledge: ["knowledge", "wiki"],
  project: ["projects", "planning", "bootstrap", "tickets", "boards", "inbox", "search"],
  inbox: ["inbox", "bootstrap"],
  chat: ["chats", "canvases", "dock"],
  // A persona edit changes the name, color and avatar every chat using it embeds.
  persona: ["personas", "chats", "dock"],
};

/** One stream per shell. Coalesce bursts from harness logs before refetching. */
export function LiveUpdates() {
  const qc = useQueryClient();
  const [offline, setOffline] = useState(false);
  useEffect(() => {
    const stream = new EventSource("/events");
    let timer: ReturnType<typeof setTimeout> | undefined;
    const pending = new Set<string>();
    const flush = () => {
      timer = undefined;
      for (const key of pending) void qc.invalidateQueries({ queryKey: [key] });
      pending.clear();
    };
    stream.onopen = () => setOffline(false);
    stream.onerror = () => setOffline(true);
    stream.onmessage = (e) => {
      try {
        const { type } = JSON.parse(e.data) as { type: string };
        if (type === "reset") {
          void qc.invalidateQueries();
          return;
        }
        for (const key of type === "run.events"
          ? ["runs"]
          : (prefixes[type.split(".")[0]] ?? []))
          pending.add(key);
        if (type !== "run.events") {
          pending.add("dashboard");
          pending.add("epics");
          pending.add("agent-overviews");
        }
        if (!timer) timer = setTimeout(flush, 100);
      } catch {
        /* Ignore malformed hints; reconnect resets every cache. */
      }
    };
    return () => {
      stream.close();
      if (timer) clearTimeout(timer);
    };
  }, [qc]);
  return offline ? (
    <div
      role="status"
      className="fixed bottom-5 left-1/2 z-50 -translate-x-1/2 rounded-lg border border-border-float bg-surface-raised px-4 py-2 text-sm text-fg-secondary shadow-lg"
    >
      Live updates paused. Reconnecting…
    </div>
  ) : null;
}
